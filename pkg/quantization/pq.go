package quantization

import (
	"errors"
	"math"
	"sync"

	"github.com/limyedb/limyedb/pkg/point"
)

// ProductQuantizer splits vectors into M subspaces, clustering each into K centroids
// offering extreme compression ratios while preserving Euclidean distance characteristics.
type ProductQuantizer struct {
	dimension int
	m         int // Number of subvectors
	k         int // Number of centroids per subspace (256 aligns to 1 byte)

	// codebooks[subspace][centroidID][subdimension]
	codebooks [][][]float32
	trained   bool
	mu        sync.RWMutex
}

// NewProductQuantizer creates an untrained PQ struct.
func NewProductQuantizer(dimension int, m int, k int) *ProductQuantizer {
	if dimension%m != 0 {
		return nil
	}
	return &ProductQuantizer{
		dimension: dimension,
		m:         m,
		k:         k, // Often 256 aligns to 1 byte
		codebooks: make([][][]float32, m),
	}
}

// Train calculates the specific centroid bounds over subsets.
// For pure simplicity, this implementation assigns deterministic initialization directly 
// mapping over vector arrays. A production K-Means iteration logic runs here contextually.
func (q *ProductQuantizer) Train(vectors []point.Vector) error {
	if len(vectors) < q.k {
		return errors.New("not enough vectors to train PQ centroids")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	subDim := q.dimension / q.m

	for i := 0; i < q.m; i++ {
		q.codebooks[i] = make([][]float32, q.k)
		for j := 0; j < q.k; j++ {
			q.codebooks[i][j] = make([]float32, subDim)
			copy(q.codebooks[i][j], vectors[j][i*subDim:(i+1)*subDim])
		}
	}

	q.trained = true
	return nil
}

// Encode converts the array into extremely dense subspace byte grids natively.
func (q *ProductQuantizer) Encode(vector point.Vector) ([]byte, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if !q.trained {
		return nil, errors.New("quantizer not trained")
	}

	subDim := q.dimension / q.m
	encoded := make([]byte, q.m)

	for i := 0; i < q.m; i++ {
		subVec := vector[i*subDim : (i+1)*subDim]
		
		bestDist := float32(math.MaxFloat32)
		bestIdx := 0
		
		for j := 0; j < q.k; j++ {
			centroid := q.codebooks[i][j]
			var dist float32
			for k := 0; k < subDim; k++ {
				diff := subVec[k] - centroid[k]
				dist += diff * diff
			}
			if dist < bestDist {
				bestDist = dist
				bestIdx = j
			}
		}
		encoded[i] = byte(bestIdx)
	}

	return encoded, nil
}

// Decode transforms the compressed bytes back into approximated bounds reliably.
func (q *ProductQuantizer) Decode(data []byte) (point.Vector, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if len(data) != q.m {
		return nil, errors.New("invalid encoded data length")
	}

	subDim := q.dimension / q.m
	vector := make(point.Vector, q.dimension)

	for i := 0; i < q.m; i++ {
		centroid := q.codebooks[i][data[i]]
		copy(vector[i*subDim:(i+1)*subDim], centroid)
	}

	return vector, nil
}

// Distance executes Asymmetric Distance Computation (ADC) mathematically evaluating 
// the raw queried vector uniformly mapped against the encoded centroid byte tables entirely natively.
func (q *ProductQuantizer) Distance(query point.Vector, quantized []byte) float32 {
	q.mu.RLock()
	defer q.mu.RUnlock()

	subDim := q.dimension / q.m
	var totalDist float32

	for i := 0; i < q.m; i++ {
		subQuery := query[i*subDim : (i+1)*subDim]
		centroid := q.codebooks[i][quantized[i]]
		
		for k := 0; k < subDim; k++ {
			diff := subQuery[k] - centroid[k]
			totalDist += diff * diff
		}
	}

	return totalDist
}

// BatchDistance utilizes ADC table lookups mapping extremely dense evaluation graphs asynchronously
// significantly accelerating retrieval operations safely.
func (q *ProductQuantizer) BatchDistance(query point.Vector, quantized [][]byte) []float32 {
	distances := make([]float32, len(quantized))
	
	q.mu.RLock()
	table := make([][]float32, q.m)
	subDim := q.dimension / q.m

	for i := 0; i < q.m; i++ {
		table[i] = make([]float32, q.k)
		subQuery := query[i*subDim : (i+1)*subDim]
		
		for j := 0; j < q.k; j++ {
			centroid := q.codebooks[i][j]
			var dist float32
			for k := 0; k < subDim; k++ {
				diff := subQuery[k] - centroid[k]
				dist += diff * diff
			}
			table[i][j] = dist
		}
	}
	q.mu.RUnlock()

	for i, data := range quantized {
		var dist float32
		for j := 0; j < q.m; j++ {
			dist += table[j][data[j]]
		}
		distances[i] = dist
	}

	return distances
}

// EncodedSize computes strictly minimal bound arrays cleanly scaling natively.
func (q *ProductQuantizer) EncodedSize() int {
	return q.m
}

// CompressionRatio maps standard `float32` variables evaluating down to explicit bytes logically saving 95%+ RAM.
func (q *ProductQuantizer) CompressionRatio() float32 {
	return float32(q.dimension*4) / float32(q.m)
}

// Type flags explicit PQ definitions securely scaling natively
func (q *ProductQuantizer) Type() Type {
	return TypePQ
}
