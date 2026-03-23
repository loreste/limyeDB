package distance

import (
	"math"
	"testing"

	"github.com/limyedb/limyedb/pkg/point"
)

func TestCosineDistance(t *testing.T) {
	calc := &Cosine{}

	tests := []struct {
		name     string
		a, b     point.Vector
		expected float32
		delta    float32
	}{
		{
			name:     "identical vectors",
			a:        point.Vector{1, 0, 0},
			b:        point.Vector{1, 0, 0},
			expected: 0.0,
			delta:    0.001,
		},
		{
			name:     "orthogonal vectors",
			a:        point.Vector{1, 0, 0},
			b:        point.Vector{0, 1, 0},
			expected: 1.0,
			delta:    0.001,
		},
		{
			name:     "opposite vectors",
			a:        point.Vector{1, 0, 0},
			b:        point.Vector{-1, 0, 0},
			expected: 2.0,
			delta:    0.001,
		},
		{
			name:     "similar vectors",
			a:        point.Vector{1, 0.1, 0},
			b:        point.Vector{1, 0, 0},
			expected: 0.0,
			delta:    0.05,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calc.Distance(tt.a, tt.b)
			if math.Abs(float64(got-tt.expected)) > float64(tt.delta) {
				t.Errorf("Distance() = %v, want %v (±%v)", got, tt.expected, tt.delta)
			}
		})
	}
}

func TestCosineSimilarity(t *testing.T) {
	calc := &Cosine{}

	tests := []struct {
		name     string
		a, b     point.Vector
		expected float32
		delta    float32
	}{
		{
			name:     "identical vectors",
			a:        point.Vector{1, 0, 0},
			b:        point.Vector{1, 0, 0},
			expected: 1.0,
			delta:    0.001,
		},
		{
			name:     "orthogonal vectors",
			a:        point.Vector{1, 0, 0},
			b:        point.Vector{0, 1, 0},
			expected: 0.0,
			delta:    0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calc.Similarity(tt.a, tt.b)
			if math.Abs(float64(got-tt.expected)) > float64(tt.delta) {
				t.Errorf("Similarity() = %v, want %v (±%v)", got, tt.expected, tt.delta)
			}
		})
	}
}

func TestEuclideanDistance(t *testing.T) {
	calc := &Euclidean{}

	tests := []struct {
		name     string
		a, b     point.Vector
		expected float32
		delta    float32
	}{
		{
			name:     "identical vectors",
			a:        point.Vector{0, 0, 0},
			b:        point.Vector{0, 0, 0},
			expected: 0.0,
			delta:    0.001,
		},
		{
			name:     "unit distance",
			a:        point.Vector{0, 0, 0},
			b:        point.Vector{1, 0, 0},
			expected: 1.0,
			delta:    0.001,
		},
		{
			name:     "pythagorean",
			a:        point.Vector{0, 0, 0},
			b:        point.Vector{3, 4, 0},
			expected: 5.0,
			delta:    0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calc.Distance(tt.a, tt.b)
			if math.Abs(float64(got-tt.expected)) > float64(tt.delta) {
				t.Errorf("Distance() = %v, want %v (±%v)", got, tt.expected, tt.delta)
			}
		})
	}
}

func TestDotProduct(t *testing.T) {
	calc := &DotProduct{}

	tests := []struct {
		name     string
		a, b     point.Vector
		expected float32
	}{
		{
			name:     "orthogonal vectors",
			a:        point.Vector{1, 0, 0},
			b:        point.Vector{0, 1, 0},
			expected: 0.0,
		},
		{
			name:     "parallel vectors",
			a:        point.Vector{1, 2, 3},
			b:        point.Vector{1, 2, 3},
			expected: 14.0, // 1+4+9
		},
		{
			name:     "mixed",
			a:        point.Vector{1, 2, 3},
			b:        point.Vector{4, 5, 6},
			expected: 32.0, // 4+10+18
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calc.Similarity(tt.a, tt.b)
			if got != tt.expected {
				t.Errorf("Similarity() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBatchDistance(t *testing.T) {
	calc := &Cosine{}
	query := point.Vector{1, 0, 0}
	vectors := []point.Vector{
		{1, 0, 0},
		{0, 1, 0},
		{-1, 0, 0},
	}

	results := BatchDistance(calc, query, vectors)

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	// First should be 0 (identical)
	if results[0] > 0.001 {
		t.Errorf("Expected ~0 for identical vector, got %v", results[0])
	}

	// Second should be 1 (orthogonal)
	if math.Abs(float64(results[1]-1.0)) > 0.001 {
		t.Errorf("Expected ~1 for orthogonal vector, got %v", results[1])
	}
}

func TestFindNearest(t *testing.T) {
	calc := &Cosine{}
	query := point.Vector{1, 0, 0}
	vectors := []point.Vector{
		{0, 1, 0},    // index 0 - orthogonal
		{1, 0, 0},    // index 1 - identical
		{0.9, 0.1, 0}, // index 2 - similar
	}

	indices := FindNearest(calc, query, vectors, 2)

	if len(indices) != 2 {
		t.Errorf("Expected 2 indices, got %d", len(indices))
	}

	// First should be index 1 (identical)
	if indices[0] != 1 {
		t.Errorf("Expected first nearest to be index 1, got %d", indices[0])
	}
}

func TestNormalize(t *testing.T) {
	v := point.Vector{3, 4, 0}
	normalized := Normalize(v)

	// Check unit length
	var sum float32
	for _, val := range normalized {
		sum += val * val
	}
	magnitude := float32(math.Sqrt(float64(sum)))

	if math.Abs(float64(magnitude-1.0)) > 0.001 {
		t.Errorf("Expected unit magnitude, got %v", magnitude)
	}

	// Check direction preserved
	expectedX := float32(0.6) // 3/5
	expectedY := float32(0.8) // 4/5
	if math.Abs(float64(normalized[0]-expectedX)) > 0.001 {
		t.Errorf("Expected x=%v, got %v", expectedX, normalized[0])
	}
	if math.Abs(float64(normalized[1]-expectedY)) > 0.001 {
		t.Errorf("Expected y=%v, got %v", expectedY, normalized[1])
	}
}
