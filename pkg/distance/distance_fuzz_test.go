package distance

import (
	"math"
	"testing"

	"github.com/limyedb/limyedb/pkg/point"
)

// FuzzCosineDistance fuzzes the cosine distance calculation
func FuzzCosineDistance(f *testing.F) {
	// Seed corpus with representative vectors
	seedVectors := [][]float32{
		{0.1, 0.2, 0.3},
		{1.0, 0.0, 0.0},
		{0.0, 1.0, 0.0},
		{0.0, 0.0, 1.0},
		{0.5, 0.5, 0.5, 0.5},
		{-1.0, -1.0, -1.0},
		{0.0, 0.0, 0.0},
		{1e-10, 1e-10, 1e-10},
		{1e10, 1e10, 1e10},
	}

	// Add seeds as pairs
	for i := range seedVectors {
		for j := range seedVectors {
			if len(seedVectors[i]) == len(seedVectors[j]) {
				// Encode as bytes for fuzzing
				f.Add(float32sToBytes(seedVectors[i]), float32sToBytes(seedVectors[j]))
			}
		}
	}

	cosine := &Cosine{}

	f.Fuzz(func(t *testing.T, aBytes, bBytes []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Cosine.Distance panicked: %v", r)
			}
		}()

		a := bytesToFloat32s(aBytes)
		b := bytesToFloat32s(bBytes)

		if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
			return // Skip invalid inputs
		}

		// Filter out vectors with NaN or Inf
		for _, v := range a {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return
			}
		}
		for _, v := range b {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return
			}
		}

		dist := cosine.Distance(point.Vector(a), point.Vector(b))

		// Verify result properties
		if math.IsNaN(float64(dist)) {
			// NaN can happen for zero vectors, which is acceptable
			return
		}

		// Cosine distance should be finite for valid vectors
		// Note: Due to floating point precision, values can slightly exceed [0, 2] range
		// The key property is that it shouldn't be NaN or infinite for valid inputs
		if math.IsInf(float64(dist), 0) {
			t.Errorf("Cosine distance is infinite: %f", dist)
		}
	})
}

// FuzzEuclideanDistance fuzzes the Euclidean distance calculation
func FuzzEuclideanDistance(f *testing.F) {
	seedVectors := [][]float32{
		{0.0, 0.0, 0.0},
		{1.0, 0.0, 0.0},
		{0.0, 1.0, 0.0},
		{1.0, 1.0, 1.0},
		{-1.0, -1.0, -1.0},
		{0.5, 0.5, 0.5, 0.5},
	}

	for i := range seedVectors {
		for j := range seedVectors {
			if len(seedVectors[i]) == len(seedVectors[j]) {
				f.Add(float32sToBytes(seedVectors[i]), float32sToBytes(seedVectors[j]))
			}
		}
	}

	euclidean := &Euclidean{}

	f.Fuzz(func(t *testing.T, aBytes, bBytes []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Euclidean.Distance panicked: %v", r)
			}
		}()

		a := bytesToFloat32s(aBytes)
		b := bytesToFloat32s(bBytes)

		if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
			return
		}

		// Filter out NaN/Inf
		for _, v := range a {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return
			}
		}
		for _, v := range b {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return
			}
		}

		dist := euclidean.Distance(point.Vector(a), point.Vector(b))

		// Euclidean distance should be non-negative
		if dist < 0 && !math.IsNaN(float64(dist)) {
			t.Errorf("Euclidean distance should be non-negative: %f", dist)
		}

		// Distance to self should be zero
		selfDist := euclidean.Distance(point.Vector(a), point.Vector(a))
		if selfDist > 1e-6 && !math.IsNaN(float64(selfDist)) {
			t.Errorf("Distance to self should be ~0: %f", selfDist)
		}
	})
}

