package leaktest

import (
	"fmt"
	"math/rand"
	"runtime"
	"testing"

	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/index/hnsw"
	"github.com/limyedb/limyedb/pkg/point"
)

func TestMemoryStabilityVectorAllocation(t *testing.T) {
	// Allocate and discard large numbers of vectors, then verify
	// that heap growth stays bounded after GC.
	runtime.GC()
	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)

	const rounds = 20
	const vectorsPerRound = 5000
	const dim = 128

	for round := 0; round < rounds; round++ {
		vectors := make([][]float32, vectorsPerRound)
		for i := 0; i < vectorsPerRound; i++ {
			vec := make([]float32, dim)
			for d := 0; d < dim; d++ {
				vec[d] = float32(i*dim + d)
			}
			vectors[i] = vec
		}
		// Discard by overwriting -- GC should reclaim.
		_ = vectors
	}

	runtime.GC()
	runtime.GC() // Double GC to ensure finalizers and weak refs are cleaned up.

	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	heapGrowth := int64(after.HeapInuse) - int64(baseline.HeapInuse)
	const maxAllowed = 20 * 1024 * 1024 // 20 MB
	t.Logf("heap growth after %d rounds of %d vectors (dim=%d): %d bytes",
		rounds, vectorsPerRound, dim, heapGrowth)

	if heapGrowth > maxAllowed {
		t.Errorf("heap grew %d bytes, exceeding limit of %d bytes", heapGrowth, maxAllowed)
	}
}

func TestMemoryStabilityHNSWInsertDelete(t *testing.T) {
	// Build an HNSW index, insert points, delete them, and check memory.
	dim := 64
	idx, err := hnsw.New(&hnsw.Config{
		M:              8,
		EfConstruction: 50,
		EfSearch:       30,
		MaxElements:    10000,
		Metric:         config.MetricCosine,
		Dimension:      dim,
	})
	if err != nil {
		t.Fatalf("failed to create HNSW: %v", err)
	}

	rng := rand.New(rand.NewSource(12345))

	// Insert a batch of points.
	const numPoints = 2000
	for i := 0; i < numPoints; i++ {
		vec := make(point.Vector, dim)
		for d := range vec {
			vec[d] = rng.Float32()
		}
		if err := idx.Insert(&point.Point{
			ID:     fmt.Sprintf("point-%d", i),
			Vector: vec,
		}); err != nil {
			t.Fatalf("insert %d failed: %v", i, err)
		}
	}

	runtime.GC()
	var afterInsert runtime.MemStats
	runtime.ReadMemStats(&afterInsert)

	// Delete all points (lazy deletion).
	for i := 0; i < numPoints; i++ {
		_ = idx.Delete(fmt.Sprintf("point-%d", i))
	}

	runtime.GC()
	var afterDelete runtime.MemStats
	runtime.ReadMemStats(&afterDelete)

	t.Logf("heap after insert: %d bytes, after delete: %d bytes",
		afterInsert.HeapInuse, afterDelete.HeapInuse)

	// The index uses lazy deletion so memory won't fully drop, but it should
	// not grow after deletion.
	growth := int64(afterDelete.HeapInuse) - int64(afterInsert.HeapInuse)
	const maxGrowthAfterDelete = 5 * 1024 * 1024 // 5 MB
	if growth > maxGrowthAfterDelete {
		t.Errorf("heap grew %d bytes after deleting all points (limit %d)",
			growth, maxGrowthAfterDelete)
	}
}

func TestMemoryStabilityRepeatedSearches(t *testing.T) {
	// Verify that repeated searches don't leak memory.
	dim := 32
	idx, err := hnsw.New(&hnsw.Config{
		M:              8,
		EfConstruction: 50,
		EfSearch:       30,
		MaxElements:    1000,
		Metric:         config.MetricCosine,
		Dimension:      dim,
	})
	if err != nil {
		t.Fatalf("failed to create HNSW: %v", err)
	}

	rng := rand.New(rand.NewSource(777))

	// Build a small index.
	for i := 0; i < 500; i++ {
		vec := make(point.Vector, dim)
		for d := range vec {
			vec[d] = rng.Float32()
		}
		if err := idx.Insert(&point.Point{
			ID:     fmt.Sprintf("s-%d", i),
			Vector: vec,
		}); err != nil {
			t.Fatalf("insert failed: %v", err)
		}
	}

	runtime.GC()
	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)

	// Run many searches.
	const searchRounds = 5000
	for i := 0; i < searchRounds; i++ {
		query := make(point.Vector, dim)
		for d := range query {
			query[d] = rng.Float32()
		}
		_, _ = idx.Search(query, 10)
	}

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	heapGrowth := int64(after.HeapInuse) - int64(baseline.HeapInuse)
	const maxAllowed = 10 * 1024 * 1024 // 10 MB
	t.Logf("heap growth after %d searches: %d bytes", searchRounds, heapGrowth)

	if heapGrowth > maxAllowed {
		t.Errorf("heap grew %d bytes after searches, limit %d", heapGrowth, maxAllowed)
	}
}
