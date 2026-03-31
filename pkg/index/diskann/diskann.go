// Package diskann implements the DiskANN algorithm for approximate nearest neighbor search.
// DiskANN (also known as Vamana) is designed for billion-scale vector search with
// disk-based storage, achieving high recall with low memory footprint.
//
// Key features:
// - Single-layer graph structure (unlike HNSW's multi-layer)
// - RobustPrune algorithm for alpha-diverse neighbor selection
// - Memory-mapped storage support for SSD-based search
// - Two-pass index construction for quality optimization
//
// Reference: Subramanya et al., "DiskANN: Fast Accurate Billion-point
// Nearest Neighbor Search on a Single Node" (NeurIPS 2019)
package diskann

import (
	"sync"

	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/point"
)

// Index is the main DiskANN index structure
type Index struct {
	config *Config
	graph  *VamanaGraph

	// Concurrency control
	mu sync.RWMutex
}

// New creates a new DiskANN index with the given configuration
func New(cfg *Config) (*Index, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Determine metric string
	var metric string
	switch cfg.Metric {
	case config.MetricCosine:
		metric = "cosine"
	case config.MetricEuclidean:
		metric = "euclidean"
	case config.MetricDotProduct:
		metric = "dot"
	default:
		metric = "euclidean"
	}

	graph := NewVamanaGraph(cfg.Dimension, cfg.R, cfg.Alpha, metric)
	graph.SetSearchL(cfg.L)

	return &Index{
		config: cfg,
		graph:  graph,
	}, nil
}

// Insert adds a new point to the index
func (idx *Index) Insert(p *point.Point) error {
	return idx.graph.Insert(p)
}

// Search performs k-NN search
func (idx *Index) Search(query point.Vector, k int) ([]Candidate, error) {
	return idx.graph.Search(query, k)
}

// SearchWithEf performs k-NN search with custom search list size
func (idx *Index) SearchWithEf(query point.Vector, k int, ef int) ([]Candidate, error) {
	return idx.graph.SearchWithEf(query, k, ef)
}

// SearchWithFilter performs filtered search
func (idx *Index) SearchWithFilter(query point.Vector, params *SearchParams) ([]Candidate, error) {
	if len(query) != idx.config.Dimension {
		return nil, ErrDimensionMismatch
	}
	return idx.graph.SearchWithFilter(query, params), nil
}

// BatchSearch performs batch k-NN search
func (idx *Index) BatchSearch(queries []point.Vector, k int) ([][]Candidate, error) {
	L := idx.config.L
	if L < k {
		L = k
	}
	return idx.graph.BatchSearch(queries, k, L), nil
}

// Delete marks a point as deleted
func (idx *Index) Delete(id string) error {
	return idx.graph.Delete(id)
}

// Get retrieves a point by ID
func (idx *Index) Get(id string) (*point.Point, error) {
	return idx.graph.Get(id)
}

// Size returns the number of active points
func (idx *Index) Size() int64 {
	return idx.graph.Size()
}

// TotalSize returns the total number of points including deleted
func (idx *Index) TotalSize() int64 {
	return idx.graph.TotalSize()
}

// GetNodeID returns the internal node ID for a point ID
func (idx *Index) GetNodeID(pointID string) (uint32, bool) {
	return idx.graph.GetNodeID(pointID)
}

// GetPointID returns the point ID for an internal node ID
func (idx *Index) GetPointID(nodeID uint32) string {
	return idx.graph.GetPointID(nodeID)
}

// Iterate iterates over all non-deleted point IDs
func (idx *Index) Iterate(fn func(id string) error) error {
	return idx.graph.Iterate(fn)
}

// GetAllPoints returns all non-deleted points
func (idx *Index) GetAllPoints() []*point.Point {
	return idx.graph.GetAllPoints()
}

// Recommend finds similar points to a given point
func (idx *Index) Recommend(id string, k int) ([]Candidate, error) {
	return idx.graph.Recommend(id, k)
}

// RangeSearch finds all points within a given radius
func (idx *Index) RangeSearch(query point.Vector, radius float32) ([]Candidate, error) {
	if len(query) != idx.config.Dimension {
		return nil, ErrDimensionMismatch
	}
	return idx.graph.RangeSearch(query, radius, 0), nil
}

// BuildIndex builds the index from a set of vectors using the Vamana algorithm
func (idx *Index) BuildIndex(vectors []point.Vector, ids []string, opts *BuildOptions) error {
	return idx.graph.BuildIndex(vectors, ids, opts)
}

// SetSearchL sets the search list size parameter
func (idx *Index) SetSearchL(L int) {
	idx.graph.SetSearchL(L)
}

// GetStats returns statistics about the index
func (idx *Index) GetStats() Stats {
	return idx.graph.GetStats()
}

// Config returns the index configuration
func (idx *Index) Config() *Config {
	return idx.config
}

// IteratePoints iterates over all non-deleted points with full point data
func (idx *Index) IteratePoints(fn func(*point.Point) bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	points := idx.graph.GetAllPoints()
	for _, p := range points {
		if !fn(p) {
			break
		}
	}
}

// SetEfSearch sets the search quality parameter (alias for SetSearchL for HNSW compatibility)
func (idx *Index) SetEfSearch(ef int) {
	idx.SetSearchL(ef)
}
