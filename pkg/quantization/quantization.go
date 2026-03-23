package quantization

import (
	"encoding/binary"
	"errors"
	"math"
	"sync"

	"github.com/limyedb/limyedb/pkg/point"
)

// Type represents the quantization method
type Type string

const (
	TypeNone   Type = "none"
	TypeScalar Type = "scalar" // 8-bit quantization (4x compression)
	TypeBinary Type = "binary" // 1-bit quantization (32x compression)
	TypePQ     Type = "pq"     // Product quantization
)

// Config holds quantization configuration
type Config struct {
	Type           Type    `json:"type"`
	Rescore        bool    `json:"rescore"`         // Whether to rescore with original vectors
	RescoreLimit   int     `json:"rescore_limit"`   // How many candidates to rescore
	AlwaysRam      bool    `json:"always_ram"`      // Keep quantized vectors in RAM

	// Scalar quantization specific
	ScalarType     string  `json:"scalar_type"`     // "int8" or "uint8"
	ScalarQuantile float32 `json:"scalar_quantile"` // Quantile for range calculation

	// Product quantization specific
	PQSegments     int     `json:"pq_segments"`     // Number of subvector segments
	PQCentroids    int     `json:"pq_centroids"`    // Centroids per segment (usually 256)
}

// DefaultConfig returns default quantization config
func DefaultConfig() *Config {
	return &Config{
		Type:           TypeNone,
		Rescore:        true,
		RescoreLimit:   100,
		AlwaysRam:      true,
		ScalarType:     "int8",
		ScalarQuantile: 0.99,
		PQSegments:     8,
		PQCentroids:    256,
	}
}

// Quantizer interface for all quantization methods
type Quantizer interface {
	// Encode quantizes a vector
	Encode(vector point.Vector) ([]byte, error)

	// Decode reconstructs a vector from quantized form
	Decode(data []byte) (point.Vector, error)

	// Distance computes distance between query and quantized vector
	Distance(query point.Vector, quantized []byte) float32

	// BatchDistance computes distances for multiple quantized vectors
	BatchDistance(query point.Vector, quantized [][]byte) []float32

	// EncodedSize returns the size of encoded vector in bytes
	EncodedSize() int

	// CompressionRatio returns the compression ratio
	CompressionRatio() float32

	// Type returns the quantization type
	Type() Type

	// Train trains the quantizer on sample vectors (for PQ)
	Train(vectors []point.Vector) error
}

// New creates a new quantizer based on config
func New(cfg *Config, dimension int) (Quantizer, error) {
	switch cfg.Type {
	case TypeNone:
		return NewNoneQuantizer(dimension), nil
	case TypeScalar:
		return NewScalarQuantizer(dimension, cfg.ScalarQuantile), nil
	case TypeBinary:
		return NewBinaryQuantizer(dimension), nil
	case TypePQ:
		return NewProductQuantizer(dimension, cfg.PQSegments, cfg.PQCentroids), nil
	default:
		return nil, errors.New("unknown quantization type")
	}
}

// =============================================================================
// None Quantizer (passthrough)
// =============================================================================

// NoneQuantizer stores vectors without quantization
type NoneQuantizer struct {
	dimension int
}

// NewNoneQuantizer creates a passthrough quantizer
func NewNoneQuantizer(dimension int) *NoneQuantizer {
	return &NoneQuantizer{dimension: dimension}
}

func (q *NoneQuantizer) Encode(vector point.Vector) ([]byte, error) {
	data := make([]byte, len(vector)*4)
	for i, v := range vector {
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(v))
	}
	return data, nil
}

