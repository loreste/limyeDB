package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestManager_Subscribe(t *testing.T) {
	m := NewManager(2, DefaultRetryPolicy())
	defer m.Close()

	sub := &Subscription{
		ID:     "test-webhook",
		URL:    "http://example.com/webhook",
		Events: []EventType{EventPointInsert, EventPointDelete},
		Secret: "test-secret",
	}

	err := m.Subscribe(sub)
	if err != nil {
		t.Errorf("failed to subscribe: %v", err)
	}

	subs := m.ListSubscriptions()
	if len(subs) != 1 {
		t.Errorf("expected 1 subscription, got %d", len(subs))
	}
}

func TestManager_Unsubscribe(t *testing.T) {
	m := NewManager(2, DefaultRetryPolicy())
	defer m.Close()

	sub := &Subscription{
		ID:     "test-webhook",
		URL:    "http://example.com/webhook",
		Events: []EventType{EventPointInsert},
	}

	m.Subscribe(sub)
	err := m.Unsubscribe("test-webhook")
	if err != nil {
		t.Errorf("failed to unsubscribe: %v", err)
	}

	subs := m.ListSubscriptions()
	if len(subs) != 0 {
		t.Error("expected no subscriptions after unsubscribe")
	}
}

func TestManager_Emit(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	m := NewManager(2, DefaultRetryPolicy())
	defer m.Close()

	sub := &Subscription{
		ID:     "test-webhook",
		URL:    server.URL,
		Events: []EventType{EventPointInsert},
	}

	m.Subscribe(sub)

	event := &Event{
		Type:      EventPointInsert,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"point_id": "123",
		},
	}

	m.Emit(event)

	// Wait for async delivery
	time.Sleep(200 * time.Millisecond)

	if received.Load() != 1 {
		t.Errorf("expected 1 webhook call, got %d", received.Load())
	}
}

func TestManager_EventFiltering(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	m := NewManager(2, DefaultRetryPolicy())
	defer m.Close()

	sub := &Subscription{
		ID:     "test-webhook",
		URL:    server.URL,
		Events: []EventType{EventPointInsert}, // Only subscribes to insert
	}

	m.Subscribe(sub)

	// Emit delete event - should not trigger webhook
	event := &Event{
		Type:      EventPointDelete,
		Timestamp: time.Now(),
	}

	m.Emit(event)

	time.Sleep(100 * time.Millisecond)

	if received.Load() != 0 {
		t.Errorf("expected 0 webhook calls for unsubscribed event, got %d", received.Load())
	}
}

func TestManager_HMAC(t *testing.T) {
	var mu sync.Mutex
	var receivedSignature string
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedSignature = r.Header.Get("X-LimyeDB-Signature")
		receivedBody, _ = io.ReadAll(r.Body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	m := NewManager(2, DefaultRetryPolicy())
	defer m.Close()

	secret := "my-secret-key"
	sub := &Subscription{
		ID:     "test-webhook",
		URL:    server.URL,
		Events: []EventType{EventPointInsert},
		Secret: secret,
	}

	m.Subscribe(sub)

	event := &Event{
		Type:      EventPointInsert,
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"test": "data"},
	}

	m.Emit(event)

	time.Sleep(200 * time.Millisecond)

	// Verify signature
	mu.Lock()
	body := make([]byte, len(receivedBody))
	copy(body, receivedBody)
	sig := receivedSignature
	mu.Unlock()

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expectedSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if sig != expectedSig {
		t.Errorf("signature mismatch: expected %s, got %s", expectedSig, sig)
	}
}

func TestManager_Retry(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := attempts.Add(1)
		if count < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	policy := RetryPolicy{
		MaxRetries:    3,
		InitialDelay:  10 * time.Millisecond,
		MaxDelay:      100 * time.Millisecond,
		BackoffFactor: 2.0,
	}

	m := NewManager(2, policy)
	defer m.Close()

	sub := &Subscription{
		ID:     "test-webhook",
		URL:    server.URL,
		Events: []EventType{EventPointInsert},
	}

	m.Subscribe(sub)

	event := &Event{
		Type:      EventPointInsert,
		Timestamp: time.Now(),
	}

	m.Emit(event)

	time.Sleep(500 * time.Millisecond)

	if attempts.Load() < 2 {
		t.Errorf("expected at least 2 attempts with retry, got %d", attempts.Load())
	}
}