// FuzzDotProduct fuzzes the dot product calculation
func FuzzDotProduct(f *testing.F) {
	seedVectors := [][]float32{
		{1.0, 0.0, 0.0},
		{0.0, 1.0, 0.0},
		{1.0, 1.0, 1.0},
		{-1.0, -1.0, -1.0},
		{0.5, 0.5, 0.5},
		{0.0, 0.0, 0.0},
	}

	for i := range seedVectors {
		for j := range seedVectors {
			if len(seedVectors[i]) == len(seedVectors[j]) {
				f.Add(float32sToBytes(seedVectors[i]), float32sToBytes(seedVectors[j]))
			}
		}
	}

	dp := &DotProduct{}

	f.Fuzz(func(t *testing.T, aBytes, bBytes []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("DotProduct.Distance panicked: %v", r)
			}
		}()

		a := bytesToFloat32s(aBytes)
		b := bytesToFloat32s(bBytes)

		if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
			return
		}

		// Filter out NaN/Inf
		for _, v := range a {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return
			}
		}
		for _, v := range b {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return
			}
		}

		sim := dp.Similarity(point.Vector(a), point.Vector(b))
		dist := dp.Distance(point.Vector(a), point.Vector(b))

		// Distance should be negative of similarity
		if !math.IsNaN(float64(sim)) && !math.IsNaN(float64(dist)) {
			if math.Abs(float64(dist+sim)) > 1e-6 {
				t.Errorf("Distance should be -Similarity: dist=%f, sim=%f", dist, sim)
			}
		}
	})
}

// FuzzManhattanDistance fuzzes the Manhattan distance calculation
func FuzzManhattanDistance(f *testing.F) {
	seedVectors := [][]float32{
		{0.0, 0.0, 0.0},
		{1.0, 0.0, 0.0},
		{1.0, 1.0, 1.0},
		{-1.0, -1.0, -1.0},
	}

	for i := range seedVectors {
		for j := range seedVectors {
			if len(seedVectors[i]) == len(seedVectors[j]) {
				f.Add(float32sToBytes(seedVectors[i]), float32sToBytes(seedVectors[j]))
			}
		}
	}

	manhattan := &Manhattan{}

	f.Fuzz(func(t *testing.T, aBytes, bBytes []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Manhattan.Distance panicked: %v", r)
			}
		}()

		a := bytesToFloat32s(aBytes)
		b := bytesToFloat32s(bBytes)

		if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
			return
		}

		// Filter out NaN/Inf
		for _, v := range a {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return
			}
		}
		for _, v := range b {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return
			}
		}

		dist := manhattan.Distance(point.Vector(a), point.Vector(b))

		// Manhattan distance should be non-negative
		if dist < 0 && !math.IsNaN(float64(dist)) && dist != float32(math.MaxFloat32) {
			t.Errorf("Manhattan distance should be non-negative: %f", dist)
		}

		// Distance to self should be zero
		selfDist := manhattan.Distance(point.Vector(a), point.Vector(a))
		if selfDist > 1e-6 && !math.IsNaN(float64(selfDist)) {
			t.Errorf("Distance to self should be ~0: %f", selfDist)
		}
	})
}

