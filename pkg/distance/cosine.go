package distance

import (
	"math"

	"github.com/limyedb/limyedb/pkg/point"
)

// Cosine implements cosine similarity distance
type Cosine struct{}

// Distance calculates 1 - cosine_similarity between two vectors
// Returns a value in [0, 2], where 0 means identical and 2 means opposite
func (c *Cosine) Distance(a, b point.Vector) float32 {
	return CosineDistanceSIMD(a, b)
}

// Similarity calculates the cosine similarity between two vectors
// Returns a value in [-1, 1], where 1 means identical direction
func (c *Cosine) Similarity(a, b point.Vector) float32 {
	return 1.0 - CosineDistanceSIMD(a, b)
}

// SimilarityNormalized calculates cosine similarity for pre-normalized vectors
// This is more efficient when vectors are already unit length
func (c *Cosine) SimilarityNormalized(a, b point.Vector) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}

	var dot float32

	// Process in chunks of 8 for better CPU utilization
	i := 0
	for ; i <= len(a)-8; i += 8 {
		dot += a[i]*b[i] + a[i+1]*b[i+1] + a[i+2]*b[i+2] + a[i+3]*b[i+3]
		dot += a[i+4]*b[i+4] + a[i+5]*b[i+5] + a[i+6]*b[i+6] + a[i+7]*b[i+7]
	}

	// Handle remaining elements
	for ; i < len(a); i++ {
		dot += a[i] * b[i]
	}

	return dot
}

// Name returns the name of this distance metric
func (c *Cosine) Name() string {
	return "cosine"
}

// IsSimilarity returns false because we return distance (1 - similarity)
func (c *Cosine) IsSimilarity() bool {
	return false
}

// AngularDistance calculates the angular distance (angle) between two vectors
// Returns a value in [0, π]
func AngularDistance(a, b point.Vector) float32 {
	cosine := &Cosine{}
	sim := cosine.Similarity(a, b)

	// Clamp to [-1, 1] to avoid NaN from acos
	if sim > 1.0 {
		sim = 1.0
	} else if sim < -1.0 {
		sim = -1.0
	}

	return float32(math.Acos(float64(sim)))
}
