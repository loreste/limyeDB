//go:build amd64

package distance

import (
	"math"
	"testing"
)

func TestCosineDistanceSIMD(t *testing.T) {
	a := []float32{1.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}
	b := []float32{1.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}

	dist := CosineDistanceSIMD(a, b)
	if dist > 0.001 {
		t.Errorf("expected distance ~0 for identical vectors, got %f", dist)
	}

	// Orthogonal vectors
	c := []float32{0.0, 1.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}
	dist = CosineDistanceSIMD(a, c)
	if dist < 0.99 || dist > 1.01 {
		t.Errorf("expected distance ~1 for orthogonal vectors, got %f", dist)
	}
}

func TestCosineDistanceSIMD_ShortVectors(t *testing.T) {
	// Test with vectors < 8 elements (scalar fallback)
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{1.0, 0.0, 0.0}

	dist := CosineDistanceSIMD(a, b)
	if dist > 0.001 {
		t.Errorf("expected distance ~0 for identical short vectors, got %f", dist)
	}
}

func TestCosineDistanceSIMD_EmptyVectors(t *testing.T) {
	a := []float32{}
	b := []float32{}

	dist := CosineDistanceSIMD(a, b)
	if dist != 1.0 {
		t.Errorf("expected distance 1.0 for empty vectors, got %f", dist)
	}
}

func TestCosineDistanceSIMD_MismatchedLength(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{1.0, 0.0, 0.0}

	dist := CosineDistanceSIMD(a, b)
	if dist != 1.0 {
		t.Errorf("expected distance 1.0 for mismatched lengths, got %f", dist)
	}
}

func TestEuclideanDistanceSIMD(t *testing.T) {
	a := []float32{0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}
	b := []float32{3.0, 4.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}

	dist := EuclideanDistanceSIMD(a, b)
	expected := float32(5.0) // 3-4-5 triangle

	if math.Abs(float64(dist-expected)) > 0.001 {
		t.Errorf("expected distance 5.0, got %f", dist)
	}
}

func TestEuclideanDistanceSIMD_Identical(t *testing.T) {
	a := []float32{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0}
	b := []float32{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0}

	dist := EuclideanDistanceSIMD(a, b)
	if dist > 0.001 {
		t.Errorf("expected distance 0 for identical vectors, got %f", dist)
	}
}

func TestDotProductSIMD(t *testing.T) {
	a := []float32{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0}
	b := []float32{1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0}

	dot := DotProductSIMD(a, b)
	expected := float32(1.0 + 2.0 + 3.0 + 4.0 + 5.0 + 6.0 + 7.0 + 8.0) // 36.0

	if math.Abs(float64(dot-expected)) > 0.001 {
		t.Errorf("expected dot product 36.0, got %f", dot)
	}
}

func TestDotProductSIMD_Orthogonal(t *testing.T) {
	a := []float32{1.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}
	b := []float32{0.0, 1.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}

	dot := DotProductSIMD(a, b)
	if dot != 0.0 {
		t.Errorf("expected dot product 0 for orthogonal vectors, got %f", dot)
	}
}

func TestBatchCosineDistance(t *testing.T) {
	query := []float32{1.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}
	vectors := [][]float32{
		{1.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}, // identical
		{0.0, 1.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}, // orthogonal
		{0.5, 0.5, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}, // similar
	}

	results := BatchCosineDistance(query, vectors)

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	if results[0] > 0.001 {
		t.Errorf("expected distance ~0 for identical, got %f", results[0])
	}
	if results[1] < 0.99 {
		t.Errorf("expected distance ~1 for orthogonal, got %f", results[1])
	}
}

func TestBatchEuclideanDistance(t *testing.T) {
	query := []float32{0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}
	vectors := [][]float32{
		{0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0},
		{1.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0},
		{3.0, 4.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0},
	}

	results := BatchEuclideanDistance(query, vectors)

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	if results[0] != 0.0 {
		t.Errorf("expected distance 0 for identical, got %f", results[0])
	}
	if math.Abs(float64(results[1]-1.0)) > 0.001 {
		t.Errorf("expected distance 1.0, got %f", results[1])
	}
	if math.Abs(float64(results[2]-5.0)) > 0.001 {
		t.Errorf("expected distance 5.0, got %f", results[2])
	}
}

func TestNormalizeVector(t *testing.T) {
	v := []float32{3.0, 4.0, 0.0, 0.0}
	normalized := NormalizeVector(v)

	// Should be unit length
	var norm float32
	for _, val := range normalized {
		norm += val * val
	}

	if math.Abs(float64(norm-1.0)) > 0.001 {
		t.Errorf("expected unit norm, got %f", norm)
	}

	// Check specific values
	if math.Abs(float64(normalized[0]-0.6)) > 0.001 {
		t.Errorf("expected 0.6, got %f", normalized[0])
	}
	if math.Abs(float64(normalized[1]-0.8)) > 0.001 {
		t.Errorf("expected 0.8, got %f", normalized[1])
	}
}

func TestNormalizeVector_Zero(t *testing.T) {
	v := []float32{0.0, 0.0, 0.0, 0.0}
	normalized := NormalizeVector(v)

	// Should return same vector
	for i, val := range normalized {
		if val != 0.0 {
			t.Errorf("expected 0.0 at index %d, got %f", i, val)
		}
	}
}

func BenchmarkCosineDistanceSIMD(b *testing.B) {
	a := make([]float32, 1536)
	c := make([]float32, 1536)
	for i := range a {
		a[i] = float32(i) / 1536.0
		c[i] = float32(i+1) / 1536.0
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CosineDistanceSIMD(a, c)
	}
}

func BenchmarkCosineDistanceScalar(b *testing.B) {
	a := make([]float32, 1536)
	c := make([]float32, 1536)
	for i := range a {
		a[i] = float32(i) / 1536.0
		c[i] = float32(i+1) / 1536.0
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cosineDistanceScalar(a, c)
	}
}

func BenchmarkEuclideanDistanceSIMD(b *testing.B) {
	a := make([]float32, 1536)
	c := make([]float32, 1536)
	for i := range a {
		a[i] = float32(i) / 1536.0
		c[i] = float32(i+1) / 1536.0
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EuclideanDistanceSIMD(a, c)
	}
}

func BenchmarkDotProductSIMD(b *testing.B) {
	a := make([]float32, 1536)
	c := make([]float32, 1536)
	for i := range a {
		a[i] = float32(i) / 1536.0
		c[i] = float32(i+1) / 1536.0
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DotProductSIMD(a, c)
	}
}

func BenchmarkBatchCosineDistance(b *testing.B) {
	query := make([]float32, 1536)
	for i := range query {
		query[i] = float32(i) / 1536.0
	}

	vectors := make([][]float32, 1000)
	for i := range vectors {
		vectors[i] = make([]float32, 1536)
		for j := range vectors[i] {
			vectors[i][j] = float32(i+j) / 1536.0
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BatchCosineDistance(query, vectors)
	}
}

func BenchmarkNormalizeVector(b *testing.B) {
	v := make([]float32, 1536)
	for i := range v {
		v[i] = float32(i) / 1536.0
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NormalizeVector(v)
	}
}
