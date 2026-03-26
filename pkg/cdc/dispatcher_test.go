package cdc

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/limyedb/limyedb/pkg/point"
)

func TestEventTypes(t *testing.T) {
	tests := []struct {
		eventType EventType
		expected  string
	}{
		{EventInsert, "insert"},
		{EventDelete, "delete"},
		{EventUpdate, "update"},
	}

	for _, tt := range tests {
		t.Run(string(tt.eventType), func(t *testing.T) {
			if string(tt.eventType) != tt.expected {
				t.Errorf("EventType = %s, want %s", tt.eventType, tt.expected)
			}
		})
	}
}

func TestEventJSONMarshal(t *testing.T) {
	event := Event{
		Collection: "test_collection",
		Type:       EventInsert,
		PointID:    "point-123",
		Timestamp:  1234567890,
		Point: &point.Point{
			ID:     "point-123",
			Vector: []float32{0.1, 0.2, 0.3},
			Payload: map[string]interface{}{
				"name": "test",
			},
		},
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded Event
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.Collection != event.Collection {
		t.Errorf("Collection = %s, want %s", decoded.Collection, event.Collection)
	}
	if decoded.Type != event.Type {
		t.Errorf("Type = %s, want %s", decoded.Type, event.Type)
	}
	if decoded.PointID != event.PointID {
		t.Errorf("PointID = %s, want %s", decoded.PointID, event.PointID)
	}
	if decoded.Timestamp != event.Timestamp {
		t.Errorf("Timestamp = %d, want %d", decoded.Timestamp, event.Timestamp)
	}
}

func TestWebhookSubscription(t *testing.T) {
	sub := WebhookSubscription{
		URL: "https://example.com/webhook",
		Headers: map[string]string{
			"Authorization": "Bearer token123",
			"X-Custom":      "custom-value",
		},
	}

	data, err := json.Marshal(sub)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded WebhookSubscription
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.URL != sub.URL {
		t.Errorf("URL = %s, want %s", decoded.URL, sub.URL)
	}
	if decoded.Headers["Authorization"] != sub.Headers["Authorization"] {
		t.Error("Authorization header mismatch")
	}
}

func TestDispatcherSubscribe(t *testing.T) {
	d := &Dispatcher{
		subscriptions: make(map[string][]WebhookSubscription),
		eventCh:       make(chan Event, 100),
		client:        &http.Client{Timeout: 3 * time.Second},
	}

	sub1 := WebhookSubscription{URL: "http://example.com/hook1"}
	sub2 := WebhookSubscription{URL: "http://example.com/hook2"}

	d.Subscribe("collection1", sub1)
	d.Subscribe("collection1", sub2)
	d.Subscribe("collection2", sub1)

	subs1 := d.Subscriptions("collection1")
	if len(subs1) != 2 {
		t.Errorf("Subscriptions(collection1) = %d, want 2", len(subs1))
	}

	subs2 := d.Subscriptions("collection2")
	if len(subs2) != 1 {
		t.Errorf("Subscriptions(collection2) = %d, want 1", len(subs2))
	}

	subs3 := d.Subscriptions("nonexistent")
	if len(subs3) != 0 {
		t.Errorf("Subscriptions(nonexistent) = %d, want 0", len(subs3))
	}
}

func TestDispatcherPublish(t *testing.T) {
	d := &Dispatcher{
		subscriptions: make(map[string][]WebhookSubscription),
		eventCh:       make(chan Event, 10),
		client:        &http.Client{Timeout: 3 * time.Second},
	}

	event := Event{
		Collection: "test",
		Type:       EventInsert,
		PointID:    "123",
		Timestamp:  time.Now().UnixMilli(),
	}

	d.Publish(event)

	select {
	case received := <-d.eventCh:
		if received.Collection != event.Collection {
			t.Errorf("Collection = %s, want %s", received.Collection, event.Collection)
		}
		if received.Type != event.Type {
			t.Errorf("Type = %s, want %s", received.Type, event.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Event not received in channel")
	}
}

func TestDispatcherPublishBufferFull(t *testing.T) {
	d := &Dispatcher{
		subscriptions: make(map[string][]WebhookSubscription),
		eventCh:       make(chan Event, 1), // Very small buffer
		client:        &http.Client{Timeout: 3 * time.Second},
	}

	// Fill the buffer
	d.Publish(Event{Collection: "test", Type: EventInsert})

	// This should not block (drop the event instead)
	done := make(chan bool)
	go func() {
		d.Publish(Event{Collection: "test", Type: EventInsert})
		done <- true
	}()

	select {
	case <-done:
		// Good, didn't block
	case <-time.After(100 * time.Millisecond):
		t.Error("Publish blocked on full buffer")
	}
}

func TestDispatcherWebhookDelivery(t *testing.T) {
	var received atomic.Int32
	var receivedEvents []Event
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Method = %s, want POST", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("Content-Type should be application/json")
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("ReadAll() error = %v", err)
			return
		}

		var event Event
		if err := json.Unmarshal(body, &event); err != nil {
			t.Errorf("json.Unmarshal() error = %v", err)
			return
		}

		mu.Lock()
		receivedEvents = append(receivedEvents, event)
		mu.Unlock()

		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	d := &Dispatcher{
		subscriptions: make(map[string][]WebhookSubscription),
		eventCh:       make(chan Event, 100),
		client:        &http.Client{Timeout: 3 * time.Second},
	}

	// Start worker
	go d.worker()

	// Subscribe
	d.Subscribe("test_collection", WebhookSubscription{URL: server.URL})

	// Publish events
	for i := 0; i < 5; i++ {
		d.Publish(Event{
			Collection: "test_collection",
			Type:       EventInsert,
			PointID:    "point-" + string(rune('0'+i)),
			Timestamp:  time.Now().UnixMilli(),
		})
	}

	// Wait for delivery
	time.Sleep(500 * time.Millisecond)

	if received.Load() != 5 {
		t.Errorf("Received %d webhooks, want 5", received.Load())
	}
}

func TestDispatcherCustomHeaders(t *testing.T) {
	var receivedAuth, receivedCustom string
	var headerMu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headerMu.Lock()
		receivedAuth = r.Header.Get("Authorization")
		receivedCustom = r.Header.Get("X-Custom-Header")
		headerMu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	d := &Dispatcher{
		subscriptions: make(map[string][]WebhookSubscription),
		eventCh:       make(chan Event, 100),
		client:        &http.Client{Timeout: 3 * time.Second},
	}

	go d.worker()

	d.Subscribe("test", WebhookSubscription{
		URL: server.URL,
		Headers: map[string]string{
			"Authorization":   "Bearer secret-token",
			"X-Custom-Header": "custom-value",
		},
	})

	d.Publish(Event{
		Collection: "test",
		Type:       EventInsert,
		PointID:    "123",
		Timestamp:  time.Now().UnixMilli(),
	})

	time.Sleep(200 * time.Millisecond)

	headerMu.Lock()
	auth := receivedAuth
	custom := receivedCustom
	headerMu.Unlock()

	if auth != "Bearer secret-token" {
		t.Errorf("Authorization = %s, want Bearer secret-token", auth)
	}
	if custom != "custom-value" {
		t.Errorf("X-Custom-Header = %s, want custom-value", custom)
	}
}

func TestDispatcherNoSubscribers(t *testing.T) {
	d := &Dispatcher{
		subscriptions: make(map[string][]WebhookSubscription),
		eventCh:       make(chan Event, 100),
		client:        &http.Client{Timeout: 3 * time.Second},
	}

	go d.worker()

	// Publish to collection with no subscribers - should not panic
	d.Publish(Event{
		Collection: "no_subscribers",
		Type:       EventInsert,
		PointID:    "123",
		Timestamp:  time.Now().UnixMilli(),
	})

	// Give worker time to process
	time.Sleep(100 * time.Millisecond)
}

func TestDispatcherMultipleSubscribers(t *testing.T) {
	var count1, count2 atomic.Int32

	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count1.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count2.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server2.Close()

	d := &Dispatcher{
		subscriptions: make(map[string][]WebhookSubscription),
		eventCh:       make(chan Event, 100),
		client:        &http.Client{Timeout: 3 * time.Second},
	}

	go d.worker()

	d.Subscribe("multi", WebhookSubscription{URL: server1.URL})
	d.Subscribe("multi", WebhookSubscription{URL: server2.URL})

	d.Publish(Event{
		Collection: "multi",
		Type:       EventInsert,
		PointID:    "123",
		Timestamp:  time.Now().UnixMilli(),
	})

	time.Sleep(200 * time.Millisecond)

	if count1.Load() != 1 {
		t.Errorf("Server1 received %d, want 1", count1.Load())
	}
	if count2.Load() != 1 {
		t.Errorf("Server2 received %d, want 1", count2.Load())
	}
}

func TestDispatcherConcurrentPublish(t *testing.T) {
	var received atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	d := &Dispatcher{
		subscriptions: make(map[string][]WebhookSubscription),
		eventCh:       make(chan Event, 1000),
		client:        &http.Client{Timeout: 3 * time.Second},
	}

	go d.worker()

	d.Subscribe("concurrent", WebhookSubscription{URL: server.URL})

	const numGoroutines = 10
	const eventsPerGoroutine = 10

	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < eventsPerGoroutine; i++ {
				d.Publish(Event{
					Collection: "concurrent",
					Type:       EventInsert,
					PointID:    "point",
					Timestamp:  time.Now().UnixMilli(),
				})
			}
		}(g)
	}

	wg.Wait()
	time.Sleep(time.Second) // Wait for all webhooks to be delivered

	expected := int32(numGoroutines * eventsPerGoroutine)
	if received.Load() != expected {
		t.Errorf("Received %d webhooks, want %d", received.Load(), expected)
	}
}

