package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestTokenBucket_Allow(t *testing.T) {
	// 10 tokens, refill rate 10 per second
	tb := NewLimiter(10, 10)

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
	// 5 tokens, refill 100 per sec
	tb := NewLimiter(5, 100)

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

func TestRateLimiter_Middleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	config := DefaultConfig()
	config.RequestsPerSecond = 2
	config.BurstSize = 2
	config.KeyFunc = func(c *gin.Context) string {
		return c.ClientIP()
	}

	handler := Middleware(config)

	// Test handler
	router := gin.New()
	router.Use(handler)
	router.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// First 2 requests should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:1234"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("request %d expected 200, got %d", i, rec.Code)
		}
	}

	// 3rd request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rec.Code)
	}
}

func TestRateLimiter_Concurrent(t *testing.T) {
	store := NewRateLimiterStore(100, 100, time.Minute)

	var wg sync.WaitGroup
	allowed := 0
	var mu sync.Mutex

	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			limiter := store.GetLimiter("192.168.1.1")
			if limiter.Allow() {
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
	store := NewRateLimiterStore(1000000, 1000000, time.Minute)
	limiter := store.GetLimiter("192.168.1.1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow()
	}
}
