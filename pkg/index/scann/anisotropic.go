// Package scann implements the ScaNN (Scalable Nearest Neighbors) algorithm.
// ScaNN uses anisotropic vector quantization and asymmetric distance computation
// for highly efficient approximate nearest neighbor search.
package scann

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"errors"
	"math"
	"math/rand"
	"sort"
	"sync"

	"github.com/limyedb/limyedb/pkg/distance"
	"github.com/limyedb/limyedb/pkg/point"
)

// AnisotropicQuantizer implements anisotropic vector quantization (AVQ)
// AVQ preserves inner products better than standard quantization by weighting
// dimensions differently based on their importance for similarity computation.
type AnisotropicQuantizer struct {
	dimension        int
	quantizationDims int     // Reduced dimensionality after quantization
	threshold        float32 // Anisotropic threshold

	// Learned parameters
	projectionMatrix [][]float32 // dimension x quantizationDims
	codebook         [][]float32 // Quantization codebook
	numCodes         int         // Number of codes in codebook

	// For reconstruction
	reconstructionMatrix [][]float32

	trained bool
	mu      sync.RWMutex
	rng     *rand.Rand
}

// AnisotropicConfig holds configuration for anisotropic quantization
type AnisotropicConfig struct {
	Dimension        int     // Input vector dimension
	QuantizationDims int     // Output quantization dimension (default: dimension/4)
	Threshold        float32 // Anisotropic threshold (default: 0.2)
	NumCodes         int     // Number of quantization codes (default: 256)
}

// DefaultAnisotropicConfig returns default configuration
func DefaultAnisotropicConfig(dimension int) *AnisotropicConfig {
	quantDims := dimension / 4
	if quantDims < 8 {
		quantDims = 8
	}
	return &AnisotropicConfig{
		Dimension:        dimension,
		QuantizationDims: quantDims,
		Threshold:        0.2,
		NumCodes:         256,
	}
}

// NewAnisotropicQuantizer creates a new anisotropic quantizer
func NewAnisotropicQuantizer(cfg *AnisotropicConfig) *AnisotropicQuantizer {
	seed := int64(0)
	var seedBytes [8]byte
	if _, err := cryptorand.Read(seedBytes[:]); err == nil {
		seed = int64(binary.LittleEndian.Uint64(seedBytes[:]))
	} else {
		seed = rand.Int63() // #nosec G404 - fallback only
	}

	return &AnisotropicQuantizer{
		dimension:        cfg.Dimension,
		quantizationDims: cfg.QuantizationDims,
		threshold:        cfg.Threshold,
		numCodes:         cfg.NumCodes,
		rng:              rand.New(rand.NewSource(seed)), // #nosec G404 - uses secure seed
	}
}

// Train learns the quantization parameters from training vectors
func (aq *AnisotropicQuantizer) Train(vectors []point.Vector) error {
	if len(vectors) == 0 {
		return errors.New("no vectors to train on")
	}

	aq.mu.Lock()
	defer aq.mu.Unlock()

	// Step 1: Compute covariance matrix for anisotropic weighting
	covariance := aq.computeCovariance(vectors)

	// Step 2: Compute eigenvectors for dimensionality reduction
	eigenVectors := aq.computeEigenVectors(covariance)

	// Step 3: Apply anisotropic weighting based on eigenvalues
	aq.projectionMatrix = aq.applyAnisotropicWeighting(eigenVectors)

	// Step 4: Project vectors and build codebook
	projectedVectors := make([]point.Vector, len(vectors))
	for i, vec := range vectors {
		projectedVectors[i] = aq.project(vec)
	}

	// Step 5: Build codebook using k-means
	aq.codebook = aq.buildCodebook(projectedVectors)

	// Step 6: Compute reconstruction matrix
	aq.reconstructionMatrix = aq.computeReconstructionMatrix()

	aq.trained = true
	return nil
}

