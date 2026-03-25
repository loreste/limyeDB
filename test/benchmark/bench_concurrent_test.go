package benchmark

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/index/hnsw"
	"github.com/limyedb/limyedb/pkg/point"
)

// BenchmarkConcurrentSearch benchmarks search throughput with concurrent queries
func BenchmarkConcurrentSearch(b *testing.B) {
	numVectors := 50000
	dimension := 128
	k := 10
	concurrencies := []int{1, 2, 4, 8, 16, 32}

	cfg := &hnsw.Config{
		M:              16,
		EfConstruction: 200,
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

	// Insert vectors
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

	// Pre-generate queries
	numQueries := 100
	queries := make([][]float32, numQueries)
	for i := range queries {
		queries[i] = make([]float32, dimension)
		for j := range queries[i] {
			queries[i][j] = rng.Float32()*2 - 1
		}
	}

	for _, concurrency := range concurrencies {
		b.Run(fmt.Sprintf("Goroutines%d", concurrency), func(b *testing.B) {
			var counter atomic.Int64

			b.ResetTimer()
			start := time.Now()

			var wg sync.WaitGroup
			sem := make(chan struct{}, concurrency)

			for i := 0; i < b.N; i++ {
				wg.Add(1)
				sem <- struct{}{}

				go func(queryIdx int) {
					defer wg.Done()
					defer func() { <-sem }()

					query := queries[queryIdx%numQueries]
					_, err := idx.Search(query, k)
					if err == nil {
						counter.Add(1)
					}
				}(i)
			}

			wg.Wait()
			elapsed := time.Since(start)

			completed := counter.Load()
			qps := float64(completed) / elapsed.Seconds()
			b.ReportMetric(qps, "queries/sec")
		})
	}
}

// BenchmarkMixedWorkload benchmarks mixed read/write workload
func BenchmarkMixedWorkload(b *testing.B) {
	initialVectors := 10000
	dimension := 128
	k := 10

	ratios := []struct {
		name         string
		readPercent  int
		writePercent int
	}{
		{"Read90_Write10", 90, 10},
		{"Read80_Write20", 80, 20},
		{"Read50_Write50", 50, 50},
		{"Read20_Write80", 20, 80},
	}

	for _, ratio := range ratios {
		b.Run(ratio.name, func(b *testing.B) {
			cfg := &hnsw.Config{
				M:              16,
				EfConstruction: 100,
				EfSearch:       100,
				MaxElements:    initialVectors + b.N,
				Metric:         config.MetricCosine,
				Dimension:      dimension,
			}

			idx, err := hnsw.New(cfg)
			if err != nil {
				b.Fatalf("Failed to create index: %v", err)
			}

			rng := rand.New(rand.NewSource(42))

			// Pre-populate
			for i := 0; i < initialVectors; i++ {
				vec := make([]float32, dimension)
				for j := range vec {
					vec[j] = rng.Float32()*2 - 1
				}
				p := &point.Point{
					ID:     fmt.Sprintf("init-%d", i),
					Vector: vec,
				}
				_ = idx.Insert(p)
			}

			// Pre-generate vectors for writes
			writeVectors := make([][]float32, b.N)
			for i := range writeVectors {
				writeVectors[i] = make([]float32, dimension)
				for j := range writeVectors[i] {
					writeVectors[i][j] = rng.Float32()*2 - 1
				}
			}

			// Pre-generate queries
			queries := make([][]float32, 100)
			for i := range queries {
				queries[i] = make([]float32, dimension)
				for j := range queries[i] {
					queries[i][j] = rng.Float32()*2 - 1
				}
			}

			var reads, writes atomic.Int64
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				isRead := rng.Intn(100) < ratio.readPercent
				if isRead {
					query := queries[i%len(queries)]
					_, _ = idx.Search(query, k)
					reads.Add(1)
				} else {
					p := &point.Point{
						ID:     fmt.Sprintf("write-%d", i),
						Vector: writeVectors[i],
					}
					_ = idx.Insert(p)
					writes.Add(1)
				}
			}

			b.ReportMetric(float64(reads.Load()), "reads")
			b.ReportMetric(float64(writes.Load()), "writes")
		})
	}
}