func TestDispatcherConcurrentSubscribe(t *testing.T) {
	d := &Dispatcher{
		subscriptions: make(map[string][]WebhookSubscription),
		eventCh:       make(chan Event, 100),
		client:        &http.Client{Timeout: 3 * time.Second},
	}

	const numGoroutines = 10

	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			d.Subscribe("concurrent_sub", WebhookSubscription{
				URL: "http://example.com/hook",
			})
		}(g)
	}

	wg.Wait()

	subs := d.Subscriptions("concurrent_sub")
	if len(subs) != numGoroutines {
		t.Errorf("Subscriptions count = %d, want %d", len(subs), numGoroutines)
	}
}

func TestDispatcherWebhookFailure(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	d := &Dispatcher{
		subscriptions: make(map[string][]WebhookSubscription),
		eventCh:       make(chan Event, 100),
		client:        &http.Client{Timeout: 3 * time.Second},
	}

	go d.worker()

	d.Subscribe("failing", WebhookSubscription{URL: server.URL})

	d.Publish(Event{
		Collection: "failing",
		Type:       EventInsert,
		PointID:    "123",
		Timestamp:  time.Now().UnixMilli(),
	})

	time.Sleep(200 * time.Millisecond)

	// Should still have attempted the webhook
	if attempts.Load() < 1 {
		t.Error("Webhook should have been attempted")
	}
}

