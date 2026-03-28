package hnsw

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"

	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/point"
)

func testHNSWConfig(dim int) *Config {
	return &Config{
		M:              16,
		EfConstruction: 100,
		EfSearch:       50,
		MaxElements:    50000,
		Metric:         config.MetricEuclidean,
		Dimension:      dim,
	}
}

func randVec(dim int) point.Vector {
	v := make(point.Vector, dim)
	for i := range v {
		v[i] = rand.Float32() // #nosec G404 - test-only RNG
	}
	return v
}

// TestRaceConcurrentInserts hammers Insert from many goroutines.
func TestRaceConcurrentInserts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	const dim = 32
	h, err := New(testHNSWConfig(dim))
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 10
	const opsPerGoroutine = 500

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				p := &point.Point{
					ID:      fmt.Sprintf("n%d-%d", id, i),
					Vector:  randVec(dim),
					Payload: map[string]interface{}{"g": id},
				}
				_ = h.Insert(p)
			}
		}(g)
	}
	wg.Wait()

	total := h.Size()
	if total <= 0 {
		t.Fatalf("expected positive size, got %d", total)
	}
	t.Logf("inserted %d points from %d goroutines", total, goroutines)
}

// TestRaceConcurrentSearchWhileInsert searches while inserts are happening.
func TestRaceConcurrentSearchWhileInsert(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	const dim = 16
	h, err := New(testHNSWConfig(dim))
	if err != nil {
		t.Fatal(err)
	}

	// Seed initial data so search has something to traverse
	for i := 0; i < 100; i++ {
		p := &point.Point{
			ID:     fmt.Sprintf("seed-%d", i),
			Vector: randVec(dim),
		}
		if insertErr := h.Insert(p); insertErr != nil {
			t.Fatal(insertErr)
		}
	}

	const goroutines = 10
	const ops = 500

	var wg sync.WaitGroup

	// Inserters
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				p := &point.Point{
					ID:     fmt.Sprintf("i%d-%d", id, i),
					Vector: randVec(dim),
				}
				_ = h.Insert(p)
			}
		}(g)
	}

	// Searchers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				q := randVec(dim)
				_, _ = h.Search(q, 5)
			}
		}()
	}

	wg.Wait()
}

// TestRaceDeleteWhileSearch runs deletions concurrently with searches.
func TestRaceDeleteWhileSearch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	const dim = 16
	const numPoints = 1000
	h, err := New(testHNSWConfig(dim))
	if err != nil {
		t.Fatal(err)
	}

	// Populate index
	for i := 0; i < numPoints; i++ {
		p := &point.Point{
			ID:     fmt.Sprintf("d-%d", i),
			Vector: randVec(dim),
		}
		if insertErr := h.Insert(p); insertErr != nil {
			t.Fatal(insertErr)
		}
	}

	const searchGoroutines = 10
	const deleteGoroutines = 5
	const searchOps = 300

	var wg sync.WaitGroup

	// Searchers
	wg.Add(searchGoroutines)
	for g := 0; g < searchGoroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < searchOps; i++ {
				q := randVec(dim)
				_, _ = h.Search(q, 10)
			}
		}()
	}

	// Deleters
	wg.Add(deleteGoroutines)
	pointsPerDeleter := numPoints / deleteGoroutines
	for g := 0; g < deleteGoroutines; g++ {
		go func(id int) {
			defer wg.Done()
			start := id * pointsPerDeleter
			end := start + pointsPerDeleter
			for i := start; i < end; i++ {
				_ = h.Delete(fmt.Sprintf("d-%d", i))
			}
		}(g)
	}

	wg.Wait()
}

// TestRaceGetAndInsert runs Get and Insert concurrently.
func TestRaceGetAndInsert(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	const dim = 8
	h, err := New(testHNSWConfig(dim))
	if err != nil {
		t.Fatal(err)
	}

	// Pre-populate
	for i := 0; i < 200; i++ {
		p := &point.Point{
			ID:     fmt.Sprintf("pre-%d", i),
			Vector: randVec(dim),
		}
		_ = h.Insert(p)
	}

	const goroutines = 10
	const ops = 300

	var wg sync.WaitGroup

	// Getters
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				_, _ = h.Get(fmt.Sprintf("pre-%d", i%200))
			}
		}()
	}

	// Inserters
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				p := &point.Point{
					ID:     fmt.Sprintf("new-%d-%d", id, i),
					Vector: randVec(dim),
				}
				_ = h.Insert(p)
			}
		}(g)
	}

	wg.Wait()
}

// TestRaceAllOperations runs Insert, Search, Delete, Get all at once.
func TestRaceAllOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	const dim = 16
	h, err := New(testHNSWConfig(dim))
	if err != nil {
		t.Fatal(err)
	}

	// Pre-populate
	for i := 0; i < 200; i++ {
		p := &point.Point{
			ID:     fmt.Sprintf("all-%d", i),
			Vector: randVec(dim),
		}
		_ = h.Insert(p)
	}

	const ops = 200
	var wg sync.WaitGroup

	// Inserters (10)
	wg.Add(10)
	for g := 0; g < 10; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				p := &point.Point{
					ID:     fmt.Sprintf("all-ins-%d-%d", id, i),
					Vector: randVec(dim),
				}
				_ = h.Insert(p)
			}
		}(g)
	}

	// Searchers (10)
	wg.Add(10)
	for g := 0; g < 10; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				_, _ = h.Search(randVec(dim), 5)
			}
		}()
	}

	// Deleters (5)
	wg.Add(5)
	for g := 0; g < 5; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				_ = h.Delete(fmt.Sprintf("all-%d", id*40+i))
			}
		}(g)
	}

	// Getters (5)
	wg.Add(5)
	for g := 0; g < 5; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				_, _ = h.Get(fmt.Sprintf("all-%d", i%200))
				_ = h.Size()
			}
		}()
	}

	wg.Wait()
}

// TestRaceBatchSearch runs BatchSearch concurrently with inserts.
func TestRaceBatchSearch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	const dim = 16
	h, err := New(testHNSWConfig(dim))
	if err != nil {
		t.Fatal(err)
	}

	// Pre-populate
	for i := 0; i < 200; i++ {
		p := &point.Point{
			ID:     fmt.Sprintf("bs-%d", i),
			Vector: randVec(dim),
		}
		_ = h.Insert(p)
	}

	const goroutines = 10
	const ops = 100
	var wg sync.WaitGroup

	// Batch searchers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				queries := make([]point.Vector, 5)
				for q := range queries {
					queries[q] = randVec(dim)
				}
				_, _ = h.BatchSearch(queries, 3)
			}
		}()
	}

	// Concurrent inserters
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				p := &point.Point{
					ID:     fmt.Sprintf("bsi-%d-%d", id, i),
					Vector: randVec(dim),
				}
				_ = h.Insert(p)
			}
		}(g)
	}

	wg.Wait()
}
