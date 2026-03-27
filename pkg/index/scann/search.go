package scann

import (
	"container/heap"
	"sort"
	"sync"

	"github.com/limyedb/limyedb/pkg/point"
)

// SearchParams holds search parameters
type SearchParams struct {
	K         int // Number of results to return
	NumLeaves int // Number of partitions to search (overrides default)
	NumRerank int // Number of candidates for reranking
	Filter    func(id string, payload map[string]interface{}) bool
}

// DefaultSearchParams returns default search parameters
func DefaultSearchParams(k int) *SearchParams {
	return &SearchParams{
		K:         k,
		NumLeaves: 0, // Use index default
		NumRerank: 0, // Use index default
	}
}

// Candidate represents a search result candidate
type Candidate struct {
	ID       uint32
	Distance float32
}

// CandidateHeap is a min-heap of candidates (smallest distance first)
type CandidateHeap []Candidate

func (h CandidateHeap) Len() int           { return len(h) }
func (h CandidateHeap) Less(i, j int) bool { return h[i].Distance < h[j].Distance }
func (h CandidateHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *CandidateHeap) Push(x interface{}) {
	if c, ok := x.(Candidate); ok {
		*h = append(*h, c)
	}
}

func (h *CandidateHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// MaxCandidateHeap is a max-heap for tracking worst results
type MaxCandidateHeap []Candidate

func (h MaxCandidateHeap) Len() int           { return len(h) }
func (h MaxCandidateHeap) Less(i, j int) bool { return h[i].Distance > h[j].Distance }
func (h MaxCandidateHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *MaxCandidateHeap) Push(x interface{}) {
	if c, ok := x.(Candidate); ok {
		*h = append(*h, c)
	}
}

func (h *MaxCandidateHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// Search performs k-NN search using two-phase approach
func (s *ScaNN) Search(query point.Vector, k int) ([]Candidate, error) {
	return s.SearchWithParams(query, &SearchParams{K: k})
}

// SearchWithParams performs k-NN search with custom parameters
func (s *ScaNN) SearchWithParams(query point.Vector, params *SearchParams) ([]Candidate, error) {
	if len(query) != s.config.Dimension {
		return nil, ErrDimensionMismatch
	}

	s.mu.RLock()
	trained := s.trained
	s.mu.RUnlock()

	if !trained {
		// Fall back to brute force if not trained
		return s.bruteForceSearch(query, params.K, params.Filter)
	}

	// Phase 1: Approximate search using quantization
	numLeaves := params.NumLeaves
	if numLeaves <= 0 {
		numLeaves = s.partitioner.NumLeaves() / 10 // Search 10% of leaves by default
		if numLeaves < 1 {
			numLeaves = 1
		}
	}

	numRerank := params.NumRerank
	if numRerank <= 0 {
		numRerank = s.config.NumRerank
	}

	// Get candidates from nearest partitions
	candidates := s.getApproximateCandidates(query, numLeaves, numRerank, params.Filter)

	// Phase 2: Exact reranking
	return s.rerankCandidates(query, candidates, params.K, params.Filter)
}

// getApproximateCandidates performs approximate search using quantization
func (s *ScaNN) getApproximateCandidates(query point.Vector, numLeaves, numRerank int, filter func(string, map[string]interface{}) bool) []Candidate {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Find nearest partitions
	nearestLeaves := s.partitioner.FindNearestLeaves(query, numLeaves)
	if len(nearestLeaves) == 0 {
		return nil
	}

	// Precompute distances to all codes for fast lookup
	distTable := s.aqEncoder.PrecomputeDistances(query)

	// Collect candidates from partitions
	results := &MaxCandidateHeap{}
	heap.Init(results)

	for _, leafIdx := range nearestLeaves {
		leaf := s.partitioner.GetLeaf(leafIdx)
		if leaf == nil {
			continue
		}

		leaf.mu.RLock()
		for _, pointID := range leaf.pointIDs {
			if int(pointID) >= len(s.points) {
				continue
			}

			p := s.points[pointID]
			if p == nil {
				continue
			}

			// Apply filter if provided
			if filter != nil && !filter(p.ID, p.Payload) {
				continue
			}

			// Use precomputed distance table for fast lookup
			var dist float32
			if distTable != nil && int(pointID) < len(s.quantized) && s.quantized[pointID] != nil {
				dist = distTable.LookupDistance(s.quantized[pointID])
			} else {
				// Fall back to exact distance if no quantization
				dist = s.distCalc.Distance(query, p.Vector)
			}

			// Add to results
			if results.Len() < numRerank {
				heap.Push(results, Candidate{ID: pointID, Distance: dist})
			} else if dist < (*results)[0].Distance {
				heap.Pop(results)
				heap.Push(results, Candidate{ID: pointID, Distance: dist})
			}
		}
		leaf.mu.RUnlock()
	}

	// Convert to slice
	candidates := make([]Candidate, results.Len())
	for i := len(candidates) - 1; i >= 0; i-- {
		if c, ok := heap.Pop(results).(Candidate); ok {
			candidates[i] = c
		}
	}

	return candidates
}

// rerankCandidates performs exact distance computation for reranking
func (s *ScaNN) rerankCandidates(query point.Vector, candidates []Candidate, k int, filter func(string, map[string]interface{}) bool) ([]Candidate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Compute exact distances
	for i := range candidates {
		pointID := candidates[i].ID
		if int(pointID) < len(s.points) {
			p := s.points[pointID]
			if p != nil {
				candidates[i].Distance = s.distCalc.Distance(query, p.Vector)
			}
		}
	}

	// Sort by exact distance
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Distance < candidates[j].Distance
	})

	// Return top k
	if len(candidates) > k {
		candidates = candidates[:k]
	}

	return candidates, nil
}

