package cache

import (
	"testing"
	"time"
)

func TestCache_SetGet(t *testing.T) {
	c := NewCache(10, time.Minute)

	c.Set("key1", "value1")
	c.Set("key2", 42)

	val, ok := c.Get("key1")
	if !ok || val != "value1" {
		t.Errorf("expected 'value1', got %v", val)
	}

	val, ok = c.Get("key2")
	if !ok || val != 42 {
		t.Errorf("expected 42, got %v", val)
	}

	_, ok = c.Get("nonexistent")
	if ok {
		t.Error("expected not found for nonexistent key")
	}
}

func TestCache_Expiration(t *testing.T) {
	c := NewCache(10, 50*time.Millisecond)

	c.Set("key1", "value1")

	val, ok := c.Get("key1")
	if !ok || val != "value1" {
		t.Errorf("expected 'value1', got %v", val)
	}

	time.Sleep(100 * time.Millisecond)

	_, ok = c.Get("key1")
	if ok {
		t.Error("expected key to be expired")
	}
}

func TestCache_Eviction(t *testing.T) {
	c := NewCache(3, time.Minute)

	c.Set("key1", "value1")
	c.Set("key2", "value2")
	c.Set("key3", "value3")

	// Access key1 to make it recently used
	c.Get("key1")

	// Add key4, should evict key2 (oldest)
	c.Set("key4", "value4")

	_, ok := c.Get("key2")
	if ok {
		t.Error("expected key2 to be evicted")
	}

	_, ok = c.Get("key1")
	if !ok {
		t.Error("expected key1 to still exist")
	}
}

func TestCache_Delete(t *testing.T) {
	c := NewCache(10, time.Minute)

	c.Set("key1", "value1")
	c.Delete("key1")

	_, ok := c.Get("key1")
	if ok {
		t.Error("expected key1 to be deleted")
	}
}

func TestCache_Clear(t *testing.T) {
	c := NewCache(10, time.Minute)

	c.Set("key1", "value1")
	c.Set("key2", "value2")
	c.Clear()

	stats := c.Stats()
	if stats.Size != 0 {
		t.Errorf("expected size 0, got %d", stats.Size)
	}
}

func TestCache_Stats(t *testing.T) {
	c := NewCache(10, time.Minute)

	c.Set("key1", "value1")
	c.Get("key1") // hit
	c.Get("key1") // hit
	c.Get("key2") // miss

	stats := c.Stats()
	if stats.Hits != 2 {
		t.Errorf("expected 2 hits, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}
	if stats.HitRate < 0.66 || stats.HitRate > 0.67 {
		t.Errorf("expected hit rate ~0.67, got %f", stats.HitRate)
	}
}

func TestSearchCache(t *testing.T) {
	sc := NewSearchCache(100, time.Minute)

	vector := []float32{0.1, 0.2, 0.3}
	results := []string{"result1", "result2"}

	key := sc.SearchKey("test_collection", vector, 10, nil)
	sc.Set(key, results)

	val, ok := sc.Get(key)
	if !ok {
		t.Error("expected to find cached results")
	}

	cachedResults, ok := val.([]string)
	if !ok || len(cachedResults) != 2 {
		t.Error("unexpected cached results")
	}
}

func TestSemanticCache(t *testing.T) {
	sc := NewSemanticCache(100, time.Minute, 0.95)

	vector1 := []float32{1.0, 0.0, 0.0}
	vector2 := []float32{0.99, 0.01, 0.0} // Very similar to vector1
	vector3 := []float32{0.0, 1.0, 0.0}   // Different from vector1

	sc.Set("key1", vector1, "result1")

	// Should find similar vector
	_, result, found := sc.FindSimilar(vector2)
	if !found {
		t.Error("expected to find similar cached result")
	}
	if result != "result1" {
		t.Errorf("expected 'result1', got %v", result)
	}

	// Should not find dissimilar vector
	_, _, found = sc.FindSimilar(vector3)
	if found {
		t.Error("should not find dissimilar vector")
	}
}

func TestCosineSimilarity(t *testing.T) {
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{1.0, 0.0, 0.0}

	sim := cosineSimilarity(a, b)
	if sim < 0.99 || sim > 1.01 {
		t.Errorf("expected similarity ~1.0, got %f", sim)
	}

	c := []float32{0.0, 1.0, 0.0}
	sim = cosineSimilarity(a, c)
	if sim < -0.01 || sim > 0.01 {
		t.Errorf("expected similarity ~0.0, got %f", sim)
	}
}

func BenchmarkCache_Set(b *testing.B) {
	c := NewCache(10000, time.Minute)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		c.Set("key", "value")
	}
}

func BenchmarkCache_Get(b *testing.B) {
	c := NewCache(10000, time.Minute)
	c.Set("key", "value")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		c.Get("key")
	}
}
