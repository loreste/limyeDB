package distance

import (
	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/point"
)

// Calculator defines the interface for distance/similarity calculations
type Calculator interface {
	// Distance calculates the distance between two vectors
	// Lower values indicate more similar vectors
	Distance(a, b point.Vector) float32

	// Name returns the name of the distance metric
	Name() string

	// IsSimilarity returns true if higher values mean more similar
	// (e.g., dot product, cosine similarity)
	IsSimilarity() bool
}

// New creates a new distance calculator based on the metric type
func New(metric config.MetricType) Calculator {
	switch metric {
	case config.MetricCosine:
		return &Cosine{}
	case config.MetricEuclidean:
		return &Euclidean{}
	case config.MetricDotProduct:
		return &DotProduct{}
	default:
		return &Cosine{} // Default to cosine
	}
}

// BatchDistance calculates distances between a query and multiple vectors
func BatchDistance(calc Calculator, query point.Vector, vectors []point.Vector) []float32 {
	results := make([]float32, len(vectors))
	for i, v := range vectors {
		results[i] = calc.Distance(query, v)
	}
	return results
}

// BatchDistanceParallel calculates distances in parallel using goroutines
func BatchDistanceParallel(calc Calculator, query point.Vector, vectors []point.Vector, workers int) []float32 {
	n := len(vectors)
	results := make([]float32, n)

	if workers <= 0 || workers > n {
		workers = n
	}

	chunkSize := (n + workers - 1) / workers
	done := make(chan struct{}, workers)

	for w := 0; w < workers; w++ {
		start := w * chunkSize
		end := start + chunkSize
		if end > n {
			end = n
		}

		go func(start, end int) {
			for i := start; i < end; i++ {
				results[i] = calc.Distance(query, vectors[i])
			}
			done <- struct{}{}
		}(start, end)
	}

	// Wait for all workers
	for w := 0; w < workers; w++ {
		<-done
	}

	return results
}

// FindNearest finds the k nearest vectors to the query
func FindNearest(calc Calculator, query point.Vector, vectors []point.Vector, k int) []int {
	if k <= 0 {
		return nil
	}
	if k > len(vectors) {
		k = len(vectors)
	}

	// Calculate all distances
	distances := BatchDistance(calc, query, vectors)

	// Find top k using partial sort
	indices := make([]int, len(vectors))
	for i := range indices {
		indices[i] = i
	}

	isSimilarity := calc.IsSimilarity()

	// Simple selection sort for top k
	for i := 0; i < k; i++ {
		best := i
		for j := i + 1; j < len(distances); j++ {
			if isSimilarity {
				// Higher is better for similarity
				if distances[indices[j]] > distances[indices[best]] {
					best = j
				}
			} else {
				// Lower is better for distance
				if distances[indices[j]] < distances[indices[best]] {
					best = j
				}
			}
		}
		indices[i], indices[best] = indices[best], indices[i]
	}

	return indices[:k]
}

// Normalize normalizes a vector to unit length (for cosine similarity)
func Normalize(v point.Vector) point.Vector {
	normalized := make(point.Vector, len(v))
	copy(normalized, v)
	normalized.Normalize()
	return normalized
}
