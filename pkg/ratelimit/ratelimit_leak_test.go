package ratelimit

import (
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestRateLimiterStoreNoGoroutineLeak(t *testing.T) {
	defer goleak.VerifyNone(t,
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"),
	)

	// Use a short cleanup interval so the goroutine ticks quickly.
	store := NewRateLimiterStore(100, 10, 50*time.Millisecond)

	// Exercise the store with several keys.
	for i := 0; i < 20; i++ {
		key := "client-" + string(rune('A'+i))
		limiter := store.GetLimiter(key)
		limiter.Allow()
	}

	// Let the cleanup loop run at least once.
	time.Sleep(100 * time.Millisecond)

	// Stop should shut down the cleanup goroutine.
	store.Stop()

	// Allow goroutine to exit.
	time.Sleep(50 * time.Millisecond)
}

func TestRateLimiterStoreRepeatedCreateStop(t *testing.T) {
	defer goleak.VerifyNone(t,
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"),
	)

	// Create and stop multiple stores to verify no goroutine accumulation.
	for i := 0; i < 10; i++ {
		store := NewRateLimiterStore(50, 5, 30*time.Millisecond)
		store.GetLimiter("key").Allow()
		time.Sleep(40 * time.Millisecond)
		store.Stop()
	}

	// Small grace period for all goroutines to exit.
	time.Sleep(50 * time.Millisecond)
}
