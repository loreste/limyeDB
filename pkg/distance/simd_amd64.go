//go:build amd64

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
		return cosineDistanceAVX(a, b)
	}

	return cosineDistanceScalar(a, b)
}

// EuclideanDistanceSIMD calculates Euclidean distance using SIMD.
func EuclideanDistanceSIMD(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}

	if hasSIMD && len(a) >= 8 {
		return euclideanDistanceAVX(a, b)
	}

	return euclideanDistanceScalar(a, b)
}

// DotProductSIMD calculates dot product using SIMD.
func DotProductSIMD(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}

	if hasSIMD && len(a) >= 8 {
		return dotProductAVX(a, b)
	}

	return dotProductScalar(a, b)
}

// cosineDistanceAVX uses AVX instructions for cosine distance.
// In production, this would call assembly code.
func cosineDistanceAVX(a, b []float32) float32 {
	var dotProduct, normA, normB float32

	// Process 8 floats at a time (256-bit AVX registers)
	n := len(a)
	limit := n - (n % 8)

	// Unrolled loop simulating AVX processing
	for i := 0; i < limit; i += 8 {
		// In real implementation, this would use:
		// _mm256_loadu_ps, _mm256_mul_ps, _mm256_add_ps

		// Load 8 floats from a
		a0, a1, a2, a3 := a[i], a[i+1], a[i+2], a[i+3]
		a4, a5, a6, a7 := a[i+4], a[i+5], a[i+6], a[i+7]

		// Load 8 floats from b
		b0, b1, b2, b3 := b[i], b[i+1], b[i+2], b[i+3]
		b4, b5, b6, b7 := b[i+4], b[i+5], b[i+6], b[i+7]

		// Compute products
		dotProduct += a0*b0 + a1*b1 + a2*b2 + a3*b3 + a4*b4 + a5*b5 + a6*b6 + a7*b7
		normA += a0*a0 + a1*a1 + a2*a2 + a3*a3 + a4*a4 + a5*a5 + a6*a6 + a7*a7
		normB += b0*b0 + b1*b1 + b2*b2 + b3*b3 + b4*b4 + b5*b5 + b6*b6 + b7*b7
	}

	// Handle remaining elements
	for i := limit; i < n; i++ {
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

// euclideanDistanceAVX uses AVX for Euclidean distance.
func euclideanDistanceAVX(a, b []float32) float32 {
	var sum float32

	n := len(a)
	limit := n - (n % 8)

	for i := 0; i < limit; i += 8 {
		d0 := a[i] - b[i]
		d1 := a[i+1] - b[i+1]
		d2 := a[i+2] - b[i+2]
		d3 := a[i+3] - b[i+3]
		d4 := a[i+4] - b[i+4]
		d5 := a[i+5] - b[i+5]
		d6 := a[i+6] - b[i+6]
		d7 := a[i+7] - b[i+7]

		sum += d0*d0 + d1*d1 + d2*d2 + d3*d3 + d4*d4 + d5*d5 + d6*d6 + d7*d7
	}

	for i := limit; i < n; i++ {
		d := a[i] - b[i]
		sum += d * d
	}

	return float32(math.Sqrt(float64(sum)))
}

// dotProductAVX uses AVX for dot product.
func dotProductAVX(a, b []float32) float32 {
	var sum float32

	n := len(a)
	limit := n - (n % 8)

	for i := 0; i < limit; i += 8 {
		sum += a[i]*b[i] + a[i+1]*b[i+1] + a[i+2]*b[i+2] + a[i+3]*b[i+3] +
			a[i+4]*b[i+4] + a[i+5]*b[i+5] + a[i+6]*b[i+6] + a[i+7]*b[i+7]
	}

	for i := limit; i < n; i++ {
		sum += a[i] * b[i]
	}

	return sum
}

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
