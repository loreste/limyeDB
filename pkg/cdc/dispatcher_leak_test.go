package cdc

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// TestDispatcherNoGoroutineLeak verifies that the dispatcher doesn't leak goroutines
func TestDispatcherNoGoroutineLeak(t *testing.T) {
	// Get initial goroutine count
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()

	// Create a short-lived server
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))

	// Create dispatcher
	d := &Dispatcher{
		subscriptions: make(map[string][]WebhookSubscription),
		eventCh:       make(chan Event, 100),
		client:        &http.Client{Timeout: 1 * time.Second},
	}

	// Start worker
	go d.worker()

	// Subscribe
	d.Subscribe("leak_test", WebhookSubscription{URL: server.URL})

	// Publish many events
	for i := 0; i < 100; i++ {
		d.Publish(Event{
			Collection: "leak_test",
			Type:       EventInsert,
			PointID:    "test",
			Timestamp:  time.Now().UnixMilli(),
		})
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Close server and drain
	server.Close()
	close(d.eventCh)

	// Wait for goroutines to settle
	time.Sleep(500 * time.Millisecond)
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	// Check goroutine count - allow for some variance
	finalGoroutines := runtime.NumGoroutine()

	// Allow up to 5 additional goroutines (test framework, etc.)
	if finalGoroutines > initialGoroutines+5 {
		t.Errorf("Potential goroutine leak: started with %d, ended with %d", initialGoroutines, finalGoroutines)
	}

	if requestCount.Load() < 50 {
		t.Errorf("Expected at least 50 requests, got %d", requestCount.Load())
	}
}

// TestDispatcherMemoryStability tests that repeated operations don't cause memory leaks
func TestDispatcherMemoryStability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory stability test in short mode")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	d := &Dispatcher{
		subscriptions: make(map[string][]WebhookSubscription),
		eventCh:       make(chan Event, 1000),
		client:        &http.Client{Timeout: 1 * time.Second},
	}

	go d.worker()
	d.Subscribe("memory_test", WebhookSubscription{URL: server.URL})

	// Get initial memory stats
	runtime.GC()
	var initialMem runtime.MemStats
	runtime.ReadMemStats(&initialMem)

	// Publish many events in batches
	for batch := 0; batch < 10; batch++ {
		for i := 0; i < 1000; i++ {
			d.Publish(Event{
				Collection: "memory_test",
				Type:       EventInsert,
				PointID:    "test",
				Timestamp:  time.Now().UnixMilli(),
			})
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Force GC and check memory
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	var finalMem runtime.MemStats
	runtime.ReadMemStats(&finalMem)

	// Allow for some memory growth but detect large leaks
	// Memory should not grow by more than 50MB for this test
	memGrowth := finalMem.HeapAlloc - initialMem.HeapAlloc
	maxAllowedGrowth := uint64(50 * 1024 * 1024) // 50MB

	if memGrowth > maxAllowedGrowth {
		t.Errorf("Potential memory leak: heap grew by %d bytes (%.2f MB)", memGrowth, float64(memGrowth)/(1024*1024))
	}
}

// BenchmarkDispatcherThroughput measures dispatcher performance
func BenchmarkDispatcherThroughput(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	d := &Dispatcher{
		subscriptions: make(map[string][]WebhookSubscription),
		eventCh:       make(chan Event, 10000),
		client:        &http.Client{Timeout: 5 * time.Second},
	}

	go d.worker()
	d.Subscribe("benchmark", WebhookSubscription{URL: server.URL})

	event := Event{
		Collection: "benchmark",
		Type:       EventInsert,
		PointID:    "test",
		Timestamp:  time.Now().UnixMilli(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Publish(event)
	}
}

// BenchmarkDispatcherConcurrent measures concurrent publishing
func BenchmarkDispatcherConcurrent(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	d := &Dispatcher{
		subscriptions: make(map[string][]WebhookSubscription),
		eventCh:       make(chan Event, 10000),
		client:        &http.Client{Timeout: 5 * time.Second},
	}

	go d.worker()
	d.Subscribe("concurrent_bench", WebhookSubscription{URL: server.URL})

	event := Event{
		Collection: "concurrent_bench",
		Type:       EventInsert,
		PointID:    "test",
		Timestamp:  time.Now().UnixMilli(),
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			d.Publish(event)
		}
	})
}