// SearchWithFilter performs filtered k-NN search
func (s *ScaNN) SearchWithFilter(query point.Vector, params *SearchParams) ([]Candidate, error) {
	if params.Filter == nil {
		return s.SearchWithParams(query, params)
	}

	// Increase search scope for filtered search
	adjustedParams := &SearchParams{
		K:         params.K,
		NumLeaves: params.NumLeaves * 2,
		NumRerank: params.NumRerank * 2,
		Filter:    params.Filter,
	}

	return s.SearchWithParams(query, adjustedParams)
}

// BatchSearch performs k-NN search for multiple queries
func (s *ScaNN) BatchSearch(queries []point.Vector, k int) ([][]Candidate, error) {
	return s.BatchSearchWithParams(queries, &SearchParams{K: k})
}

// BatchSearchWithParams performs batch search with custom parameters
func (s *ScaNN) BatchSearchWithParams(queries []point.Vector, params *SearchParams) ([][]Candidate, error) {
	results := make([][]Candidate, len(queries))
	errors := make([]error, len(queries))

	var wg sync.WaitGroup
	sem := make(chan struct{}, 32) // Limit concurrency

	for i, query := range queries {
		wg.Add(1)
		sem <- struct{}{}

		go func(idx int, q point.Vector) {
			defer wg.Done()
			defer func() { <-sem }()

			res, err := s.SearchWithParams(q, params)
			results[idx] = res
			errors[idx] = err
		}(i, query)
	}

	wg.Wait()

	// Return first error if any
	for _, err := range errors {
		if err != nil {
			return results, err
		}
	}

	return results, nil
}

// bruteForceSearch performs linear search (used when not trained)
func (s *ScaNN) bruteForceSearch(query point.Vector, k int, filter func(string, map[string]interface{}) bool) ([]Candidate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	results := &MaxCandidateHeap{}
	heap.Init(results)

	for i, p := range s.points {
		if p == nil {
			continue
		}

		// Apply filter if provided
		if filter != nil && !filter(p.ID, p.Payload) {
			continue
		}

		dist := s.distCalc.Distance(query, p.Vector)

		if results.Len() < k {
			heap.Push(results, Candidate{ID: uint32(i), Distance: dist})
		} else if dist < (*results)[0].Distance {
			heap.Pop(results)
			heap.Push(results, Candidate{ID: uint32(i), Distance: dist})
		}
	}

	// Convert to sorted slice
	result := make([]Candidate, results.Len())
	for i := len(result) - 1; i >= 0; i-- {
		result[i] = heap.Pop(results).(Candidate)
	}

	return result, nil
}

