// Package quantization provides vector compression using product quantization.
package quantization

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"sync"
)

// ProductQuantizer implements product quantization for vector compression.
// It divides vectors into subvectors and quantizes each independently.
type ProductQuantizer struct {
	mu           sync.RWMutex
	dimension    int
	numSubvectors int
	numCentroids  int
	subvectorDim int
	codebooks    [][][]float32 // [subvector][centroid][subvector_dim]
	trained      bool
}

// PQConfig holds product quantization configuration.
type PQConfig struct {
	Dimension    int // Vector dimension
	NumSubvectors int // Number of subvectors (must divide dimension evenly)
	NumCentroids  int // Number of centroids per subvector (typically 256 for 8-bit codes)
}

// NewProductQuantizer creates a new product quantizer.
func NewProductQuantizer(config PQConfig) (*ProductQuantizer, error) {
	if config.Dimension%config.NumSubvectors != 0 {
		return nil, fmt.Errorf("dimension %d must be divisible by num_subvectors %d",
			config.Dimension, config.NumSubvectors)
	}

	return &ProductQuantizer{
		dimension:    config.Dimension,
		numSubvectors: config.NumSubvectors,
		numCentroids:  config.NumCentroids,
		subvectorDim: config.Dimension / config.NumSubvectors,
		codebooks:    make([][][]float32, config.NumSubvectors),
		trained:      false,
	}, nil
}

// Train trains the product quantizer on a set of vectors.
func (pq *ProductQuantizer) Train(vectors [][]float32, maxIterations int) error {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if len(vectors) < pq.numCentroids {
		return fmt.Errorf("need at least %d vectors for training, got %d",
			pq.numCentroids, len(vectors))
	}

	// Train each subvector independently
	for s := 0; s < pq.numSubvectors; s++ {
		// Extract subvectors
		subvectors := make([][]float32, len(vectors))
		start := s * pq.subvectorDim
		end := start + pq.subvectorDim

		for i, v := range vectors {
			subvectors[i] = v[start:end]
		}

		// Run k-means clustering
		centroids := pq.kmeans(subvectors, pq.numCentroids, maxIterations)
		pq.codebooks[s] = centroids
	}

	pq.trained = true
	return nil
}

// kmeans performs k-means clustering.
func (pq *ProductQuantizer) kmeans(vectors [][]float32, k, maxIterations int) [][]float32 {
	dim := len(vectors[0])

	// Initialize centroids randomly
	centroids := make([][]float32, k)
	perm := rand.Perm(len(vectors))
	for i := 0; i < k; i++ {
		centroids[i] = make([]float32, dim)
		copy(centroids[i], vectors[perm[i]])
	}

	assignments := make([]int, len(vectors))

	for iter := 0; iter < maxIterations; iter++ {
		// Assign vectors to nearest centroid
		changed := false
		for i, v := range vectors {
			nearest := pq.findNearest(v, centroids)
			if nearest != assignments[i] {
				assignments[i] = nearest
				changed = true
			}
		}

		if !changed {
			break
		}

		// Update centroids
		counts := make([]int, k)
		newCentroids := make([][]float32, k)
		for i := range newCentroids {
			newCentroids[i] = make([]float32, dim)
		}

		for i, v := range vectors {
			c := assignments[i]
			counts[c]++
			for j := range v {
				newCentroids[c][j] += v[j]
			}
		}

		for c := 0; c < k; c++ {
			if counts[c] > 0 {
				for j := range centroids[c] {
					centroids[c][j] = newCentroids[c][j] / float32(counts[c])
				}
			}
		}
	}

	return centroids
}

// Encode encodes a vector into PQ codes.
func (pq *ProductQuantizer) Encode(vector []float32) ([]uint8, error) {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	if !pq.trained {
		return nil, fmt.Errorf("quantizer not trained")
	}

	if len(vector) != pq.dimension {
		return nil, fmt.Errorf("vector dimension mismatch: got %d, expected %d",
			len(vector), pq.dimension)
	}

	codes := make([]uint8, pq.numSubvectors)

	for s := 0; s < pq.numSubvectors; s++ {
		start := s * pq.subvectorDim
		end := start + pq.subvectorDim
		subvector := vector[start:end]

		nearest := pq.findNearest(subvector, pq.codebooks[s])
		codes[s] = uint8(nearest)
	}

	return codes, nil
}

// Decode decodes PQ codes back to an approximate vector.
func (pq *ProductQuantizer) Decode(codes []uint8) ([]float32, error) {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	if !pq.trained {
		return nil, fmt.Errorf("quantizer not trained")
	}

	if len(codes) != pq.numSubvectors {
		return nil, fmt.Errorf("codes length mismatch: got %d, expected %d",
			len(codes), pq.numSubvectors)
	}

	vector := make([]float32, pq.dimension)

	for s := 0; s < pq.numSubvectors; s++ {
		start := s * pq.subvectorDim
		centroid := pq.codebooks[s][codes[s]]
		copy(vector[start:start+pq.subvectorDim], centroid)
	}

	return vector, nil
}

