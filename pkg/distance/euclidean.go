package distance

import (
	"math"

	"github.com/limyedb/limyedb/pkg/point"
)

// Euclidean implements L2 (Euclidean) distance
type Euclidean struct{}

// Distance calculates the Euclidean distance between two vectors
func (e *Euclidean) Distance(a, b point.Vector) float32 {
	return float32(math.Sqrt(float64(e.DistanceSquared(a, b))))
}

// DistanceSquared calculates the squared Euclidean distance
// This is more efficient when you only need to compare distances
func (e *Euclidean) DistanceSquared(a, b point.Vector) float32 {
	if len(a) != len(b) {
		return float32(math.MaxFloat32)
	}
	if len(a) == 0 {
		return 0.0
	}

	var sum float32

	// Process in chunks of 4 for better CPU cache utilization
	i := 0
	for ; i <= len(a)-4; i += 4 {
		d0 := a[i] - b[i]
		d1 := a[i+1] - b[i+1]
		d2 := a[i+2] - b[i+2]
		d3 := a[i+3] - b[i+3]
		sum += d0*d0 + d1*d1 + d2*d2 + d3*d3
	}

	// Handle remaining elements
	for ; i < len(a); i++ {
		d := a[i] - b[i]
		sum += d * d
	}

	return sum
}

// Name returns the name of this distance metric
func (e *Euclidean) Name() string {
	return "euclidean"
}

// IsSimilarity returns false because lower values mean more similar
func (e *Euclidean) IsSimilarity() bool {
	return false
}

// Manhattan calculates the L1 (Manhattan) distance between two vectors
type Manhattan struct{}

// Distance calculates the Manhattan distance
func (m *Manhattan) Distance(a, b point.Vector) float32 {
	if len(a) != len(b) {
		return float32(math.MaxFloat32)
	}

	var sum float32
	for i := range a {
		d := a[i] - b[i]
		if d < 0 {
			d = -d
		}
		sum += d
	}
	return sum
}

// Name returns the name of this distance metric
func (m *Manhattan) Name() string {
	return "manhattan"
}

// IsSimilarity returns false
func (m *Manhattan) IsSimilarity() bool {
	return false
}

// Chebyshev calculates the L∞ (Chebyshev) distance
type Chebyshev struct{}

// Distance calculates the Chebyshev distance (maximum absolute difference)
func (c *Chebyshev) Distance(a, b point.Vector) float32 {
	if len(a) != len(b) {
		return float32(math.MaxFloat32)
	}

	var maxDiff float32
	for i := range a {
		d := a[i] - b[i]
		if d < 0 {
			d = -d
		}
		if d > maxDiff {
			maxDiff = d
		}
	}
	return maxDiff
}

// Name returns the name of this distance metric
func (c *Chebyshev) Name() string {
	return "chebyshev"
}

// IsSimilarity returns false
func (c *Chebyshev) IsSimilarity() bool {
	return false
}

// Minkowski calculates the Lp distance for any p
type Minkowski struct {
	P float64
}

// NewMinkowski creates a new Minkowski distance calculator
func NewMinkowski(p float64) *Minkowski {
	if p < 1 {
		p = 2 // Default to Euclidean
	}
	return &Minkowski{P: p}
}

// Distance calculates the Minkowski distance
func (m *Minkowski) Distance(a, b point.Vector) float32 {
	if len(a) != len(b) {
		return float32(math.MaxFloat32)
	}

	var sum float64
	for i := range a {
		d := math.Abs(float64(a[i] - b[i]))
		sum += math.Pow(d, m.P)
	}
	return float32(math.Pow(sum, 1.0/m.P))
}

// Name returns the name of this distance metric
func (m *Minkowski) Name() string {
	return "minkowski"
}

// IsSimilarity returns false
func (m *Minkowski) IsSimilarity() bool {
	return false
}
