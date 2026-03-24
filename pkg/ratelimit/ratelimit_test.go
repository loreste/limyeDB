package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestTokenBucket_Allow(t *testing.T) {
	tb := NewTokenBucket(10, 10) // 10 tokens, 10 per second

	// Should allow first 10 requests
	for i := 0; i < 10; i++ {
		if !tb.Allow() {
			t.Errorf("request %d should be allowed", i)
		}
	}

	// 11th request should be denied
	if tb.Allow() {
		t.Error("11th request should be denied")
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	tb := NewTokenBucket(5, 100) // 5 tokens, 100 per second

	// Use all tokens
	for i := 0; i < 5; i++ {
		tb.Allow()
	}

	// Wait for refill
	time.Sleep(60 * time.Millisecond)

	// Should have ~5 tokens refilled
	allowed := 0
	for i := 0; i < 10; i++ {
		if tb.Allow() {
			allowed++
		}
	}

	if allowed < 4 {
		t.Errorf("expected at least 4 tokens refilled, got %d", allowed)
	}
}

func TestRateLimiter_Allow(t *testing.T) {
	config := RateLimiterConfig{
		RequestsPerSecond: 10,
		BurstSize:         10,
		KeyFunc: func(r *http.Request) string {
			return r.RemoteAddr
		},
	}

	rl := NewRateLimiter(config)

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"

	// First 10 should pass
	for i := 0; i < 10; i++ {
		if !rl.Allow(req) {
			t.Errorf("request %d should be allowed", i)
		}
	}

	// 11th should be denied
	if rl.Allow(req) {
		t.Error("11th request should be denied")
	}

	// Different IP should be allowed
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "192.168.1.2:1234"
	if !rl.Allow(req2) {
		t.Error("request from different IP should be allowed")
	}
}

func TestRateLimiter_Cleanup(t *testing.T) {
	config := RateLimiterConfig{
		RequestsPerSecond: 10,
		BurstSize:         10,
		CleanupInterval:   50 * time.Millisecond,
		KeyFunc: func(r *http.Request) string {
			return r.RemoteAddr
		},
	}

	rl := NewRateLimiter(config)

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	rl.Allow(req)

	// Wait for cleanup
	time.Sleep(100 * time.Millisecond)

	// Bucket should be cleaned up and fresh one created
	if !rl.Allow(req) {
		t.Error("request should be allowed after cleanup")
	}

	rl.Stop()
}

func TestRateLimiter_Middleware(t *testing.T) {
	config := RateLimiterConfig{
		RequestsPerSecond: 2,
		BurstSize:         2,
		KeyFunc: func(r *http.Request) string {
			return r.RemoteAddr
		},
	}

	rl := NewRateLimiter(config)

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First 2 requests should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("request %d expected 200, got %d", i, rec.Code)
		}
	}

	// 3rd request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rec.Code)
	}
}

func TestRateLimiter_Concurrent(t *testing.T) {
	config := RateLimiterConfig{
		RequestsPerSecond: 100,
		BurstSize:         100,
		KeyFunc: func(r *http.Request) string {
			return r.RemoteAddr
		},
	}

	rl := NewRateLimiter(config)

	var wg sync.WaitGroup
	allowed := 0
	var mu sync.Mutex

	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = "192.168.1.1:1234"
			if rl.Allow(req) {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if allowed > 105 { // Some tolerance for timing
		t.Errorf("expected ~100 allowed, got %d", allowed)
	}
}

func BenchmarkRateLimiter_Allow(b *testing.B) {
	config := RateLimiterConfig{
		RequestsPerSecond: 1000000,
		BurstSize:         1000000,
		KeyFunc: func(r *http.Request) string {
			return r.RemoteAddr
		},
	}

	rl := NewRateLimiter(config)
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Allow(req)
	}
}
