package cache

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

// TestRaceCacheSetGet hammers Set and Get from many goroutines.
func TestRaceCacheSetGet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	c := NewCache(1000, 5*time.Minute)

	const goroutines = 20
	const opsPerGoroutine = 500

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				key := fmt.Sprintf("key-%d-%d", id, i)
				c.Set(key, fmt.Sprintf("value-%d-%d", id, i))

				// Read back a random key
				readKey := fmt.Sprintf("key-%d-%d", rand.Intn(goroutines), rand.Intn(opsPerGoroutine)) // #nosec G404
				_, _ = c.Get(readKey)
			}
		}(g)
	}
	wg.Wait()

	stats := c.Stats()
	if stats.Size <= 0 {
		t.Fatal("expected cache to have entries")
	}
}

// TestRaceCacheSetDeleteClear mixes Set, Delete, and Clear operations.
func TestRaceCacheSetDeleteClear(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	c := NewCache(500, 5*time.Minute)

	const goroutines = 10
	const ops = 500

	var wg sync.WaitGroup

	// Setters
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				c.Set(fmt.Sprintf("item-%d-%d", id, i), i)
			}
		}(g)
	}

	// Deleters
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				c.Delete(fmt.Sprintf("item-%d-%d", id, i))
			}
		}(g)
	}

	// Clearers (less frequent)
	wg.Add(3)
	for g := 0; g < 3; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				c.Clear()
			}
		}()
	}

	// Stats readers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				_ = c.Stats()
			}
		}()
	}

	wg.Wait()
}

// TestRaceSearchCache hammers the SearchCache concurrently.
func TestRaceSearchCache(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	sc := NewSearchCache(500, 5*time.Minute)

	const goroutines = 10
	const ops = 300

	var wg sync.WaitGroup

	// Setters
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				vec := make([]float32, 8)
				for v := range vec {
					vec[v] = rand.Float32() // #nosec G404
				}
				key := sc.SearchKey("testcoll", vec, 10, nil)
				sc.Set(key, fmt.Sprintf("result-%d-%d", id, i))
			}
		}(g)
	}

	// Getters
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				vec := make([]float32, 8)
				for v := range vec {
					vec[v] = rand.Float32() // #nosec G404
				}
				key := sc.SearchKey("testcoll", vec, 10, nil)
				_, _ = sc.Get(key)
			}
		}(g)
	}

	// Invalidators
	wg.Add(5)
	for g := 0; g < 5; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				sc.InvalidateCollection("testcoll")
			}
		}()
	}

	// Stats readers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				_ = sc.Stats()
			}
		}()
	}

	wg.Wait()
}

// TestRaceSemanticCache hammers the SemanticCache including FindSimilar.
func TestRaceSemanticCache(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	sc := NewSemanticCache(200, 5*time.Minute, 0.9)

	const goroutines = 10
	const ops = 200
	const dim = 8

	var wg sync.WaitGroup

	// Setters
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				vec := make([]float32, dim)
				for v := range vec {
					vec[v] = rand.Float32() // #nosec G404
				}
				key := fmt.Sprintf("sem-%d-%d", id, i)
				sc.Set(key, vec, fmt.Sprintf("result-%d-%d", id, i))
			}
		}(g)
	}

	// FindSimilar readers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				vec := make([]float32, dim)
				for v := range vec {
					vec[v] = rand.Float32() // #nosec G404
				}
				_, _, _ = sc.FindSimilar(vec)
			}
		}()
	}

	wg.Wait()
}

// TestRaceCacheEviction stresses cache eviction under concurrent load.
func TestRaceCacheEviction(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	// Small capacity to force lots of eviction
	c := NewCache(50, 5*time.Minute)

	const goroutines = 20
	const ops = 1000

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				key := fmt.Sprintf("ev-%d-%d", id, i)
				c.Set(key, i)
				_, _ = c.Get(key)
			}
		}(g)
	}

	wg.Wait()

	stats := c.Stats()
	if stats.Size > 50 {
		t.Fatalf("expected cache size <= 50, got %d", stats.Size)
	}
}