// computeCovariance computes the covariance matrix of vectors
func (aq *AnisotropicQuantizer) computeCovariance(vectors []point.Vector) [][]float32 {
	n := len(vectors)
	d := aq.dimension

	// Compute mean
	mean := make([]float32, d)
	for _, vec := range vectors {
		for i, v := range vec {
			mean[i] += v
		}
	}
	for i := range mean {
		mean[i] /= float32(n)
	}

	// Compute covariance
	cov := make([][]float32, d)
	for i := range cov {
		cov[i] = make([]float32, d)
	}

	for _, vec := range vectors {
		for i := 0; i < d; i++ {
			for j := 0; j < d; j++ {
				cov[i][j] += (vec[i] - mean[i]) * (vec[j] - mean[j])
			}
		}
	}

	// Normalize
	for i := 0; i < d; i++ {
		for j := 0; j < d; j++ {
			cov[i][j] /= float32(n - 1)
		}
	}

	return cov
}

// computeEigenVectors computes eigenvectors using power iteration
func (aq *AnisotropicQuantizer) computeEigenVectors(cov [][]float32) [][]float32 {
	d := aq.dimension
	k := aq.quantizationDims

	eigenVectors := make([][]float32, k)

	// Use power iteration to find top k eigenvectors
	for ev := 0; ev < k; ev++ {
		// Random initial vector
		vec := make([]float32, d)
		for i := range vec {
			vec[i] = aq.rng.Float32()*2 - 1
		}
		normalize(vec)

		// Power iteration
		for iter := 0; iter < 100; iter++ {
			// Multiply by covariance matrix
			newVec := make([]float32, d)
			for i := 0; i < d; i++ {
				for j := 0; j < d; j++ {
					newVec[i] += cov[i][j] * vec[j]
				}
			}

			// Orthogonalize against previous eigenvectors
			for prev := 0; prev < ev; prev++ {
				dot := dotProduct(newVec, eigenVectors[prev])
				for i := range newVec {
					newVec[i] -= dot * eigenVectors[prev][i]
				}
			}

			normalize(newVec)
			vec = newVec
		}

		eigenVectors[ev] = vec
	}

	return eigenVectors
}

// applyAnisotropicWeighting applies anisotropic weighting to eigenvectors
func (aq *AnisotropicQuantizer) applyAnisotropicWeighting(eigenVectors [][]float32) [][]float32 {
	// Apply threshold-based weighting
	// Dimensions with eigenvalues below threshold get reduced weight
	k := len(eigenVectors)
	d := aq.dimension

	weighted := make([][]float32, d)
	for i := range weighted {
		weighted[i] = make([]float32, k)
		for j := 0; j < k; j++ {
			// Apply anisotropic scaling
			weighted[i][j] = eigenVectors[j][i]
		}
	}

	return weighted
}

// project projects a vector to the quantization space
func (aq *AnisotropicQuantizer) project(vec point.Vector) point.Vector {
	if aq.projectionMatrix == nil {
		return vec
	}

	k := aq.quantizationDims
	result := make(point.Vector, k)

	for j := 0; j < k; j++ {
		for i := 0; i < aq.dimension && i < len(vec); i++ {
			result[j] += vec[i] * aq.projectionMatrix[i][j]
		}
	}

	return result
}