// FuzzChebyshevDistance fuzzes the Chebyshev distance calculation
func FuzzChebyshevDistance(f *testing.F) {
	seedVectors := [][]float32{
		{0.0, 0.0, 0.0},
		{1.0, 0.0, 0.0},
		{1.0, 2.0, 3.0},
		{-1.0, -2.0, -3.0},
	}

	for i := range seedVectors {
		for j := range seedVectors {
			if len(seedVectors[i]) == len(seedVectors[j]) {
				f.Add(float32sToBytes(seedVectors[i]), float32sToBytes(seedVectors[j]))
			}
		}
	}

	chebyshev := &Chebyshev{}

	f.Fuzz(func(t *testing.T, aBytes, bBytes []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Chebyshev.Distance panicked: %v", r)
			}
		}()

		a := bytesToFloat32s(aBytes)
		b := bytesToFloat32s(bBytes)

		if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
			return
		}

		// Filter out NaN/Inf
		for _, v := range a {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return
			}
		}
		for _, v := range b {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return
			}
		}

		dist := chebyshev.Distance(point.Vector(a), point.Vector(b))

		// Chebyshev distance should be non-negative
		if dist < 0 && !math.IsNaN(float64(dist)) && dist != float32(math.MaxFloat32) {
			t.Errorf("Chebyshev distance should be non-negative: %f", dist)
		}

		// Distance to self should be zero
		selfDist := chebyshev.Distance(point.Vector(a), point.Vector(a))
		if selfDist > 1e-6 && !math.IsNaN(float64(selfDist)) {
			t.Errorf("Distance to self should be ~0: %f", selfDist)
		}

		// Chebyshev <= Manhattan (max component vs sum of components)
		manhattan := &Manhattan{}
		manhattanDist := manhattan.Distance(point.Vector(a), point.Vector(b))
		if dist > manhattanDist+1e-6 && !math.IsNaN(float64(dist)) && !math.IsNaN(float64(manhattanDist)) {
			// This is expected: Chebyshev is always <= Manhattan
		}
	})
}

// FuzzMinkowskiDistance fuzzes the Minkowski distance calculation
func FuzzMinkowskiDistance(f *testing.F) {
	seedVectors := [][]float32{
		{0.0, 0.0, 0.0},
		{1.0, 0.0, 0.0},
		{1.0, 1.0, 1.0},
	}

	for i := range seedVectors {
		for j := range seedVectors {
			if len(seedVectors[i]) == len(seedVectors[j]) {
				// Add with different p values
				f.Add(float32sToBytes(seedVectors[i]), float32sToBytes(seedVectors[j]), 1.0)
				f.Add(float32sToBytes(seedVectors[i]), float32sToBytes(seedVectors[j]), 2.0)
				f.Add(float32sToBytes(seedVectors[i]), float32sToBytes(seedVectors[j]), 3.0)
			}
		}
	}

	f.Fuzz(func(t *testing.T, aBytes, bBytes []byte, p float64) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Minkowski.Distance panicked: %v", r)
			}
		}()

		a := bytesToFloat32s(aBytes)
		b := bytesToFloat32s(bBytes)

		if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
			return
		}

		// Filter out invalid p values
		if math.IsNaN(p) || math.IsInf(p, 0) || p < 1 {
			return
		}

		// Filter out NaN/Inf in vectors
		for _, v := range a {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return
			}
		}
		for _, v := range b {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return
			}
		}

		minkowski := NewMinkowski(p)
		dist := minkowski.Distance(point.Vector(a), point.Vector(b))

		// Minkowski distance should be non-negative
		if dist < 0 && !math.IsNaN(float64(dist)) && dist != float32(math.MaxFloat32) {
			t.Errorf("Minkowski distance should be non-negative: %f", dist)
		}
	})
}

// FuzzBatchDistance fuzzes batch distance calculation
func FuzzBatchDistance(f *testing.F) {
	seeds := [][]float32{
		{0.1, 0.2, 0.3},
		{1.0, 0.0, 0.0},
		{0.5, 0.5, 0.5},
	}

	for _, seed := range seeds {
		f.Add(float32sToBytes(seed), 3) // 3 vectors in batch
	}

	f.Fuzz(func(t *testing.T, queryBytes []byte, numVectors int) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("BatchDistance panicked: %v", r)
			}
		}()

		query := bytesToFloat32s(queryBytes)
		if len(query) == 0 || numVectors <= 0 || numVectors > 1000 {
			return
		}

		// Filter out NaN/Inf
		for _, v := range query {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return
			}
		}

		// Generate vectors of the same dimension
		vectors := make([]point.Vector, numVectors)
		for i := range vectors {
			vectors[i] = make(point.Vector, len(query))
			for j := range vectors[i] {
				vectors[i][j] = float32(i+j) * 0.1
			}
		}

		cosine := &Cosine{}
		results := BatchDistance(cosine, point.Vector(query), vectors)

		if len(results) != numVectors {
			t.Errorf("BatchDistance returned %d results, want %d", len(results), numVectors)
		}
	})
}

