//go:build arm64

// Package distance provides SIMD-optimized distance calculations.
package distance

import (
	"math"
	"unsafe"
)

// hasSIMD indicates whether SIMD instructions are available.
// This would be detected at runtime in production.
var hasSIMD = true

// CosineDistanceSIMD calculates cosine distance using SIMD when available.
func CosineDistanceSIMD(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 1.0
	}

	// Use SIMD for vectors >= 8 elements
	if hasSIMD && len(a) >= 8 {
		return cosineDistanceNEON(a, b)
	}

	return cosineDistanceScalar(a, b)
}

// EuclideanDistanceSIMD calculates Euclidean distance using SIMD.
func EuclideanDistanceSIMD(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}

	if hasSIMD && len(a) >= 8 {
		return euclideanDistanceNEON(a, b)
	}

	return euclideanDistanceScalar(a, b)
}

// DotProductSIMD calculates dot product using SIMD.
func DotProductSIMD(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}

	if hasSIMD && len(a) >= 8 {
		return dotProductNEON(a, b)
	}

	return dotProductScalar(a, b)
}

//go:noescape
func cosineDistanceNEON(a, b []float32) float32

//go:noescape
func euclideanDistanceNEON(a, b []float32) float32

//go:noescape
func dotProductNEON(a, b []float32) float32

// Scalar fallback implementations

func cosineDistanceScalar(a, b []float32) float32 {
	var dotProduct, normA, normB float32
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 1.0
	}

	similarity := dotProduct / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
	return 1.0 - similarity
}

func euclideanDistanceScalar(a, b []float32) float32 {
	var sum float32
	for i := range a {
		d := a[i] - b[i]
		sum += d * d
	}
	return float32(math.Sqrt(float64(sum)))
}

func dotProductScalar(a, b []float32) float32 {
	var sum float32
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

// BatchCosineDistance calculates cosine distance for multiple vectors.
func BatchCosineDistance(query []float32, vectors [][]float32) []float32 {
	results := make([]float32, len(vectors))

	// Process in parallel for large batches
	if len(vectors) >= 100 {
		batchParallel(query, vectors, results, CosineDistanceSIMD)
	} else {
		for i, v := range vectors {
			results[i] = CosineDistanceSIMD(query, v)
		}
	}

	return results
}

// BatchEuclideanDistance calculates Euclidean distance for multiple vectors.
func BatchEuclideanDistance(query []float32, vectors [][]float32) []float32 {
	results := make([]float32, len(vectors))

	if len(vectors) >= 100 {
		batchParallel(query, vectors, results, EuclideanDistanceSIMD)
	} else {
		for i, v := range vectors {
			results[i] = EuclideanDistanceSIMD(query, v)
		}
	}

	return results
}

func batchParallel(query []float32, vectors [][]float32, results []float32, fn func([]float32, []float32) float32) {
	// Simple parallel processing
	// In production, use worker pools
	const numWorkers = 4
	chunkSize := (len(vectors) + numWorkers - 1) / numWorkers

	done := make(chan struct{}, numWorkers)

	for w := 0; w < numWorkers; w++ {
		start := w * chunkSize
		end := start + chunkSize
		if end > len(vectors) {
			end = len(vectors)
		}

		go func(start, end int) {
			for i := start; i < end; i++ {
				results[i] = fn(query, vectors[i])
			}
			done <- struct{}{}
		}(start, end)
	}

	for w := 0; w < numWorkers; w++ {
		<-done
	}
}

// NormalizeVector normalizes a vector to unit length.
func NormalizeVector(v []float32) []float32 {
	var norm float32
	for _, val := range v {
		norm += val * val
	}
	norm = float32(math.Sqrt(float64(norm)))

	if norm == 0 {
		return v
	}

	result := make([]float32, len(v))
	invNorm := 1.0 / norm

	// SIMD-style unrolled normalization
	n := len(v)
	limit := n - (n % 4)

	for i := 0; i < limit; i += 4 {
		result[i] = v[i] * invNorm
		result[i+1] = v[i+1] * invNorm
		result[i+2] = v[i+2] * invNorm
		result[i+3] = v[i+3] * invNorm
	}

	for i := limit; i < n; i++ {
		result[i] = v[i] * invNorm
	}

	return result
}

// Prefetch hints the CPU to prefetch data.
// This is a no-op in pure Go but would use PREFETCH in assembly.
func Prefetch(ptr unsafe.Pointer) {
	// In assembly: PREFETCHT0 (ptr)
}