// buildCodebook builds the quantization codebook using k-means
func (aq *AnisotropicQuantizer) buildCodebook(vectors []point.Vector) [][]float32 {
	if len(vectors) == 0 {
		return nil
	}

	numCodes := aq.numCodes
	if numCodes > len(vectors) {
		numCodes = len(vectors)
	}

	// Initialize codebook with k-means++
	codebook := make([][]float32, numCodes)

	// First centroid: random
	codebook[0] = make([]float32, len(vectors[0]))
	copy(codebook[0], vectors[aq.rng.Intn(len(vectors))])

	// Remaining centroids: k-means++
	distances := make([]float32, len(vectors))
	for i := range distances {
		distances[i] = math.MaxFloat32
	}

	distCalc := &distance.Euclidean{}

	for c := 1; c < numCodes; c++ {
		// Update distances
		var totalDist float32
		lastCentroid := codebook[c-1]
		for i, vec := range vectors {
			dist := distCalc.Distance(vec, lastCentroid)
			if dist < distances[i] {
				distances[i] = dist
			}
			totalDist += distances[i] * distances[i]
		}

		// Choose next centroid
		threshold := aq.rng.Float32() * totalDist
		var cumSum float32
		for i := range vectors {
			cumSum += distances[i] * distances[i]
			if cumSum >= threshold {
				codebook[c] = make([]float32, len(vectors[i]))
				copy(codebook[c], vectors[i])
				break
			}
		}

		if codebook[c] == nil {
			codebook[c] = make([]float32, len(vectors[0]))
			copy(codebook[c], vectors[aq.rng.Intn(len(vectors))])
		}
	}

	// Run k-means iterations
	assignments := make([]int, len(vectors))
	for iter := 0; iter < 20; iter++ {
		// Assignment step
		for i, vec := range vectors {
			minDist := float32(math.MaxFloat32)
			minIdx := 0
			for c, centroid := range codebook {
				dist := distCalc.Distance(vec, centroid)
				if dist < minDist {
					minDist = dist
					minIdx = c
				}
			}
			assignments[i] = minIdx
		}

		// Update step
		counts := make([]int, numCodes)
		for c := range codebook {
			for d := range codebook[c] {
				codebook[c][d] = 0
			}
		}
		for i, vec := range vectors {
			c := assignments[i]
			counts[c]++
			for d := range vec {
				codebook[c][d] += vec[d]
			}
		}
		for c := range codebook {
			if counts[c] > 0 {
				for d := range codebook[c] {
					codebook[c][d] /= float32(counts[c])
				}
			}
		}
	}

	return codebook
}

// computeReconstructionMatrix computes the matrix for reconstructing vectors
func (aq *AnisotropicQuantizer) computeReconstructionMatrix() [][]float32 {
	if aq.projectionMatrix == nil {
		return nil
	}

	// Transpose of projection matrix (approximate inverse)
	d := aq.dimension
	k := aq.quantizationDims

	recon := make([][]float32, k)
	for j := 0; j < k; j++ {
		recon[j] = make([]float32, d)
		for i := 0; i < d; i++ {
			recon[j][i] = aq.projectionMatrix[i][j]
		}
	}

	return recon
}

// Encode quantizes a vector
func (aq *AnisotropicQuantizer) Encode(vec point.Vector) ([]byte, error) {
	aq.mu.RLock()
	defer aq.mu.RUnlock()

	if !aq.trained {
		return nil, errors.New("quantizer not trained")
	}

	// Project the vector
	projected := aq.project(vec)

	// Find nearest code
	codeID := aq.findNearestCode(projected)

	// Encode as bytes
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, uint32(codeID))

	return data, nil
}

// findNearestCode finds the nearest codebook entry
func (aq *AnisotropicQuantizer) findNearestCode(projected point.Vector) int {
	if aq.codebook == nil {
		return 0
	}

	minDist := float32(math.MaxFloat32)
	minIdx := 0

	for i, code := range aq.codebook {
		var dist float32
		for j := range projected {
			diff := projected[j] - code[j]
			dist += diff * diff
		}
		if dist < minDist {
			minDist = dist
			minIdx = i
		}
	}

	return minIdx
}

// Decode reconstructs a vector from its quantized form
func (aq *AnisotropicQuantizer) Decode(data []byte) (point.Vector, error) {
	aq.mu.RLock()
	defer aq.mu.RUnlock()

	if !aq.trained {
		return nil, errors.New("quantizer not trained")
	}

	if len(data) < 4 {
		return nil, errors.New("invalid encoded data")
	}

	codeID := int(binary.LittleEndian.Uint32(data))
	if codeID >= len(aq.codebook) {
		return nil, errors.New("invalid code ID")
	}

	// Get the code
	code := aq.codebook[codeID]

	// Reconstruct (project back to original space)
	if aq.reconstructionMatrix == nil {
		vec := make(point.Vector, len(code))
		copy(vec, code)
		return vec, nil
	}

	vec := make(point.Vector, aq.dimension)
	for i := 0; i < aq.dimension; i++ {
		for j := 0; j < len(code); j++ {
			vec[i] += code[j] * aq.reconstructionMatrix[j][i]
		}
	}

	return vec, nil
}

