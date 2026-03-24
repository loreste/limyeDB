// Package multimodel provides support for multiple vector types in a single collection.
package multimodel

import (
	"fmt"
	"sync"
)

// VectorType represents the type of vector.
type VectorType string

const (
	VectorTypeDense  VectorType = "dense"
	VectorTypeSparse VectorType = "sparse"
	VectorTypeBinary VectorType = "binary"
)

// VectorConfig defines configuration for a vector type.
type VectorConfig struct {
	Name      string     `json:"name"`
	Type      VectorType `json:"type"`
	Dimension int        `json:"dimension"` // For dense vectors
	Metric    string     `json:"metric"`    // cosine, euclidean, dot_product, jaccard
}

// MultiVectorPoint represents a point with multiple vector types.
type MultiVectorPoint struct {
	ID       string                 `json:"id"`
	Vectors  map[string]interface{} `json:"vectors"` // name -> vector data
	Payload  map[string]interface{} `json:"payload,omitempty"`
}

// DenseVector represents a dense floating-point vector.
type DenseVector struct {
	Values []float32 `json:"values"`
}

// SparseVector represents a sparse vector with indices and values.
type SparseVector struct {
	Indices []uint32  `json:"indices"`
	Values  []float32 `json:"values"`
}

// BinaryVector represents a binary vector for Hamming distance.
type BinaryVector struct {
	Bits []uint64 `json:"bits"`
}

// MultiModelIndex manages multiple vector types for a collection.
type MultiModelIndex struct {
	mu        sync.RWMutex
	configs   map[string]VectorConfig
	points    map[string]*MultiVectorPoint
	denseIdx  map[string]*DenseIndex  // vector_name -> index
	sparseIdx map[string]*SparseIndex // vector_name -> index
}

// NewMultiModelIndex creates a new multi-model index.
func NewMultiModelIndex(configs []VectorConfig) *MultiModelIndex {
	idx := &MultiModelIndex{
		configs:   make(map[string]VectorConfig),
		points:    make(map[string]*MultiVectorPoint),
		denseIdx:  make(map[string]*DenseIndex),
		sparseIdx: make(map[string]*SparseIndex),
	}

	for _, cfg := range configs {
		idx.configs[cfg.Name] = cfg
		switch cfg.Type {
		case VectorTypeDense:
			idx.denseIdx[cfg.Name] = NewDenseIndex(cfg)
		case VectorTypeSparse:
			idx.sparseIdx[cfg.Name] = NewSparseIndex(cfg)
		}
	}

	return idx
}

// Insert inserts a multi-vector point.
func (idx *MultiModelIndex) Insert(point *MultiVectorPoint) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Validate vectors match configs
	for name, vec := range point.Vectors {
		cfg, exists := idx.configs[name]
		if !exists {
			return fmt.Errorf("unknown vector type: %s", name)
		}

		// Validate and index based on type
		switch cfg.Type {
		case VectorTypeDense:
			denseVec, ok := vec.([]float32)
			if !ok {
				// Try to convert from interface
				if arr, ok := vec.([]interface{}); ok {
					denseVec = make([]float32, len(arr))
					for i, v := range arr {
						if f, ok := v.(float64); ok {
							denseVec[i] = float32(f)
						}
					}
				} else {
					return fmt.Errorf("invalid dense vector for %s", name)
				}
			}
			if len(denseVec) != cfg.Dimension {
				return fmt.Errorf("dimension mismatch for %s: got %d, expected %d",
					name, len(denseVec), cfg.Dimension)
			}
			idx.denseIdx[name].Insert(point.ID, denseVec)

		case VectorTypeSparse:
			sparseVec, ok := vec.(*SparseVector)
			if !ok {
				return fmt.Errorf("invalid sparse vector for %s", name)
			}
			idx.sparseIdx[name].Insert(point.ID, sparseVec)
		}
	}

	idx.points[point.ID] = point
	return nil
}

// Search searches using a specific vector type.
func (idx *MultiModelIndex) Search(vectorName string, query interface{}, k int) ([]SearchResult, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	cfg, exists := idx.configs[vectorName]
	if !exists {
		return nil, fmt.Errorf("unknown vector type: %s", vectorName)
	}

	switch cfg.Type {
	case VectorTypeDense:
		queryVec, ok := query.([]float32)
		if !ok {
			return nil, fmt.Errorf("invalid dense query vector")
		}
		return idx.denseIdx[vectorName].Search(queryVec, k), nil

	case VectorTypeSparse:
		queryVec, ok := query.(*SparseVector)
		if !ok {
			return nil, fmt.Errorf("invalid sparse query vector")
		}
		return idx.sparseIdx[vectorName].Search(queryVec, k), nil

	default:
		return nil, fmt.Errorf("unsupported vector type: %s", cfg.Type)
	}
}

