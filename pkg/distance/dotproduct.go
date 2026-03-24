package distance

import (
	"github.com/limyedb/limyedb/pkg/point"
)

// DotProduct implements inner product similarity
type DotProduct struct{}

// Distance returns the negative dot product (for use as a distance)
// This allows using the same "lower is better" semantics as other distances
func (d *DotProduct) Distance(a, b point.Vector) float32 {
	return -d.Similarity(a, b)
}

// Similarity calculates the dot product between two vectors
// Higher values indicate more similar vectors
func (d *DotProduct) Similarity(a, b point.Vector) float32 {
	return DotProductSIMD(a, b)
}

// Name returns the name of this metric
func (d *DotProduct) Name() string {
	return "dot_product"
}

// IsSimilarity returns true because higher dot products mean more similar
func (d *DotProduct) IsSimilarity() bool {
	return true
}

// WeightedDotProduct calculates a weighted dot product
func WeightedDotProduct(a, b, weights point.Vector) float32 {
	if len(a) != len(b) || len(a) != len(weights) {
		return 0.0
	}

	var dot float32
	for i := range a {
		dot += a[i] * b[i] * weights[i]
	}
	return dot
}

// MaxInnerProduct is an alias for DotProduct
// Used when working with Maximum Inner Product Search (MIPS)
type MaxInnerProduct = DotProduct

// NegativeDotProduct returns negative dot product directly as distance
// Useful for maximum inner product search where we want to maximize similarity
type NegativeDotProduct struct {
	DotProduct
}

// Distance returns negative dot product
func (n *NegativeDotProduct) Distance(a, b point.Vector) float32 {
	return -n.Similarity(a, b)
}

// IsSimilarity returns false since we return distance
func (n *NegativeDotProduct) IsSimilarity() bool {
	return false
}