// RangeSearch finds all points within a given distance
func (s *ScaNN) RangeSearch(query point.Vector, radius float32) ([]Candidate, error) {
	if len(query) != s.config.Dimension {
		return nil, ErrDimensionMismatch
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []Candidate

	if !s.trained {
		// Brute force for untrained index
		for i, p := range s.points {
			if p == nil {
				continue
			}
			dist := s.distCalc.Distance(query, p.Vector)
			if dist <= radius {
				results = append(results, Candidate{ID: uint32(i), Distance: dist})
			}
		}
	} else {
		// Use partitioning for trained index
		// Search all partitions whose centroids are within 2*radius
		for i := 0; i < s.partitioner.NumLeaves(); i++ {
			leaf := s.partitioner.GetLeaf(i)
			if leaf == nil {
				continue
			}

			// Check if partition might contain points within radius
			centroidDist := s.distCalc.Distance(query, leaf.centroid)
			if centroidDist > 2*radius {
				continue // Skip distant partitions
			}

			leaf.mu.RLock()
			for _, pointID := range leaf.pointIDs {
				if int(pointID) >= len(s.points) {
					continue
				}
				p := s.points[pointID]
				if p == nil {
					continue
				}

				dist := s.distCalc.Distance(query, p.Vector)
				if dist <= radius {
					results = append(results, Candidate{ID: pointID, Distance: dist})
				}
			}
			leaf.mu.RUnlock()
		}
	}

	// Sort by distance
	sort.Slice(results, func(i, j int) bool {
		return results[i].Distance < results[j].Distance
	})

	return results, nil
}

// Recommend finds similar points to a given point ID
func (s *ScaNN) Recommend(id string, k int) ([]Candidate, error) {
	p, err := s.Get(id)
	if err != nil {
		return nil, err
	}

	// Search for k+1 since the query point will be in results
	results, err := s.Search(p.Vector, k+1)
	if err != nil {
		return nil, err
	}

	// Filter out the query point
	filtered := make([]Candidate, 0, k)
	for _, c := range results {
		if s.points[c.ID].ID != id {
			filtered = append(filtered, c)
		}
	}

	if len(filtered) > k {
		filtered = filtered[:k]
	}

	return filtered, nil
}

// DiscoverParams holds discovery parameters
type DiscoverParams struct {
	Target  point.Vector
	Context []ContextPair
	K       int
	Filter  func(id string, payload map[string]interface{}) bool
}

// ContextPair defines positive and negative examples for discovery
type ContextPair struct {
	Positive point.Vector
	Negative point.Vector
}

// Discover performs context-aware discovery search
func (s *ScaNN) Discover(params *DiscoverParams) ([]Candidate, error) {
	if params.Target != nil && len(params.Target) != s.config.Dimension {
		return nil, ErrDimensionMismatch
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	results := &MaxCandidateHeap{}
	heap.Init(results)

	for i, p := range s.points {
		if p == nil {
			continue
		}

		// Apply filter if provided
		if params.Filter != nil && !params.Filter(p.ID, p.Payload) {
			continue
		}

		// Compute context distance
		dist := s.contextDistance(p.Vector, params.Target, params.Context)

		if results.Len() < params.K {
			heap.Push(results, Candidate{ID: uint32(i), Distance: dist})
		} else if dist < (*results)[0].Distance {
			heap.Pop(results)
			heap.Push(results, Candidate{ID: uint32(i), Distance: dist})
		}
	}

	// Convert to sorted slice
	result := make([]Candidate, results.Len())
	for i := len(result) - 1; i >= 0; i-- {
		result[i] = heap.Pop(results).(Candidate)
	}

	return result, nil
}

// contextDistance computes distance with context penalties
func (s *ScaNN) contextDistance(vec point.Vector, target point.Vector, context []ContextPair) float32 {
	var totalPenalty float32

	for _, pair := range context {
		posDist := s.distCalc.Distance(vec, pair.Positive)
		negDist := s.distCalc.Distance(vec, pair.Negative)

		// Penalty if closer to negative than positive
		if posDist > negDist {
			totalPenalty += posDist - negDist
		}
	}

	if len(target) > 0 {
		return totalPenalty + s.distCalc.Distance(vec, target)
	}

	// If no target, add small regularization towards positives
	var regDist float32
	if len(context) > 0 {
		for _, pair := range context {
			regDist += s.distCalc.Distance(vec, pair.Positive)
		}
		regDist /= float32(len(context))
	}

	return totalPenalty + 0.1*regDist
}