// HybridSearch searches using multiple vector types and fuses results.
func (idx *MultiModelIndex) HybridSearch(queries map[string]interface{}, k int, weights map[string]float32) ([]SearchResult, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// Collect results from each vector type
	allResults := make(map[string][]SearchResult)

	for name, query := range queries {
		results, err := idx.Search(name, query, k*2) // Fetch more for fusion
		if err != nil {
			return nil, err
		}
		allResults[name] = results
	}

	// Fuse results using weighted combination
	return idx.fuseResults(allResults, weights, k), nil
}

// SearchResult represents a search result.
type SearchResult struct {
	ID      string
	Score   float32
	Payload map[string]interface{}
}

func (idx *MultiModelIndex) fuseResults(results map[string][]SearchResult, weights map[string]float32, k int) []SearchResult {
	// Score aggregation
	scores := make(map[string]float32)
	payloads := make(map[string]map[string]interface{})

	for name, res := range results {
		weight := weights[name]
		if weight == 0 {
			weight = 1.0
		}

		for rank, r := range res {
			// RRF scoring: 1 / (k + rank)
			rrfScore := 1.0 / float32(60+rank) * weight
			scores[r.ID] += rrfScore
			if payloads[r.ID] == nil {
				payloads[r.ID] = r.Payload
			}
		}
	}

	// Sort by score
	type scoredResult struct {
		id    string
		score float32
	}
	var sorted []scoredResult
	for id, score := range scores {
		sorted = append(sorted, scoredResult{id, score})
	}

	// Simple sort (production would use heap)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].score > sorted[i].score {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Take top k
	if len(sorted) > k {
		sorted = sorted[:k]
	}

	results_final := make([]SearchResult, len(sorted))
	for i, s := range sorted {
		results_final[i] = SearchResult{
			ID:      s.id,
			Score:   s.score,
			Payload: payloads[s.id],
		}
	}

	return results_final
}

// DenseIndex is a simple dense vector index.
type DenseIndex struct {
	config  VectorConfig
	vectors map[string][]float32
}

// NewDenseIndex creates a new dense index.
func NewDenseIndex(config VectorConfig) *DenseIndex {
	return &DenseIndex{
		config:  config,
		vectors: make(map[string][]float32),
	}
}

// Insert adds a dense vector.
func (idx *DenseIndex) Insert(id string, vector []float32) {
	idx.vectors[id] = vector
}

// Search finds nearest neighbors.
func (idx *DenseIndex) Search(query []float32, k int) []SearchResult {
	type scored struct {
		id    string
		score float32
	}
	var results []scored

	for id, vec := range idx.vectors {
		score := cosineSimilarity(query, vec)
		results = append(results, scored{id, score})
	}

	// Sort descending
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].score > results[i].score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	if len(results) > k {
		results = results[:k]
	}

	out := make([]SearchResult, len(results))
	for i, r := range results {
		out[i] = SearchResult{ID: r.id, Score: r.score}
	}
	return out
}

// SparseIndex is a simple sparse vector index.
type SparseIndex struct {
	config  VectorConfig
	vectors map[string]*SparseVector
}

// NewSparseIndex creates a new sparse index.
func NewSparseIndex(config VectorConfig) *SparseIndex {
	return &SparseIndex{
		config:  config,
		vectors: make(map[string]*SparseVector),
	}
}

// Insert adds a sparse vector.
func (idx *SparseIndex) Insert(id string, vector *SparseVector) {
	idx.vectors[id] = vector
}

// Search finds nearest neighbors using sparse dot product.
func (idx *SparseIndex) Search(query *SparseVector, k int) []SearchResult {
	type scored struct {
		id    string
		score float32
	}
	var results []scored

	for id, vec := range idx.vectors {
		score := sparseDotProduct(query, vec)
		results = append(results, scored{id, score})
	}

	// Sort descending
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].score > results[i].score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	if len(results) > k {
		results = results[:k]
	}

	out := make([]SearchResult, len(results))
	for i, r := range results {
		out[i] = SearchResult{ID: r.id, Score: r.score}
	}
	return out
}

func cosineSimilarity(a, b []float32) float32 {
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (sqrt(normA) * sqrt(normB))
}

func sparseDotProduct(a, b *SparseVector) float32 {
	var sum float32
	i, j := 0, 0
	for i < len(a.Indices) && j < len(b.Indices) {
		if a.Indices[i] == b.Indices[j] {
			sum += a.Values[i] * b.Values[j]
			i++
			j++
		} else if a.Indices[i] < b.Indices[j] {
			i++
		} else {
			j++
		}
	}
	return sum
}

func sqrt(x float32) float32 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}