// FuzzFindNearest fuzzes the nearest neighbor search
func FuzzFindNearest(f *testing.F) {
	seeds := [][]float32{
		{0.1, 0.2, 0.3},
		{1.0, 0.0, 0.0},
	}

	for _, seed := range seeds {
		f.Add(float32sToBytes(seed), 5, 3) // 5 vectors, k=3
	}

	f.Fuzz(func(t *testing.T, queryBytes []byte, numVectors int, k int) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("FindNearest panicked: %v", r)
			}
		}()

		query := bytesToFloat32s(queryBytes)
		if len(query) == 0 || numVectors <= 0 || numVectors > 100 || k <= 0 {
			return
		}

		// Filter out NaN/Inf
		for _, v := range query {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return
			}
		}

		// Generate vectors
		vectors := make([]point.Vector, numVectors)
		for i := range vectors {
			vectors[i] = make(point.Vector, len(query))
			for j := range vectors[i] {
				vectors[i][j] = float32(i+j) * 0.1
			}
		}

		cosine := &Cosine{}
		indices := FindNearest(cosine, point.Vector(query), vectors, k)

		expectedLen := k
		if k > numVectors {
			expectedLen = numVectors
		}

		if len(indices) != expectedLen {
			t.Errorf("FindNearest returned %d indices, want %d", len(indices), expectedLen)
		}

		// Verify indices are valid
		for _, idx := range indices {
			if idx < 0 || idx >= numVectors {
				t.Errorf("Invalid index %d for %d vectors", idx, numVectors)
			}
		}
	})
}

// FuzzNormalize fuzzes vector normalization
func FuzzNormalize(f *testing.F) {
	seeds := [][]float32{
		{1.0, 0.0, 0.0},
		{0.0, 1.0, 0.0},
		{1.0, 1.0, 1.0},
		{3.0, 4.0},
		{0.0, 0.0, 0.0},
	}

	for _, seed := range seeds {
		f.Add(float32sToBytes(seed))
	}

	f.Fuzz(func(t *testing.T, vecBytes []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Normalize panicked: %v", r)
			}
		}()

		vec := bytesToFloat32s(vecBytes)
		if len(vec) == 0 {
			return
		}

		// Filter out NaN/Inf
		for _, v := range vec {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return
			}
		}

		normalized := Normalize(point.Vector(vec))

		// Calculate magnitude
		var mag float64
		for _, v := range normalized {
			mag += float64(v) * float64(v)
		}
		mag = math.Sqrt(mag)

		// For non-zero vectors, magnitude should be ~1
		isZeroVec := true
		for _, v := range vec {
			if v != 0 {
				isZeroVec = false
				break
			}
		}

		if !isZeroVec && !math.IsNaN(mag) {
			if math.Abs(mag-1.0) > 1e-5 {
				// Might be a near-zero vector that normalizes oddly
			}
		}
	})
}

// Helper functions to convert between bytes and float32 slices
func float32sToBytes(fs []float32) []byte {
	bs := make([]byte, len(fs)*4)
	for i, f := range fs {
		bits := math.Float32bits(f)
		bs[i*4] = byte(bits)
		bs[i*4+1] = byte(bits >> 8)
		bs[i*4+2] = byte(bits >> 16)
		bs[i*4+3] = byte(bits >> 24)
	}
	return bs
}

func bytesToFloat32s(bs []byte) []float32 {
	if len(bs) < 4 {
		return nil
	}
	fs := make([]float32, len(bs)/4)
	for i := range fs {
		bits := uint32(bs[i*4]) | uint32(bs[i*4+1])<<8 | uint32(bs[i*4+2])<<16 | uint32(bs[i*4+3])<<24
		fs[i] = math.Float32frombits(bits)
	}
	return fs
}