// BenchmarkConcurrentMixedWorkload benchmarks concurrent mixed workload
func BenchmarkConcurrentMixedWorkload(b *testing.B) {
	initialVectors := 10000
	dimension := 128
	k := 10
	concurrency := 8
	readPercent := 80

	cfg := &hnsw.Config{
		M:              16,
		EfConstruction: 100,
		EfSearch:       100,
		MaxElements:    initialVectors + b.N,
		Metric:         config.MetricCosine,
		Dimension:      dimension,
	}

	idx, err := hnsw.New(cfg)
	if err != nil {
		b.Fatalf("Failed to create index: %v", err)
	}

	rng := rand.New(rand.NewSource(42))

	// Pre-populate
	for i := 0; i < initialVectors; i++ {
		vec := make([]float32, dimension)
		for j := range vec {
			vec[j] = rng.Float32()*2 - 1
		}
		p := &point.Point{
			ID:     fmt.Sprintf("init-%d", i),
			Vector: vec,
		}
		_ = idx.Insert(p)
	}

	// Pre-generate data
	vectors := make([][]float32, b.N)
	for i := range vectors {
		vectors[i] = make([]float32, dimension)
		for j := range vectors[i] {
			vectors[i][j] = rng.Float32()*2 - 1
		}
	}

	var reads, writes atomic.Int64
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)

	b.ResetTimer()
	start := time.Now()

	for i := 0; i < b.N; i++ {
		wg.Add(1)
		sem <- struct{}{}

		go func(idx_ *hnsw.HNSW, opIdx int) {
			defer wg.Done()
			defer func() { <-sem }()

			localRng := rand.New(rand.NewSource(int64(opIdx)))
			isRead := localRng.Intn(100) < readPercent

			if isRead {
				query := vectors[opIdx%len(vectors)]
				_, _ = idx_.Search(query, k)
				reads.Add(1)
			} else {
				p := &point.Point{
					ID:     fmt.Sprintf("write-%d", opIdx),
					Vector: vectors[opIdx],
				}
				_ = idx_.Insert(p)
				writes.Add(1)
			}
		}(idx, i)
	}

	wg.Wait()
	elapsed := time.Since(start)

	totalOps := reads.Load() + writes.Load()
	opsPerSec := float64(totalOps) / elapsed.Seconds()

	b.ReportMetric(opsPerSec, "ops/sec")
	b.ReportMetric(float64(reads.Load()), "reads")
	b.ReportMetric(float64(writes.Load()), "writes")
}

// BenchmarkInsertWhileSearching benchmarks insert throughput while searches are running
func BenchmarkInsertWhileSearching(b *testing.B) {
	initialVectors := 10000
	dimension := 128
	searchWorkers := 4

	cfg := &hnsw.Config{
		M:              16,
		EfConstruction: 100,
		EfSearch:       100,
		MaxElements:    initialVectors + b.N,
		Metric:         config.MetricCosine,
		Dimension:      dimension,
	}

	idx, err := hnsw.New(cfg)
	if err != nil {
		b.Fatalf("Failed to create index: %v", err)
	}

	rng := rand.New(rand.NewSource(42))

	// Pre-populate
	for i := 0; i < initialVectors; i++ {
		vec := make([]float32, dimension)
		for j := range vec {
			vec[j] = rng.Float32()*2 - 1
		}
		p := &point.Point{
			ID:     fmt.Sprintf("init-%d", i),
			Vector: vec,
		}
		_ = idx.Insert(p)
	}

	// Pre-generate vectors for inserts
	insertVectors := make([][]float32, b.N)
	for i := range insertVectors {
		insertVectors[i] = make([]float32, dimension)
		for j := range insertVectors[i] {
			insertVectors[i][j] = rng.Float32()*2 - 1
		}
	}

	// Pre-generate queries
	queries := make([][]float32, 100)
	for i := range queries {
		queries[i] = make([]float32, dimension)
		for j := range queries[i] {
			queries[i][j] = rng.Float32()*2 - 1
		}
	}

	// Start background search workers
	ctx := make(chan struct{})
	var searchCount atomic.Int64

	for w := 0; w < searchWorkers; w++ {
		go func(workerID int) {
			localRng := rand.New(rand.NewSource(int64(workerID)))
			for {
				select {
				case <-ctx:
					return
				default:
					query := queries[localRng.Intn(len(queries))]
					_, _ = idx.Search(query, 10)
					searchCount.Add(1)
				}
			}
		}(w)
	}

	b.ResetTimer()
	start := time.Now()

	// Run inserts
	for i := 0; i < b.N; i++ {
		p := &point.Point{
			ID:     fmt.Sprintf("insert-%d", i),
			Vector: insertVectors[i],
		}
		_ = idx.Insert(p)
	}

	elapsed := time.Since(start)
	close(ctx) // Stop search workers

	insertsPerSec := float64(b.N) / elapsed.Seconds()
	searchesCompleted := searchCount.Load()

	b.ReportMetric(insertsPerSec, "inserts/sec")
	b.ReportMetric(float64(searchesCompleted), "background_searches")
}

