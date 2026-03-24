// Package cache provides query result caching for LimyeDB.
package cache

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

// Cache is a thread-safe LRU cache for query results.
type Cache struct {
	mu       sync.RWMutex
	capacity int
	ttl      time.Duration
	items    map[string]*list.Element
	order    *list.List
	hits     int64
	misses   int64
}

// CacheEntry represents a cached item.
type CacheEntry struct {
	Key       string
	Value     interface{}
	ExpiresAt time.Time
}

// NewCache creates a new cache with the given capacity and TTL.
func NewCache(capacity int, ttl time.Duration) *Cache {
	return &Cache{
		capacity: capacity,
		ttl:      ttl,
		items:    make(map[string]*list.Element),
		order:    list.New(),
	}
}

// Get retrieves a value from the cache.
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		entry := elem.Value.(*CacheEntry)
		if time.Now().Before(entry.ExpiresAt) {
			c.order.MoveToFront(elem)
			c.hits++
			return entry.Value, true
		}
		// Expired, remove it
		c.removeElement(elem)
	}
	c.misses++
	return nil, false
}

// Set stores a value in the cache.
func (c *Cache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.order.MoveToFront(elem)
		entry := elem.Value.(*CacheEntry)
		entry.Value = value
		entry.ExpiresAt = time.Now().Add(c.ttl)
		return
	}

	// Evict if at capacity
	for c.order.Len() >= c.capacity {
		c.removeOldest()
	}

	entry := &CacheEntry{
		Key:       key,
		Value:     value,
		ExpiresAt: time.Now().Add(c.ttl),
	}
	elem := c.order.PushFront(entry)
	c.items[key] = elem
}

// Delete removes a key from the cache.
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.removeElement(elem)
	}
}

// Clear removes all items from the cache.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.order.Init()
}

// Stats returns cache statistics.
func (c *Cache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}

	return CacheStats{
		Size:    c.order.Len(),
		Hits:    c.hits,
		Misses:  c.misses,
		HitRate: hitRate,
	}
}

// CacheStats holds cache statistics.
type CacheStats struct {
	Size    int
	Hits    int64
	Misses  int64
	HitRate float64
}

func (c *Cache) removeElement(elem *list.Element) {
	c.order.Remove(elem)
	entry := elem.Value.(*CacheEntry)
	delete(c.items, entry.Key)
}

func (c *Cache) removeOldest() {
	elem := c.order.Back()
	if elem != nil {
		c.removeElement(elem)
	}
}

// SearchCache provides caching specifically for search queries.
type SearchCache struct {
	cache *Cache
}

// NewSearchCache creates a new search cache.
func NewSearchCache(capacity int, ttl time.Duration) *SearchCache {
	return &SearchCache{
		cache: NewCache(capacity, ttl),
	}
}

// SearchKey generates a cache key for a search query.
func (sc *SearchCache) SearchKey(collection string, vector []float32, k int, filter interface{}) string {
	data := struct {
		Collection string
		Vector     []float32
		K          int
		Filter     interface{}
	}{collection, vector, k, filter}

	bytes, _ := json.Marshal(data)
	hash := sha256.Sum256(bytes)
	return hex.EncodeToString(hash[:])
}

// Get retrieves cached search results.
func (sc *SearchCache) Get(key string) (interface{}, bool) {
	return sc.cache.Get(key)
}

// Set stores search results in cache.
func (sc *SearchCache) Set(key string, results interface{}) {
	sc.cache.Set(key, results)
}

// InvalidateCollection removes all cache entries for a collection.
func (sc *SearchCache) InvalidateCollection(collection string) {
	// For simplicity, clear all cache
	// A more sophisticated implementation would track keys by collection
	sc.cache.Clear()
}

// Stats returns cache statistics.
func (sc *SearchCache) Stats() CacheStats {
	return sc.cache.Stats()
}

// SemanticCache caches queries based on semantic similarity.
type SemanticCache struct {
	cache           *Cache
	similarityThreshold float32
	vectors         map[string][]float32
	mu              sync.RWMutex
}

// NewSemanticCache creates a semantic cache.
func NewSemanticCache(capacity int, ttl time.Duration, threshold float32) *SemanticCache {
	return &SemanticCache{
		cache:              NewCache(capacity, ttl),
		similarityThreshold: threshold,
		vectors:            make(map[string][]float32),
	}
}

// FindSimilar finds a cached result with a similar query vector.
func (sc *SemanticCache) FindSimilar(vector []float32) (string, interface{}, bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	for key, cachedVector := range sc.vectors {
		similarity := cosineSimilarity(vector, cachedVector)
		if similarity >= sc.similarityThreshold {
			if result, ok := sc.cache.Get(key); ok {
				return key, result, true
			}
		}
	}
	return "", nil, false
}

// Set stores a result with its query vector.
func (sc *SemanticCache) Set(key string, vector []float32, result interface{}) {
	sc.mu.Lock()
	sc.vectors[key] = vector
	sc.mu.Unlock()

	sc.cache.Set(key, result)
}

// cosineSimilarity calculates cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float32
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (sqrt(normA) * sqrt(normB))
}

func sqrt(x float32) float32 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}
