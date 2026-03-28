package collection

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"

	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/point"
)

func makeCollectionConfig(name string, dim int) *config.CollectionConfig {
	return &config.CollectionConfig{
		Name:      name,
		Dimension: dim,
		Metric:    config.MetricEuclidean,
		HNSW: config.HNSWConfig{
			M:              16,
			EfConstruction: 100,
			EfSearch:       50,
			MaxElements:    10000,
		},
	}
}

func randomVector(dim int) point.Vector {
	v := make(point.Vector, dim)
	for i := range v {
		v[i] = rand.Float32() // #nosec G404 - test-only RNG
	}
	return v
}

// TestRaceManagerCreateDelete hammers Create and Delete concurrently.
func TestRaceManagerCreateDelete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	dir := t.TempDir()
	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mgr.Close() }()

	const goroutines = 20
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				name := fmt.Sprintf("coll-%d-%d", id, i)
				cfg := makeCollectionConfig(name, 8)

				_, createErr := mgr.Create(cfg)
				if createErr != nil && createErr != ErrCollectionExists && createErr != ErrMaxCollections {
					// Acceptable race outcomes
					continue
				}

				// Interleave reads
				_ = mgr.List()
				_ = mgr.Exists(name)
				_ = mgr.Count()

				// Delete it
				_ = mgr.Delete(name)
			}
		}(g)
	}
	wg.Wait()
}

// TestRaceConcurrentInsert inserts points from many goroutines into the same collection.
func TestRaceConcurrentInsert(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	const dim = 16
	cfg := makeCollectionConfig("insertrace", dim)
	coll, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 20
	const opsPerGoroutine = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				p := point.NewPointWithID(
					fmt.Sprintf("p-%d-%d", id, i),
					randomVector(dim),
					map[string]interface{}{"g": id, "i": i},
				)
				if insertErr := coll.Insert(p); insertErr != nil {
					// Duplicates are fine, other errors are not expected
					continue
				}
			}
		}(g)
	}
	wg.Wait()

	size := coll.Size()
	if size <= 0 {
		t.Fatalf("expected positive collection size, got %d", size)
	}
}

// TestRaceConcurrentSearchAndInsert runs searches while inserts happen simultaneously.
func TestRaceConcurrentSearchAndInsert(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	const dim = 16
	cfg := makeCollectionConfig("searchinsert", dim)
	coll, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Pre-populate with some points so search has data to work with
	for i := 0; i < 50; i++ {
		p := point.NewPointWithID(
			fmt.Sprintf("seed-%d", i),
			randomVector(dim),
			nil,
		)
		if insertErr := coll.Insert(p); insertErr != nil {
			t.Fatal(insertErr)
		}
	}

	const goroutines = 10
	const opsPerGoroutine = 200

	var wg sync.WaitGroup

	// Inserters
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				p := point.NewPointWithID(
					fmt.Sprintf("ins-%d-%d", id, i),
					randomVector(dim),
					nil,
				)
				_ = coll.Insert(p)
			}
		}(g)
	}

	// Searchers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				query := randomVector(dim)
				_, _ = coll.Search(query, 5)
			}
		}()
	}

	wg.Wait()
}

// TestRaceConcurrentGetDelete runs Get and Delete against the same collection.
func TestRaceConcurrentGetDelete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	const dim = 8
	cfg := makeCollectionConfig("getdelete", dim)
	coll, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Pre-populate
	const numPoints = 500
	for i := 0; i < numPoints; i++ {
		p := point.NewPointWithID(
			fmt.Sprintf("pt-%d", i),
			randomVector(dim),
			nil,
		)
		if insertErr := coll.Insert(p); insertErr != nil {
			t.Fatal(insertErr)
		}
	}

	const goroutines = 10
	var wg sync.WaitGroup

	// Getters
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < numPoints; i++ {
				_, _ = coll.Get(fmt.Sprintf("pt-%d", i))
			}
		}()
	}

	// Deleters
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			start := id * (numPoints / goroutines)
			end := start + (numPoints / goroutines)
			for i := start; i < end; i++ {
				_ = coll.Delete(fmt.Sprintf("pt-%d", i))
			}
		}(g)
	}

	wg.Wait()
}

// TestRaceManagerListWhileMutating reads List and ListInfo while creating/deleting collections.
func TestRaceManagerListWhileMutating(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	dir := t.TempDir()
	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mgr.Close() }()

	const goroutines = 10
	const ops = 100

	var wg sync.WaitGroup

	// Writers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				name := fmt.Sprintf("list-coll-%d-%d", id, i)
				cfg := makeCollectionConfig(name, 4)
				_, _ = mgr.Create(cfg)
				_ = mgr.Delete(name)
			}
		}(g)
	}

	// Readers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops*2; i++ {
				_ = mgr.List()
				_ = mgr.ListInfo()
				_ = mgr.Count()
			}
		}()
	}

	wg.Wait()
}
