package benchmark

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/index/hnsw"
	"github.com/limyedb/limyedb/pkg/point"
)

// BenchmarkFilteredSearch benchmarks filtered search at various selectivities
func BenchmarkFilteredSearch(b *testing.B) {
	// Test parameters
	numVectors := 100000
	dimension := 128
	k := 10
	ef := 100

	// Create index
	cfg := &hnsw.Config{
		M:              16,
		EfConstruction: 200,
		EfSearch:       ef,
		MaxElements:    numVectors,
		Metric:         config.MetricCosine,
		Dimension:      dimension,
	}

	idx, err := hnsw.New(cfg)
	if err != nil {
		b.Fatalf("Failed to create index: %v", err)
	}

	// Generate and insert vectors with category labels
	rng := rand.New(rand.NewSource(42))
	categories := []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J"}

	for i := 0; i < numVectors; i++ {
		vec := make([]float32, dimension)
		for j := range vec {
			vec[j] = rng.Float32()*2 - 1
		}

		// Assign random category
		category := categories[i%len(categories)]

		p := &point.Point{
			ID:     fmt.Sprintf("point-%d", i),
			Vector: vec,
			Payload: map[string]interface{}{
				"category": category,
				"value":    rng.Float64() * 100,
			},
		}

		if err := idx.Insert(p); err != nil {
			b.Fatalf("Failed to insert point: %v", err)
		}
	}

	// Generate query vector
	query := make([]float32, dimension)
	for j := range query {
		query[j] = rng.Float32()*2 - 1
	}

	// Define selectivity levels
	selectivities := []struct {
		name        string
		selectivity float64
		filter      func(id string, payload map[string]interface{}) bool
	}{
		{
			name:        "90%",
			selectivity: 0.9,
			filter: func(id string, payload map[string]interface{}) bool {
				// 9 out of 10 categories pass
				cat := payload["category"].(string)
				return cat != "A"
			},
		},
		{
			name:        "50%",
			selectivity: 0.5,
			filter: func(id string, payload map[string]interface{}) bool {
				// 5 out of 10 categories pass
				cat := payload["category"].(string)
				return cat == "A" || cat == "B" || cat == "C" || cat == "D" || cat == "E"
			},
		},
		{
			name:        "20%",
			selectivity: 0.2,
			filter: func(id string, payload map[string]interface{}) bool {
				// 2 out of 10 categories pass
				cat := payload["category"].(string)
				return cat == "A" || cat == "B"
			},
		},
		{
			name:        "10%",
			selectivity: 0.1,
			filter: func(id string, payload map[string]interface{}) bool {
				// 1 out of 10 categories pass
				cat := payload["category"].(string)
				return cat == "A"
			},
		},
		{
			name:        "5%",
			selectivity: 0.05,
			filter: func(id string, payload map[string]interface{}) bool {
				// Value filter - ~5% pass
				val := payload["value"].(float64)
				return val < 5
			},
		},
		{
			name:        "1%",
			selectivity: 0.01,
			filter: func(id string, payload map[string]interface{}) bool {
				// Value filter - ~1% pass
				val := payload["value"].(float64)
				return val < 1
			},
		},
	}

	for _, sel := range selectivities {
		b.Run(fmt.Sprintf("Selectivity_%s", sel.name), func(b *testing.B) {
			params := &hnsw.SearchParams{
				K:                    k,
				Ef:                   ef,
				Filter:               sel.filter,
				EstimatedSelectivity: sel.selectivity,
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := idx.SearchWithFilter(query, params)
				if err != nil {
					b.Fatalf("Search failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkFilteredSearchAdaptive compares fixed vs adaptive ef
func BenchmarkFilteredSearchAdaptive(b *testing.B) {
	numVectors := 50000
	dimension := 128
	k := 10
	ef := 100

	cfg := &hnsw.Config{
		M:              16,
		EfConstruction: 200,
		EfSearch:       ef,
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
			Payload: map[string]interface{}{
				"bucket": i % 100, // 100 buckets, 1% each
			},
		}

		if err := idx.Insert(p); err != nil {
			b.Fatalf("Failed to insert point: %v", err)
		}
	}

	query := make([]float32, dimension)
	for j := range query {
		query[j] = rng.Float32()*2 - 1
	}

	// 5% selectivity filter (5 buckets)
	filter := func(id string, payload map[string]interface{}) bool {
		bucket := payload["bucket"].(int)
		return bucket < 5
	}

	b.Run("WithSelectivityHint", func(b *testing.B) {
		params := &hnsw.SearchParams{
			K:                    k,
			Ef:                   ef,
			Filter:               filter,
			EstimatedSelectivity: 0.05, // Provide hint
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = idx.SearchWithFilter(query, params)
		}
	})

	b.Run("WithoutSelectivityHint", func(b *testing.B) {
		params := &hnsw.SearchParams{
			K:      k,
			Ef:     ef,
			Filter: filter,
			// No selectivity hint - uses default
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = idx.SearchWithFilter(query, params)
		}
	})
}

// BenchmarkRangeFilter benchmarks range filter performance
func BenchmarkRangeFilter(b *testing.B) {
	numVectors := 50000
	dimension := 128
	k := 10
	ef := 100

	cfg := &hnsw.Config{
		M:              16,
		EfConstruction: 200,
		EfSearch:       ef,
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
			Payload: map[string]interface{}{
				"timestamp": float64(i), // Ordered timestamps
			},
		}

		if err := idx.Insert(p); err != nil {
			b.Fatalf("Failed to insert point: %v", err)
		}
	}

	query := make([]float32, dimension)
	for j := range query {
		query[j] = rng.Float32()*2 - 1
	}

	ranges := []struct {
		name   string
		minVal float64
		maxVal float64
	}{
		{"Last_10%", float64(numVectors) * 0.9, float64(numVectors)},
		{"Last_25%", float64(numVectors) * 0.75, float64(numVectors)},
		{"Middle_50%", float64(numVectors) * 0.25, float64(numVectors) * 0.75},
		{"First_10%", 0, float64(numVectors) * 0.1},
	}

	for _, r := range ranges {
		b.Run(r.name, func(b *testing.B) {
			selectivity := (r.maxVal - r.minVal) / float64(numVectors)

			params := &hnsw.SearchParams{
				K:  k,
				Ef: ef,
				Filter: func(id string, payload map[string]interface{}) bool {
					ts := payload["timestamp"].(float64)
					return ts >= r.minVal && ts < r.maxVal
				},
				EstimatedSelectivity: selectivity,
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = idx.SearchWithFilter(query, params)
			}
		})
	}
}
