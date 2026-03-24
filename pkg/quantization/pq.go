package quantization

import (
	"errors"
	"math"
	"sync"

	"github.com/limyedb/limyedb/pkg/point"
)

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

// euclideanDistSub computes squared Euclidean distance for subvectors
func euclideanDistSub(a, b []float32) float32 {
	var sum float32
	for i := range a {
		diff := a[i] - b[i]
		sum += diff * diff
	}
	return sum
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
