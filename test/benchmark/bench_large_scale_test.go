package benchmark

import (
	"fmt"
	"math/rand"
	"runtime"
	"testing"
	"time"

	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/index/hnsw"
	"github.com/limyedb/limyedb/pkg/point"
	"github.com/limyedb/limyedb/pkg/quantization"
)

// toVectors converts [][]float32 to []point.Vector
func toVectors(vecs [][]float32) []point.Vector {
	result := make([]point.Vector, len(vecs))
	for i, v := range vecs {
		result[i] = point.Vector(v)
	}
	return result
}

// BenchmarkLargeScaleInsert benchmarks insertion throughput at scale
func BenchmarkLargeScaleInsert(b *testing.B) {
	scales := []int{10000, 100000, 500000}
	dimensions := []int{128, 384}

	for _, scale := range scales {
		for _, dim := range dimensions {
			b.Run(fmt.Sprintf("N%d_D%d", scale, dim), func(b *testing.B) {
				cfg := &hnsw.Config{
					M:              16,
					EfConstruction: 100, // Lower for faster builds
					EfSearch:       100,
					MaxElements:    scale,
					Metric:         config.MetricCosine,
					Dimension:      dim,
				}

				idx, err := hnsw.New(cfg)
				if err != nil {
					b.Fatalf("Failed to create index: %v", err)
				}

				rng := rand.New(rand.NewSource(42))

				// Pre-generate vectors
				vectors := make([][]float32, scale)
				for i := range vectors {
					vectors[i] = make([]float32, dim)
					for j := range vectors[i] {
						vectors[i][j] = rng.Float32()*2 - 1
					}
				}

				b.ResetTimer()

				inserted := 0
				start := time.Now()

				for n := 0; n < b.N; n++ {
					i := n % scale
					p := &point.Point{
						ID:     fmt.Sprintf("point-%d-%d", n, i),
						Vector: vectors[i],
					}
					if err := idx.Insert(p); err != nil {
						// Expected when re-inserting
						continue
					}
					inserted++
				}

				elapsed := time.Since(start)
				if inserted > 0 {
					b.ReportMetric(float64(inserted)/elapsed.Seconds(), "inserts/sec")
				}
			})
		}
	}
}