// BenchmarkSearchLatencyDistribution measures latency distribution under load
func BenchmarkSearchLatencyDistribution(b *testing.B) {
	numVectors := 50000
	dimension := 128
	k := 10
	concurrency := 8

	cfg := &hnsw.Config{
		M:              16,
		EfConstruction: 200,
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
		_ = idx.Insert(p)
	}

	queries := make([][]float32, 100)
	for i := range queries {
		queries[i] = make([]float32, dimension)
		for j := range queries[i] {
			queries[i][j] = rng.Float32()*2 - 1
		}
	}

	var latencies []time.Duration
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		wg.Add(1)
		sem <- struct{}{}

		go func(queryIdx int) {
			defer wg.Done()
			defer func() { <-sem }()

			start := time.Now()
			query := queries[queryIdx%len(queries)]
			_, _ = idx.Search(query, k)
			elapsed := time.Since(start)

			mu.Lock()
			latencies = append(latencies, elapsed)
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	if len(latencies) > 0 {
		// Calculate percentiles
		var total time.Duration
		for _, l := range latencies {
			total += l
		}
		avgLatency := total / time.Duration(len(latencies))
		b.ReportMetric(float64(avgLatency.Microseconds()), "avg_latency_us")
	}
}

// BenchmarkDeleteUnderLoad benchmarks deletion while searches are running
func BenchmarkDeleteUnderLoad(b *testing.B) {
	numVectors := 50000
	dimension := 128
	searchWorkers := 4

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

	// Insert vectors
	ids := make([]string, numVectors)
	for i := 0; i < numVectors; i++ {
		vec := make([]float32, dimension)
		for j := range vec {
			vec[j] = rng.Float32()*2 - 1
		}
		id := fmt.Sprintf("point-%d", i)
		ids[i] = id
		p := &point.Point{
			ID:     id,
			Vector: vec,
		}
		_ = idx.Insert(p)
	}

	// Shuffle IDs for random deletion order
	rng.Shuffle(len(ids), func(i, j int) {
		ids[i], ids[j] = ids[j], ids[i]
	})

	queries := make([][]float32, 100)
	for i := range queries {
		queries[i] = make([]float32, dimension)
		for j := range queries[i] {
			queries[i][j] = rng.Float32()*2 - 1
		}
	}

	// Start background search workers
	ctx := make(chan struct{})
	var searchCount atomic.Int64

	for w := 0; w < searchWorkers; w++ {
		go func(workerID int) {
			localRng := rand.New(rand.NewSource(int64(workerID)))
			for {
				select {
				case <-ctx:
					return
				default:
					query := queries[localRng.Intn(len(queries))]
					_, _ = idx.Search(query, 10)
					searchCount.Add(1)
				}
			}
		}(w)
	}

	// Limit deletions to not exceed available IDs
	deleteCount := min(b.N, len(ids))

	b.ResetTimer()
	start := time.Now()

	for i := 0; i < deleteCount; i++ {
		_ = idx.Delete(ids[i])
	}

	elapsed := time.Since(start)
	close(ctx)

	deletesPerSec := float64(deleteCount) / elapsed.Seconds()
	searchesCompleted := searchCount.Load()

	b.ReportMetric(deletesPerSec, "deletes/sec")
	b.ReportMetric(float64(searchesCompleted), "background_searches")
}