func TestManager_Payload(t *testing.T) {
	var mu sync.Mutex
	var receivedPayload Event
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		json.Unmarshal(body, &receivedPayload)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	m := NewManager(2, DefaultRetryPolicy())
	defer m.Close()

	sub := &Subscription{
		ID:     "test-webhook",
		URL:    server.URL,
		Events: []EventType{EventCollectionCreate},
	}

	m.Subscribe(sub)

	event := &Event{
		Type:      EventCollectionCreate,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"name":      "new_collection",
			"dimension": 128,
		},
	}

	m.Emit(event)

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	eventType := receivedPayload.Type
	mu.Unlock()

	if eventType != EventCollectionCreate {
		t.Errorf("expected event type %s, got %s", EventCollectionCreate, eventType)
	}
}

func TestManager_GetSubscription(t *testing.T) {
	m := NewManager(2, DefaultRetryPolicy())
	defer m.Close()

	sub := &Subscription{
		ID:     "test-webhook",
		URL:    "http://example.com/webhook",
		Events: []EventType{EventPointInsert},
	}

	m.Subscribe(sub)

	found, err := m.GetSubscription("test-webhook")
	if err != nil {
		t.Errorf("failed to get subscription: %v", err)
	}
	if found.URL != sub.URL {
		t.Errorf("expected URL %s, got %s", sub.URL, found.URL)
	}

	_, err = m.GetSubscription("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent subscription")
	}
}

func TestManager_CollectionFilter(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	m := NewManager(2, DefaultRetryPolicy())
	defer m.Close()

	sub := &Subscription{
		ID:         "test-webhook",
		URL:        server.URL,
		Events:     []EventType{EventPointInsert},
		Collection: "my_collection", // Only this collection
	}

	m.Subscribe(sub)

	// Event for different collection - should not trigger
	event1 := &Event{
		Type:      EventPointInsert,
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"collection": "other_collection"},
	}
	m.Emit(event1)

	// Event for matching collection - should trigger
	event2 := &Event{
		Type:      EventPointInsert,
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"collection": "my_collection"},
	}
	m.Emit(event2)

	time.Sleep(200 * time.Millisecond)

	if received.Load() != 1 {
		t.Errorf("expected 1 webhook call for matching collection, got %d", received.Load())
	}
}

func TestManager_EmitHelpers(t *testing.T) {
	var mu sync.Mutex
	var receivedEvents []Event
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var event Event
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &event)
		mu.Lock()
		receivedEvents = append(receivedEvents, event)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	m := NewManager(2, DefaultRetryPolicy())
	defer m.Close()

	sub := &Subscription{
		ID:     "test-webhook",
		URL:    server.URL,
		Events: []EventType{EventPointInsert, EventPointDelete, EventCollectionCreate, EventCollectionDelete},
	}

	m.Subscribe(sub)

	m.EmitPointInsert("col1", "p1", map[string]interface{}{"key": "value"})
	m.EmitPointDelete("col1", "p1")
	m.EmitCollectionCreate("col1", 128)
	m.EmitCollectionDelete("col1")

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	numEvents := len(receivedEvents)
	mu.Unlock()

	if numEvents < 4 {
		t.Errorf("expected 4 events, got %d", numEvents)
	}
}

func BenchmarkManager_Emit(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	m := NewManager(4, DefaultRetryPolicy())
	defer m.Close()

	sub := &Subscription{
		ID:     "test-webhook",
		URL:    server.URL,
		Events: []EventType{EventPointInsert},
	}

	m.Subscribe(sub)

	event := &Event{
		Type:      EventPointInsert,
		Timestamp: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Emit(event)
	}
}