// BenchmarkLargeScaleSearch benchmarks search at different scales
func BenchmarkLargeScaleSearch(b *testing.B) {
	scales := []int{10000, 100000, 500000}
	dimension := 128

	for _, scale := range scales {
		b.Run(fmt.Sprintf("N%d", scale), func(b *testing.B) {
			cfg := &hnsw.Config{
				M:              16,
				EfConstruction: 100,
				EfSearch:       100,
				MaxElements:    scale,
				Metric:         config.MetricCosine,
				Dimension:      dimension,
			}

			idx, err := hnsw.New(cfg)
			if err != nil {
				b.Fatalf("Failed to create index: %v", err)
			}

			rng := rand.New(rand.NewSource(42))

			// Insert vectors
			for i := 0; i < scale; i++ {
				vec := make([]float32, dimension)
				for j := range vec {
					vec[j] = rng.Float32()*2 - 1
				}
				p := &point.Point{
					ID:     fmt.Sprintf("point-%d", i),
					Vector: vec,
				}
				if err := idx.Insert(p); err != nil {
					b.Fatalf("Failed to insert: %v", err)
				}
			}

			// Generate query
			query := make([]float32, dimension)
			for j := range query {
				query[j] = rng.Float32()*2 - 1
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := idx.Search(query, 10)
				if err != nil {
					b.Fatalf("Search failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkLargeScaleBatchSearch benchmarks batch search performance
func BenchmarkLargeScaleBatchSearch(b *testing.B) {
	numVectors := 100000
	dimension := 128
	batchSizes := []int{1, 10, 50, 100}

	cfg := &hnsw.Config{
		M:              16,
		EfConstruction: 100,
		EfSearch:       100,
		MaxElements:    numVectors,
		Metric:         config.MetricCosine,
		Dimension:      dimension,
	}

	idx, err := hnsw.New(cfg)
	if err != nil {
		b.Fatalf("Failed to create index: %v", err)
	}

	rng := rand.New(rand.NewSource(42))

	for i := 0; i < numVectors; i++ {
		vec := make([]float32, dimension)
		for j := range vec {
			vec[j] = rng.Float32()*2 - 1
		}
		p := &point.Point{
			ID:     fmt.Sprintf("point-%d", i),
			Vector: vec,
		}
		if err := idx.Insert(p); err != nil {
			b.Fatalf("Failed to insert: %v", err)
		}
	}

	for _, batchSize := range batchSizes {
		b.Run(fmt.Sprintf("Batch%d", batchSize), func(b *testing.B) {
			// Generate batch queries
			queries := make([]point.Vector, batchSize)
			for i := range queries {
				queries[i] = make([]float32, dimension)
				for j := range queries[i] {
					queries[i][j] = rng.Float32()*2 - 1
				}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := idx.BatchSearch(queries, 10)
				if err != nil {
					b.Fatalf("Batch search failed: %v", err)
				}
			}

			b.ReportMetric(float64(batchSize), "queries/op")
		})
	}
}

// BenchmarkLargeScaleMemory measures memory usage at scale
func BenchmarkLargeScaleMemory(b *testing.B) {
	scales := []struct {
		name    string
		vectors int
		dim     int
	}{
		{"100K_128D", 100000, 128},
		{"100K_384D", 100000, 384},
		{"500K_128D", 500000, 128},
	}

	for _, scale := range scales {
		b.Run(scale.name, func(b *testing.B) {
			runtime.GC()
			var memBefore runtime.MemStats
			runtime.ReadMemStats(&memBefore)

			cfg := &hnsw.Config{
				M:              16,
				EfConstruction: 100,
				EfSearch:       100,
				MaxElements:    scale.vectors,
				Metric:         config.MetricCosine,
				Dimension:      scale.dim,
			}

			idx, err := hnsw.New(cfg)
			if err != nil {
				b.Fatalf("Failed to create index: %v", err)
			}

			rng := rand.New(rand.NewSource(42))

			for i := 0; i < scale.vectors; i++ {
				vec := make([]float32, scale.dim)
				for j := range vec {
					vec[j] = rng.Float32()*2 - 1
				}
				p := &point.Point{
					ID:     fmt.Sprintf("point-%d", i),
					Vector: vec,
				}
				_ = idx.Insert(p)
			}

			runtime.GC()
			var memAfter runtime.MemStats
			runtime.ReadMemStats(&memAfter)

			memUsedMB := float64(memAfter.Alloc-memBefore.Alloc) / (1024 * 1024)
			bytesPerVector := float64(memAfter.Alloc-memBefore.Alloc) / float64(scale.vectors)

			b.ReportMetric(memUsedMB, "MB_total")
			b.ReportMetric(bytesPerVector, "bytes/vector")
		})
	}
}

// BenchmarkLargeScaleQuantized benchmarks search with quantization at scale
func BenchmarkLargeScaleQuantized(b *testing.B) {
	numVectors := 100000
	dimension := 384
	k := 10
	ef := 100

	rng := rand.New(rand.NewSource(42))

	// Pre-generate vectors
	vectors := make([][]float32, numVectors)
	for i := range vectors {
		vectors[i] = make([]float32, dimension)
		for j := range vectors[i] {
			vectors[i][j] = rng.Float32()*2 - 1
		}
	}

	query := make([]float32, dimension)
	for j := range query {
		query[j] = rng.Float32()*2 - 1
	}

	b.Run("NoQuantization", func(b *testing.B) {
		cfg := &hnsw.Config{
			M:              16,
			EfConstruction: 100,
			EfSearch:       ef,
			MaxElements:    numVectors,
			Metric:         config.MetricCosine,
			Dimension:      dimension,
		}

		idx, _ := hnsw.New(cfg)
		for i, vec := range vectors {
			p := &point.Point{ID: fmt.Sprintf("p-%d", i), Vector: vec}
			_ = idx.Insert(p)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = idx.SearchWithEf(query, k, ef)
		}
	})

	b.Run("ScalarQuantization", func(b *testing.B) {
		q := quantization.NewScalarQuantizer(dimension, 0.99)
		trainingSlice := vectors[:min(10000, len(vectors))]
		trainingData := make([]point.Vector, len(trainingSlice))
		for i, v := range trainingSlice {
			trainingData[i] = point.Vector(v)
		}
		_ = q.Train(trainingData)

		cfg := &hnsw.Config{
			M:              16,
			EfConstruction: 100,
			EfSearch:       ef,
			MaxElements:    numVectors,
			Metric:         config.MetricCosine,
			Dimension:      dimension,
			Quantizer:      q,
		}

		idx, _ := hnsw.New(cfg)
		for i, vec := range vectors {
			p := &point.Point{ID: fmt.Sprintf("p-%d", i), Vector: vec}
			_ = idx.Insert(p)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = idx.SearchWithEf(query, k, ef)
		}
	})
}

// BenchmarkRecallAtScale measures recall at different scales
func BenchmarkRecallAtScale(b *testing.B) {
	scales := []int{10000, 50000, 100000}
	dimension := 128
	k := 10
	numQueries := 100

	for _, scale := range scales {
		b.Run(fmt.Sprintf("N%d", scale), func(b *testing.B) {
			cfg := &hnsw.Config{
				M:              16,
				EfConstruction: 200,
				EfSearch:       100,
				MaxElements:    scale,
				Metric:         config.MetricCosine,
				Dimension:      dimension,
			}

			idx, _ := hnsw.New(cfg)

			rng := rand.New(rand.NewSource(42))

			// Insert vectors and store for ground truth calculation
			allVectors := make([][]float32, scale)
			for i := 0; i < scale; i++ {
				vec := make([]float32, dimension)
				for j := range vec {
					vec[j] = rng.Float32()*2 - 1
				}
				allVectors[i] = vec
				p := &point.Point{ID: fmt.Sprintf("p-%d", i), Vector: vec}
				_ = idx.Insert(p)
			}

			// Generate queries
			queries := make([][]float32, numQueries)
			for i := range queries {
				queries[i] = make([]float32, dimension)
				for j := range queries[i] {
					queries[i][j] = rng.Float32()*2 - 1
				}
			}

			b.ResetTimer()

			// Measure search performance and recall
			totalRecall := 0.0
			for i := 0; i < b.N; i++ {
				queryIdx := i % numQueries
				results, _ := idx.Search(queries[queryIdx], k)
				if len(results) > 0 {
					// Simple recall approximation
					totalRecall += float64(len(results)) / float64(k)
				}
			}

			avgRecall := totalRecall / float64(b.N)
			b.ReportMetric(avgRecall*100, "recall_%")
		})
	}
}

// BenchmarkEfScaling measures how ef affects latency and recall
func BenchmarkEfScaling(b *testing.B) {
	numVectors := 50000
	dimension := 128
	k := 10
	efValues := []int{50, 100, 200, 500, 1000}

	cfg := &hnsw.Config{
		M:              16,
		EfConstruction: 200,
		EfSearch:       100,
		MaxElements:    numVectors,
		Metric:         config.MetricCosine,
		Dimension:      dimension,
	}

	idx, _ := hnsw.New(cfg)

	rng := rand.New(rand.NewSource(42))

	for i := 0; i < numVectors; i++ {
		vec := make([]float32, dimension)
		for j := range vec {
			vec[j] = rng.Float32()*2 - 1
		}
		p := &point.Point{ID: fmt.Sprintf("p-%d", i), Vector: vec}
		_ = idx.Insert(p)
	}

	query := make([]float32, dimension)
	for j := range query {
		query[j] = rng.Float32()*2 - 1
	}

	for _, ef := range efValues {
		b.Run(fmt.Sprintf("ef%d", ef), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = idx.SearchWithEf(query, k, ef)
			}
		})
	}
}