// DistanceTable precomputes distances from a query to all centroids.
type DistanceTable struct {
	distances [][]float32 // [subvector][centroid]
}

// ComputeDistanceTable precomputes distances for asymmetric distance computation.
func (pq *ProductQuantizer) ComputeDistanceTable(query []float32) (*DistanceTable, error) {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	if !pq.trained {
		return nil, fmt.Errorf("quantizer not trained")
	}

	table := &DistanceTable{
		distances: make([][]float32, pq.numSubvectors),
	}

	for s := 0; s < pq.numSubvectors; s++ {
		start := s * pq.subvectorDim
		end := start + pq.subvectorDim
		subquery := query[start:end]

		table.distances[s] = make([]float32, pq.numCentroids)
		for c, centroid := range pq.codebooks[s] {
			table.distances[s][c] = pq.l2Distance(subquery, centroid)
		}
	}

	return table, nil
}

// AsymmetricDistance computes distance using precomputed table (fast).
func (pq *ProductQuantizer) AsymmetricDistance(table *DistanceTable, codes []uint8) float32 {
	var dist float32
	for s, code := range codes {
		dist += table.distances[s][code]
	}
	return float32(math.Sqrt(float64(dist)))
}

// SymmetricDistance computes distance between two encoded vectors.
func (pq *ProductQuantizer) SymmetricDistance(codes1, codes2 []uint8) float32 {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	var dist float32
	for s := 0; s < pq.numSubvectors; s++ {
		c1 := pq.codebooks[s][codes1[s]]
		c2 := pq.codebooks[s][codes2[s]]
		dist += pq.l2Distance(c1, c2)
	}
	return float32(math.Sqrt(float64(dist)))
}

func (pq *ProductQuantizer) findNearest(vector []float32, centroids [][]float32) int {
	minDist := float32(math.MaxFloat32)
	minIdx := 0

	for i, c := range centroids {
		dist := pq.l2Distance(vector, c)
		if dist < minDist {
			minDist = dist
			minIdx = i
		}
	}

	return minIdx
}

func (pq *ProductQuantizer) l2Distance(a, b []float32) float32 {
	var sum float32
	for i := range a {
		d := a[i] - b[i]
		sum += d * d
	}
	return sum
}

// CompressionRatio returns the compression ratio achieved.
func (pq *ProductQuantizer) CompressionRatio() float64 {
	originalSize := pq.dimension * 4 // float32 = 4 bytes
	compressedSize := pq.numSubvectors // 1 byte per subvector (uint8)
	return float64(originalSize) / float64(compressedSize)
}

// SaveCodebook serializes the codebook to bytes.
func (pq *ProductQuantizer) SaveCodebook() ([]byte, error) {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	if !pq.trained {
		return nil, fmt.Errorf("quantizer not trained")
	}

	// Header: dimension, numSubvectors, numCentroids, subvectorDim
	size := 16 // header
	size += pq.numSubvectors * pq.numCentroids * pq.subvectorDim * 4

	data := make([]byte, size)
	offset := 0

	binary.LittleEndian.PutUint32(data[offset:], uint32(pq.dimension))
	offset += 4
	binary.LittleEndian.PutUint32(data[offset:], uint32(pq.numSubvectors))
	offset += 4
	binary.LittleEndian.PutUint32(data[offset:], uint32(pq.numCentroids))
	offset += 4
	binary.LittleEndian.PutUint32(data[offset:], uint32(pq.subvectorDim))
	offset += 4

	for s := 0; s < pq.numSubvectors; s++ {
		for c := 0; c < pq.numCentroids; c++ {
			for d := 0; d < pq.subvectorDim; d++ {
				bits := math.Float32bits(pq.codebooks[s][c][d])
				binary.LittleEndian.PutUint32(data[offset:], bits)
				offset += 4
			}
		}
	}

	return data, nil
}

// LoadCodebook deserializes the codebook from bytes.
func (pq *ProductQuantizer) LoadCodebook(data []byte) error {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if len(data) < 16 {
		return fmt.Errorf("invalid codebook data")
	}

	offset := 0
	pq.dimension = int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	pq.numSubvectors = int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	pq.numCentroids = int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	pq.subvectorDim = int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4

	pq.codebooks = make([][][]float32, pq.numSubvectors)

	for s := 0; s < pq.numSubvectors; s++ {
		pq.codebooks[s] = make([][]float32, pq.numCentroids)
		for c := 0; c < pq.numCentroids; c++ {
			pq.codebooks[s][c] = make([]float32, pq.subvectorDim)
			for d := 0; d < pq.subvectorDim; d++ {
				bits := binary.LittleEndian.Uint32(data[offset:])
				pq.codebooks[s][c][d] = math.Float32frombits(bits)
				offset += 4
			}
		}
	}

	pq.trained = true
	return nil
}