func TestDispatcherUnreachableWebhook(t *testing.T) {
	d := &Dispatcher{
		subscriptions: make(map[string][]WebhookSubscription),
		eventCh:       make(chan Event, 100),
		client:        &http.Client{Timeout: 100 * time.Millisecond},
	}

	go d.worker()

	// Subscribe to unreachable URL
	d.Subscribe("unreachable", WebhookSubscription{URL: "http://192.0.2.1:12345/webhook"})

	// This should not hang or panic
	d.Publish(Event{
		Collection: "unreachable",
		Type:       EventInsert,
		PointID:    "123",
		Timestamp:  time.Now().UnixMilli(),
	})

	time.Sleep(300 * time.Millisecond)
}

func TestGetDispatcherSingleton(t *testing.T) {
	// Note: This test may affect other tests due to singleton nature
	d1 := GetDispatcher()
	d2 := GetDispatcher()

	if d1 != d2 {
		t.Error("GetDispatcher() should return the same instance")
	}
}

func BenchmarkPublish(b *testing.B) {
	d := &Dispatcher{
		subscriptions: make(map[string][]WebhookSubscription),
		eventCh:       make(chan Event, 100000),
		client:        &http.Client{Timeout: 3 * time.Second},
	}

	// Drain channel in background
	go func() {
		for range d.eventCh {
		}
	}()

	event := Event{
		Collection: "benchmark",
		Type:       EventInsert,
		PointID:    "123",
		Timestamp:  time.Now().UnixMilli(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Publish(event)
	}
}

func BenchmarkSubscribe(b *testing.B) {
	d := &Dispatcher{
		subscriptions: make(map[string][]WebhookSubscription),
		eventCh:       make(chan Event, 100),
		client:        &http.Client{Timeout: 3 * time.Second},
	}

	sub := WebhookSubscription{URL: "http://example.com/hook"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Subscribe("benchmark", sub)
	}
}