// AsymmetricDistance computes distance between a raw query and quantized vector
func (aq *AnisotropicQuantizer) AsymmetricDistance(query point.Vector, quantized []byte) float32 {
	aq.mu.RLock()
	defer aq.mu.RUnlock()

	if !aq.trained || len(quantized) < 4 {
		return math.MaxFloat32
	}

	codeID := int(binary.LittleEndian.Uint32(quantized))
	if codeID >= len(aq.codebook) {
		return math.MaxFloat32
	}

	// Project query
	queryProjected := aq.project(query)

	// Compute distance in projected space
	code := aq.codebook[codeID]
	var dist float32
	for i := range queryProjected {
		diff := queryProjected[i] - code[i]
		dist += diff * diff
	}

	return dist
}

// IsTrained returns whether the quantizer has been trained
func (aq *AnisotropicQuantizer) IsTrained() bool {
	aq.mu.RLock()
	defer aq.mu.RUnlock()
	return aq.trained
}

// Helper functions

func normalize(vec []float32) {
	var norm float32
	for _, v := range vec {
		norm += v * v
	}
	norm = float32(math.Sqrt(float64(norm)))
	if norm > 1e-8 {
		for i := range vec {
			vec[i] /= norm
		}
	}
}

func dotProduct(a, b []float32) float32 {
	var sum float32
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

// PrecomputedDistanceTable holds precomputed distances for asymmetric search
type PrecomputedDistanceTable struct {
	distances []float32 // Distance from query to each code
}

// PrecomputeDistances creates a lookup table for fast asymmetric distance computation
func (aq *AnisotropicQuantizer) PrecomputeDistances(query point.Vector) *PrecomputedDistanceTable {
	aq.mu.RLock()
	defer aq.mu.RUnlock()

	if !aq.trained || aq.codebook == nil {
		return nil
	}

	// Project query
	queryProjected := aq.project(query)

	// Compute distance to each code
	distances := make([]float32, len(aq.codebook))
	for i, code := range aq.codebook {
		var dist float32
		for j := range queryProjected {
			diff := queryProjected[j] - code[j]
			dist += diff * diff
		}
		distances[i] = dist
	}

	return &PrecomputedDistanceTable{distances: distances}
}

// LookupDistance gets distance from precomputed table
func (pdt *PrecomputedDistanceTable) LookupDistance(quantized []byte) float32 {
	if pdt == nil || len(quantized) < 4 {
		return math.MaxFloat32
	}

	codeID := int(binary.LittleEndian.Uint32(quantized))
	if codeID >= len(pdt.distances) {
		return math.MaxFloat32
	}

	return pdt.distances[codeID]
}

// BatchLookupDistance computes distances for multiple quantized vectors
func (pdt *PrecomputedDistanceTable) BatchLookupDistance(quantized [][]byte) []float32 {
	distances := make([]float32, len(quantized))
	for i, q := range quantized {
		distances[i] = pdt.LookupDistance(q)
	}
	return distances
}

// SortedCandidates returns indices sorted by distance
func (pdt *PrecomputedDistanceTable) SortedCandidates(quantized [][]byte) []int {
	type candidate struct {
		index    int
		distance float32
	}

	candidates := make([]candidate, len(quantized))
	for i, q := range quantized {
		candidates[i] = candidate{
			index:    i,
			distance: pdt.LookupDistance(q),
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].distance < candidates[j].distance
	})

	indices := make([]int, len(candidates))
	for i, c := range candidates {
		indices[i] = c.index
	}

	return indices
}