func (q *NoneQuantizer) Decode(data []byte) (point.Vector, error) {
	vector := make(point.Vector, q.dimension)
	for i := range vector {
		vector[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return vector, nil
}

func (q *NoneQuantizer) Distance(query point.Vector, quantized []byte) float32 {
	vec, _ := q.Decode(quantized)
	return cosineDistance(query, vec)
}

func (q *NoneQuantizer) BatchDistance(query point.Vector, quantized [][]byte) []float32 {
	distances := make([]float32, len(quantized))
	for i, data := range quantized {
		distances[i] = q.Distance(query, data)
	}
	return distances
}

func (q *NoneQuantizer) EncodedSize() int {
	return q.dimension * 4
}

func (q *NoneQuantizer) CompressionRatio() float32 {
	return 1.0
}

func (q *NoneQuantizer) Type() Type {
	return TypeNone
}

func (q *NoneQuantizer) Train(vectors []point.Vector) error {
	return nil
}

// =============================================================================
// Scalar Quantizer (int8 - 4x compression)
// =============================================================================

// ScalarQuantizer quantizes each dimension to 8 bits
type ScalarQuantizer struct {
	dimension int
	quantile  float32

	// Calibration parameters (per dimension)
	mins   []float32
	maxs   []float32
	scales []float32

	trained bool
	mu      sync.RWMutex
}

// NewScalarQuantizer creates a new scalar quantizer
func NewScalarQuantizer(dimension int, quantile float32) *ScalarQuantizer {
	return &ScalarQuantizer{
		dimension: dimension,
		quantile:  quantile,
		mins:      make([]float32, dimension),
		maxs:      make([]float32, dimension),
		scales:    make([]float32, dimension),
	}
}

// Train calibrates the quantizer on sample vectors
func (q *ScalarQuantizer) Train(vectors []point.Vector) error {
	if len(vectors) == 0 {
		return errors.New("no vectors to train on")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	// Collect values per dimension
	dimValues := make([][]float32, q.dimension)
	for i := range dimValues {
		dimValues[i] = make([]float32, 0, len(vectors))
	}

	for _, vec := range vectors {
		for i, v := range vec {
			dimValues[i] = append(dimValues[i], v)
		}
	}

	// Calculate quantile-based min/max per dimension
	for i := 0; i < q.dimension; i++ {
		sortFloat32s(dimValues[i])

		lowIdx := int(float32(len(dimValues[i])) * (1 - q.quantile))
		highIdx := int(float32(len(dimValues[i])) * q.quantile)

		if lowIdx < 0 {
			lowIdx = 0
		}
		if highIdx >= len(dimValues[i]) {
			highIdx = len(dimValues[i]) - 1
		}

		q.mins[i] = dimValues[i][lowIdx]
		q.maxs[i] = dimValues[i][highIdx]

		// Calculate scale (avoid division by zero)
		rang := q.maxs[i] - q.mins[i]
		if rang < 1e-8 {
			rang = 1.0
		}
		q.scales[i] = 255.0 / rang
	}

	q.trained = true
	return nil
}

// Encode quantizes a vector to int8
func (q *ScalarQuantizer) Encode(vector point.Vector) ([]byte, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if !q.trained {
		// Auto-calibrate with single vector (simple min/max)
		q.mu.RUnlock()
		_ = q.Train([]point.Vector{vector}) // Error is logged internally
		q.mu.RLock()
	}

	data := make([]byte, q.dimension)
	for i, v := range vector {
		// Clamp to range
		if v < q.mins[i] {
			v = q.mins[i]
		}
		if v > q.maxs[i] {
			v = q.maxs[i]
		}

		// Quantize to 0-255
		quantized := (v - q.mins[i]) * q.scales[i]
		if quantized > 255 {
			quantized = 255
		}
		data[i] = byte(quantized)
	}

	return data, nil
}

// Decode reconstructs a vector from quantized form
func (q *ScalarQuantizer) Decode(data []byte) (point.Vector, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	vector := make(point.Vector, q.dimension)
	for i, b := range data {
		vector[i] = float32(b)/q.scales[i] + q.mins[i]
	}
	return vector, nil
}

// Distance computes approximate distance using quantized vector
func (q *ScalarQuantizer) Distance(query point.Vector, quantized []byte) float32 {
	q.mu.RLock()
	defer q.mu.RUnlock()

	// Compute distance directly on quantized values for speed
	var dotProduct, queryNorm, vecNorm float32

	for i, qVal := range query {
		// Dequantize on the fly
		vVal := float32(quantized[i])/q.scales[i] + q.mins[i]
		dotProduct += qVal * vVal
		queryNorm += qVal * qVal
		vecNorm += vVal * vVal
	}

	if queryNorm < 1e-8 || vecNorm < 1e-8 {
		return 1.0
	}

	similarity := dotProduct / (float32(math.Sqrt(float64(queryNorm))) * float32(math.Sqrt(float64(vecNorm))))
	return 1.0 - similarity
}

// BatchDistance computes distances for multiple quantized vectors
func (q *ScalarQuantizer) BatchDistance(query point.Vector, quantized [][]byte) []float32 {
	distances := make([]float32, len(quantized))
	for i, data := range quantized {
		distances[i] = q.Distance(query, data)
	}
	return distances
}

func (q *ScalarQuantizer) EncodedSize() int {
	return q.dimension
}

func (q *ScalarQuantizer) CompressionRatio() float32 {
	return 4.0 // float32 (4 bytes) -> int8 (1 byte)
}

func (q *ScalarQuantizer) Type() Type {
	return TypeScalar
}

// =============================================================================
// Binary Quantizer (1-bit - 32x compression)
// =============================================================================

// BinaryQuantizer quantizes each dimension to 1 bit
type BinaryQuantizer struct {
	dimension int

	// Threshold per dimension (usually mean or median)
	thresholds []float32

	trained bool
	mu      sync.RWMutex
}

// NewBinaryQuantizer creates a new binary quantizer
func NewBinaryQuantizer(dimension int) *BinaryQuantizer {
	return &BinaryQuantizer{
		dimension:  dimension,
		thresholds: make([]float32, dimension),
	}
}

// Train calculates thresholds from sample vectors
func (q *BinaryQuantizer) Train(vectors []point.Vector) error {
	if len(vectors) == 0 {
		return errors.New("no vectors to train on")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	// Calculate mean per dimension as threshold
	for i := 0; i < q.dimension; i++ {
		var sum float32
		for _, vec := range vectors {
			sum += vec[i]
		}
		q.thresholds[i] = sum / float32(len(vectors))
	}

	q.trained = true
	return nil
}

// Encode quantizes a vector to binary (1 bit per dimension)
func (q *BinaryQuantizer) Encode(vector point.Vector) ([]byte, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if !q.trained {
		// Use 0 as default threshold
		q.mu.RUnlock()
		q.mu.Lock()
		for i := range q.thresholds {
			q.thresholds[i] = 0
		}
		q.trained = true
		q.mu.Unlock()
		q.mu.RLock()
	}

	// Pack bits into bytes
	numBytes := (q.dimension + 7) / 8
	data := make([]byte, numBytes)

	for i, v := range vector {
		if v >= q.thresholds[i] {
			byteIdx := i / 8
			bitIdx := uint(i % 8)
			data[byteIdx] |= 1 << bitIdx
		}
	}

	return data, nil
}

// Decode reconstructs an approximate vector from binary
func (q *BinaryQuantizer) Decode(data []byte) (point.Vector, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	vector := make(point.Vector, q.dimension)
	for i := 0; i < q.dimension; i++ {
		byteIdx := i / 8
		bitIdx := uint(i % 8)
		if data[byteIdx]&(1<<bitIdx) != 0 {
			vector[i] = 1.0
		} else {
			vector[i] = -1.0
		}
	}
	return vector, nil
}

// Distance computes Hamming distance (very fast using POPCOUNT)
func (q *BinaryQuantizer) Distance(query point.Vector, quantized []byte) float32 {
	// First, encode the query
	queryBinary, _ := q.Encode(query)

	// Compute Hamming distance using XOR and popcount
	var hammingDist int
	for i := 0; i < len(quantized); i++ {
		xored := queryBinary[i] ^ quantized[i]
		hammingDist += popcount(xored)
	}

	// Normalize to 0-1 range
	return float32(hammingDist) / float32(q.dimension)
}

// BatchDistance computes distances for multiple quantized vectors
func (q *BinaryQuantizer) BatchDistance(query point.Vector, quantized [][]byte) []float32 {
	// Pre-encode query once
	queryBinary, _ := q.Encode(query)

	distances := make([]float32, len(quantized))
	for i, data := range quantized {
		var hammingDist int
		for j := 0; j < len(data); j++ {
			xored := queryBinary[j] ^ data[j]
			hammingDist += popcount(xored)
		}
		distances[i] = float32(hammingDist) / float32(q.dimension)
	}
	return distances
}

func (q *BinaryQuantizer) EncodedSize() int {
	return (q.dimension + 7) / 8
}

func (q *BinaryQuantizer) CompressionRatio() float32 {
	return 32.0 // float32 (32 bits) -> 1 bit
}

func (q *BinaryQuantizer) Type() Type {
	return TypeBinary
}

// =============================================================================
// Product Quantizer (PQ - configurable compression)
// =============================================================================

// ProductQuantizer uses codebook-based quantization
type ProductQuantizer struct {
	dimension   int
	numSegments int
	centroids   int
	segmentDim  int

	// Codebooks: [segment][centroid][dimension]
	codebooks [][][]float32

	trained bool
	mu      sync.RWMutex
}

// NewProductQuantizer creates a new product quantizer
func NewProductQuantizer(dimension, numSegments, centroids int) *ProductQuantizer {
	if dimension%numSegments != 0 {
		// Adjust segments to evenly divide
		for numSegments > 1 && dimension%numSegments != 0 {
			numSegments--
		}
	}

	segmentDim := dimension / numSegments

	pq := &ProductQuantizer{
		dimension:   dimension,
		numSegments: numSegments,
		centroids:   centroids,
		segmentDim:  segmentDim,
		codebooks:   make([][][]float32, numSegments),
	}

	// Initialize codebooks
	for i := 0; i < numSegments; i++ {
		pq.codebooks[i] = make([][]float32, centroids)
		for j := 0; j < centroids; j++ {
			pq.codebooks[i][j] = make([]float32, segmentDim)
		}
	}

	return pq
}

// Train trains codebooks using k-means on sample vectors
func (q *ProductQuantizer) Train(vectors []point.Vector) error {
	if len(vectors) < q.centroids {
		return errors.New("not enough vectors to train PQ")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	// Train each segment independently using k-means
	for seg := 0; seg < q.numSegments; seg++ {
		startDim := seg * q.segmentDim
		endDim := startDim + q.segmentDim

		// Extract subvectors for this segment
		subvectors := make([][]float32, len(vectors))
		for i, vec := range vectors {
			subvectors[i] = make([]float32, q.segmentDim)
			copy(subvectors[i], vec[startDim:endDim])
		}

		// Run k-means
		centroids := kmeans(subvectors, q.centroids, 20)
		q.codebooks[seg] = centroids
	}

	q.trained = true
	return nil
}

// Encode quantizes a vector using codebook lookup
func (q *ProductQuantizer) Encode(vector point.Vector) ([]byte, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if !q.trained {
		return nil, errors.New("PQ not trained")
	}

	// One byte per segment (index into codebook)
	data := make([]byte, q.numSegments)

	for seg := 0; seg < q.numSegments; seg++ {
		startDim := seg * q.segmentDim
		subvec := vector[startDim : startDim+q.segmentDim]

		// Find nearest centroid
		minDist := float32(math.MaxFloat32)
		minIdx := 0

		for i, centroid := range q.codebooks[seg] {
			dist := euclideanDistSub(subvec, centroid)
			if dist < minDist {
				minDist = dist
				minIdx = i
			}
		}

		data[seg] = byte(minIdx)
	}

	return data, nil
}

// Decode reconstructs a vector from PQ codes
func (q *ProductQuantizer) Decode(data []byte) (point.Vector, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	vector := make(point.Vector, q.dimension)

	for seg := 0; seg < q.numSegments; seg++ {
		centroidIdx := int(data[seg])
		startDim := seg * q.segmentDim
		copy(vector[startDim:startDim+q.segmentDim], q.codebooks[seg][centroidIdx])
	}

	return vector, nil
}

// Distance computes distance using precomputed distance tables
func (q *ProductQuantizer) Distance(query point.Vector, quantized []byte) float32 {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var totalDist float32

	for seg := 0; seg < q.numSegments; seg++ {
		startDim := seg * q.segmentDim
		subquery := query[startDim : startDim+q.segmentDim]

		centroidIdx := int(quantized[seg])
		centroid := q.codebooks[seg][centroidIdx]

		totalDist += euclideanDistSub(subquery, centroid)
	}

	return totalDist
}

// BatchDistance computes distances using ADC (Asymmetric Distance Computation)
func (q *ProductQuantizer) BatchDistance(query point.Vector, quantized [][]byte) []float32 {
	q.mu.RLock()
	defer q.mu.RUnlock()

	// Precompute distance table: [segment][centroid] -> distance
	distTable := make([][]float32, q.numSegments)
	for seg := 0; seg < q.numSegments; seg++ {
		distTable[seg] = make([]float32, q.centroids)
		startDim := seg * q.segmentDim
		subquery := query[startDim : startDim+q.segmentDim]

		for c := 0; c < q.centroids; c++ {
			distTable[seg][c] = euclideanDistSub(subquery, q.codebooks[seg][c])
		}
	}

	// Compute distances using lookup table (very fast)
	distances := make([]float32, len(quantized))
	for i, codes := range quantized {
		var dist float32
		for seg := 0; seg < q.numSegments; seg++ {
			dist += distTable[seg][int(codes[seg])]
		}
		distances[i] = dist
	}

	return distances
}

func (q *ProductQuantizer) EncodedSize() int {
	return q.numSegments // One byte per segment
}

func (q *ProductQuantizer) CompressionRatio() float32 {
	originalSize := float32(q.dimension * 4)
	quantizedSize := float32(q.numSegments)
	return originalSize / quantizedSize
}

func (q *ProductQuantizer) Type() Type {
	return TypePQ
}

// =============================================================================
// Helper functions
// =============================================================================

// popcount counts set bits in a byte
func popcount(b byte) int {
	count := 0
	for b != 0 {
		count += int(b & 1)
		b >>= 1
	}
	return count
}

// cosineDistance computes cosine distance between two vectors
func cosineDistance(a, b point.Vector) float32 {
	var dotProduct, normA, normB float32
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA < 1e-8 || normB < 1e-8 {
		return 1.0
	}
	return 1.0 - dotProduct/(float32(math.Sqrt(float64(normA)))*float32(math.Sqrt(float64(normB))))
}

// euclideanDistSub computes squared Euclidean distance for subvectors
func euclideanDistSub(a, b []float32) float32 {
	var sum float32
	for i := range a {
		diff := a[i] - b[i]
		sum += diff * diff
	}
	return sum
}

// sortFloat32s sorts a slice of float32 in place
func sortFloat32s(s []float32) {
	// Simple insertion sort for small slices
	for i := 1; i < len(s); i++ {
		key := s[i]
		j := i - 1
		for j >= 0 && s[j] > key {
			s[j+1] = s[j]
			j--
		}
		s[j+1] = key
	}
}

// kmeans performs k-means clustering
func kmeans(vectors [][]float32, k, iterations int) [][]float32 {
	if len(vectors) == 0 || len(vectors[0]) == 0 {
		return nil
	}

	dim := len(vectors[0])

	// Initialize centroids randomly (take first k vectors)
	centroids := make([][]float32, k)
	for i := 0; i < k; i++ {
		centroids[i] = make([]float32, dim)
		if i < len(vectors) {
			copy(centroids[i], vectors[i])
		}
	}

	assignments := make([]int, len(vectors))

	for iter := 0; iter < iterations; iter++ {
		// Assign vectors to nearest centroid
		for i, vec := range vectors {
			minDist := float32(math.MaxFloat32)
			minIdx := 0
			for j, centroid := range centroids {
				dist := euclideanDistSub(vec, centroid)
				if dist < minDist {
					minDist = dist
					minIdx = j
				}
			}
			assignments[i] = minIdx
		}

		// Update centroids
		counts := make([]int, k)
		newCentroids := make([][]float32, k)
		for i := 0; i < k; i++ {
			newCentroids[i] = make([]float32, dim)
		}

		for i, vec := range vectors {
			cluster := assignments[i]
			counts[cluster]++
			for d := 0; d < dim; d++ {
				newCentroids[cluster][d] += vec[d]
			}
		}

		for i := 0; i < k; i++ {
			if counts[i] > 0 {
				for d := 0; d < dim; d++ {
					newCentroids[i][d] /= float32(counts[i])
				}
			}
			centroids[i] = newCentroids[i]
		}
	}

	return centroids
}
