package cache

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestCacheNoGoroutineLeak(t *testing.T) {
	// The cache package does not spawn goroutines, but this test confirms that
	// creating, using, and discarding a cache does not leak goroutines.
	defer goleak.VerifyNone(t,
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"),
	)

	c := NewCache(100, 500*time.Millisecond)

	// Fill and read back.
	for i := 0; i < 200; i++ {
		key := fmt.Sprintf("key-%d", i)
		c.Set(key, i)
	}

	for i := 0; i < 200; i++ {
		key := fmt.Sprintf("key-%d", i)
		c.Get(key)
	}

	stats := c.Stats()
	if stats.Size == 0 {
		t.Error("expected non-zero cache size")
	}

	// Clear the cache and let GC run.
	c.Clear()
	runtime.GC()
}

func TestCacheMemoryStability(t *testing.T) {
	// Verify that repeatedly filling and clearing the cache does not cause
	// unbounded heap growth.
	var baseline runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&baseline)

	c := NewCache(1000, time.Minute)

	for round := 0; round < 10; round++ {
		for i := 0; i < 1000; i++ {
			key := fmt.Sprintf("round-%d-key-%d", round, i)
			c.Set(key, make([]byte, 1024)) // 1KB values
		}
		c.Clear()
	}

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	// Heap in-use should not grow by more than 10MB from baseline after clearing.
	heapGrowth := int64(after.HeapInuse) - int64(baseline.HeapInuse)
	const maxGrowth = 10 * 1024 * 1024 // 10 MB
	if heapGrowth > maxGrowth {
		t.Errorf("excessive heap growth after cache clear cycles: grew %d bytes (limit %d)",
			heapGrowth, maxGrowth)
	}
}

func TestSemanticCacheNoGoroutineLeak(t *testing.T) {
	defer goleak.VerifyNone(t,
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"),
	)

	sc := NewSemanticCache(50, time.Second, 0.9)

	vec1 := []float32{1.0, 0.0, 0.0}
	vec2 := []float32{0.0, 1.0, 0.0}
	vec3 := []float32{0.99, 0.01, 0.0}

	sc.Set("key1", vec1, "result1")
	sc.Set("key2", vec2, "result2")

	// vec3 is very similar to vec1.
	_, result, found := sc.FindSimilar(vec3)
	if found {
		t.Logf("found similar entry: %v", result)
	}

	runtime.GC()
}
