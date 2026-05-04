package distance

import (
	"math"
	"math/rand"
	"testing"
)

// These tests run on every supported architecture and verify that the
// public SIMD dispatchers (CosineDistanceSIMD, EuclideanDistanceSIMD,
// DotProductSIMD) agree with a hand-rolled reference implementation.
// They exist because simd_test.go is gated to //go:build amd64, which
// previously hid an arm64 NEON-assembly correctness bug for years.

func refDot(a, b []float32) float32 {
	var s float32
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

func refEuclidean(a, b []float32) float32 {
	var s float32
	for i := range a {
		d := a[i] - b[i]
		s += d * d
	}
	return float32(math.Sqrt(float64(s)))
}

func refCosineDist(a, b []float32) float32 {
	var dot, na, nb float32
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 1.0
	}
	sim := dot / (float32(math.Sqrt(float64(na))) * float32(math.Sqrt(float64(nb))))
	return 1.0 - sim
}

// approxEqual returns true when the two values agree to within an
// adaptive tolerance suitable for accumulated float32 error.
func approxEqual(a, b float32) bool {
	abs := math.Abs(float64(a - b))
	return abs <= 1e-3+1e-3*math.Abs(float64(b))
}

func TestSIMDvsScalarCosine(t *testing.T) {
	rng := rand.New(rand.NewSource(1)) //nolint:gosec
	for trial := 0; trial < 30; trial++ {
		// Mix sizes that fall above and below the SIMD threshold.
		n := 1 + rng.Intn(200)
		a := make([]float32, n)
		b := make([]float32, n)
		for i := range a {
			a[i] = rng.Float32()*4 - 2
			b[i] = rng.Float32()*4 - 2
		}
		got := CosineDistanceSIMD(a, b)
		want := refCosineDist(a, b)
		if !approxEqual(got, want) {
			t.Errorf("trial %d (n=%d): CosineDistanceSIMD=%v, want %v", trial, n, got, want)
		}
	}
}

func TestSIMDvsScalarEuclidean(t *testing.T) {
	rng := rand.New(rand.NewSource(2)) //nolint:gosec
	for trial := 0; trial < 30; trial++ {
		n := 1 + rng.Intn(200)
		a := make([]float32, n)
		b := make([]float32, n)
		for i := range a {
			a[i] = rng.Float32()*4 - 2
			b[i] = rng.Float32()*4 - 2
		}
		got := EuclideanDistanceSIMD(a, b)
		want := refEuclidean(a, b)
		if !approxEqual(got, want) {
			t.Errorf("trial %d (n=%d): EuclideanDistanceSIMD=%v, want %v", trial, n, got, want)
		}
	}
}

func TestSIMDvsScalarDotProduct(t *testing.T) {
	rng := rand.New(rand.NewSource(3)) //nolint:gosec
	for trial := 0; trial < 30; trial++ {
		n := 1 + rng.Intn(200)
		a := make([]float32, n)
		b := make([]float32, n)
		for i := range a {
			a[i] = rng.Float32()*4 - 2
			b[i] = rng.Float32()*4 - 2
		}
		got := DotProductSIMD(a, b)
		want := refDot(a, b)
		if !approxEqual(got, want) {
			t.Errorf("trial %d (n=%d): DotProductSIMD=%v, want %v", trial, n, got, want)
		}
	}
}
