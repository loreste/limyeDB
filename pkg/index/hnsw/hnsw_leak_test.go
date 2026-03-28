package hnsw

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"

	"go.uber.org/goleak"

	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/point"
)

func TestHNSWConcurrentNoGoroutineLeak(t *testing.T) {
	defer goleak.VerifyNone(t,
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"),
	)

	dim := 32
	idx, err := New(&Config{
		M:              16,
		EfConstruction: 100,
		EfSearch:       50,
		MaxElements:    5000,
		Metric:         config.MetricCosine,
		Dimension:      dim,
	})
	if err != nil {
		t.Fatalf("failed to create HNSW index: %v", err)
	}

	rng := rand.New(rand.NewSource(42))

	// Insert a base set of points sequentially to build the graph.
	for i := 0; i < 200; i++ {
		vec := randomVector(rng, dim)
		p := &point.Point{
			ID:     fmt.Sprintf("base-%d", i),
			Vector: vec,
		}
		if err := idx.Insert(p); err != nil {
			t.Fatalf("base insert %d failed: %v", i, err)
		}
	}

	// Concurrently insert and search.
	var wg sync.WaitGroup
	const writers = 4
	const readers = 8
	const opsPerWorker = 50

	// Concurrent inserts.
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			localRng := rand.New(rand.NewSource(int64(workerID) * 1000))
			for i := 0; i < opsPerWorker; i++ {
				vec := randomVector(localRng, dim)
				p := &point.Point{
					ID:     fmt.Sprintf("w%d-%d", workerID, i),
					Vector: vec,
				}
				// Insert may fail with ErrPointExists on collision; that's fine.
				_ = idx.Insert(p)
			}
		}(w)
	}

	// Concurrent searches.
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			localRng := rand.New(rand.NewSource(int64(readerID) * 2000))
			for i := 0; i < opsPerWorker; i++ {
				query := randomVector(localRng, dim)
				_, _ = idx.Search(query, 5)
			}
		}(r)
	}

	wg.Wait()

	// Verify the index is in a consistent state.
	size := idx.Size()
	if size == 0 {
		t.Error("expected non-zero index size after concurrent inserts")
	}
	t.Logf("index size after concurrent operations: %d", size)
}

func TestHNSWBatchSearchNoGoroutineLeak(t *testing.T) {
	defer goleak.VerifyNone(t,
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"),
	)

	dim := 16
	idx, err := New(&Config{
		M:              8,
		EfConstruction: 50,
		EfSearch:       30,
		MaxElements:    1000,
		Metric:         config.MetricCosine,
		Dimension:      dim,
	})
	if err != nil {
		t.Fatalf("failed to create HNSW index: %v", err)
	}

	rng := rand.New(rand.NewSource(99))

	// Build a small index.
	for i := 0; i < 100; i++ {
		vec := randomVector(rng, dim)
		if err := idx.Insert(&point.Point{ID: fmt.Sprintf("p-%d", i), Vector: vec}); err != nil {
			t.Fatalf("insert failed: %v", err)
		}
	}

	// Run batch searches which internally spawn goroutines.
	queries := make([]point.Vector, 20)
	for i := range queries {
		queries[i] = randomVector(rng, dim)
	}

	results, err := idx.BatchSearch(queries, 5)
	if err != nil {
		t.Fatalf("batch search failed: %v", err)
	}

	if len(results) != len(queries) {
		t.Errorf("expected %d result sets, got %d", len(queries), len(results))
	}
}

// randomVector creates a random float32 vector of the given dimension.
func randomVector(rng *rand.Rand, dim int) point.Vector {
	vec := make(point.Vector, dim)
	for i := range vec {
		vec[i] = rng.Float32()
	}
	return vec
}
