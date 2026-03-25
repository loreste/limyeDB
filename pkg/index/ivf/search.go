package ivf

import (
	"container/heap"
	"sort"
	"sync"

	"github.com/limyedb/limyedb/pkg/point"
)

// SearchParams holds search parameters
type SearchParams struct {
	K       int                                             // Number of results to return
	Nprobe  int                                             // Number of clusters to search (overrides default)
	Filter  func(id string, payload map[string]interface{}) bool
}

// DefaultSearchParams returns default search parameters
func DefaultSearchParams(k int) *SearchParams {
	return &SearchParams{
		K:      k,
		Nprobe: 0, // Use index default
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
	*h = append(*h, x.(Candidate))
}

func (h *CandidateHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// MaxCandidateHeap is a max-heap for tracking worst results (largest distance first)
type MaxCandidateHeap []Candidate

func (h MaxCandidateHeap) Len() int           { return len(h) }
func (h MaxCandidateHeap) Less(i, j int) bool { return h[i].Distance > h[j].Distance }
func (h MaxCandidateHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *MaxCandidateHeap) Push(x interface{}) {
	*h = append(*h, x.(Candidate))
}

func (h *MaxCandidateHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// Search performs k-NN search
func (ivf *IVF) Search(query point.Vector, k int) ([]Candidate, error) {
	return ivf.SearchWithParams(query, &SearchParams{K: k})
}

// SearchWithParams performs k-NN search with custom parameters
func (ivf *IVF) SearchWithParams(query point.Vector, params *SearchParams) ([]Candidate, error) {
	if len(query) != ivf.config.Dimension {
		return nil, ErrDimensionMismatch
	}

	ivf.mu.RLock()
	trained := ivf.trained
	ivf.mu.RUnlock()

	if !trained {
		// Fall back to brute force search if not trained
		return ivf.bruteForceSearch(query, params.K, params.Filter)
	}

	nprobe := params.Nprobe
	if nprobe <= 0 {
		nprobe = ivf.config.Nprobe
	}

	// Find nearest clusters
	nearestClusters := ivf.kmeans.FindNearestCentroids(query, nprobe)

	// Search within selected clusters
	results := &MaxCandidateHeap{}
	heap.Init(results)

	ivf.mu.RLock()
	defer ivf.mu.RUnlock()

	for _, cd := range nearestClusters {
		cluster, exists := ivf.clusters[cd.Index]
		if !exists {
			continue
		}

		cluster.mu.RLock()
		for _, pointID := range cluster.PointIDs {
			if int(pointID) >= len(ivf.points) {
				continue
			}

			p := ivf.points[pointID]
			if p == nil {
				continue
			}

			// Apply filter if provided
			if params.Filter != nil && !params.Filter(p.ID, p.Payload) {
				continue
			}

			// Calculate distance
			var dist float32
			if ivf.quantizer != nil {
				// Use quantized distance if available
				qData, err := ivf.quantizer.Encode(p.Vector)
				if err == nil {
					dist = ivf.quantizer.Distance(query, qData)
				} else {
					dist = ivf.distCalc.Distance(query, p.Vector)
				}
			} else {
				dist = ivf.distCalc.Distance(query, p.Vector)
			}

			// Add to results
			if results.Len() < params.K {
				heap.Push(results, Candidate{ID: pointID, Distance: dist})
			} else if dist < (*results)[0].Distance {
				heap.Pop(results)
				heap.Push(results, Candidate{ID: pointID, Distance: dist})
			}
		}
		cluster.mu.RUnlock()
	}

	// Convert to sorted slice
	result := make([]Candidate, results.Len())
	for i := len(result) - 1; i >= 0; i-- {
		result[i] = heap.Pop(results).(Candidate)
	}

	// Rescore with exact distances if using quantization
	if ivf.quantizer != nil {
		for i := range result {
			p := ivf.points[result[i].ID]
			result[i].Distance = ivf.distCalc.Distance(query, p.Vector)
		}
		sort.Slice(result, func(i, j int) bool {
			return result[i].Distance < result[j].Distance
		})
	}

	return result, nil
}

// SearchWithFilter performs filtered k-NN search
func (ivf *IVF) SearchWithFilter(query point.Vector, params *SearchParams) ([]Candidate, error) {
	if params.Filter == nil {
		return ivf.SearchWithParams(query, params)
	}

	// Increase nprobe for filtered search to compensate for filtered-out results
	adjustedParams := &SearchParams{
		K:      params.K,
		Nprobe: params.Nprobe * 2, // Double nprobe for filtering
		Filter: params.Filter,
	}

	if adjustedParams.Nprobe > ivf.NumClusters() {
		adjustedParams.Nprobe = ivf.NumClusters()
	}

	return ivf.SearchWithParams(query, adjustedParams)
}

// BatchSearch performs k-NN search for multiple queries
func (ivf *IVF) BatchSearch(queries []point.Vector, k int) ([][]Candidate, error) {
	return ivf.BatchSearchWithParams(queries, &SearchParams{K: k})
}

// BatchSearchWithParams performs batch search with custom parameters
func (ivf *IVF) BatchSearchWithParams(queries []point.Vector, params *SearchParams) ([][]Candidate, error) {
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

			res, err := ivf.SearchWithParams(q, params)
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
func (ivf *IVF) bruteForceSearch(query point.Vector, k int, filter func(string, map[string]interface{}) bool) ([]Candidate, error) {
	ivf.mu.RLock()
	defer ivf.mu.RUnlock()

	results := &MaxCandidateHeap{}
	heap.Init(results)

	for i, p := range ivf.points {
		if p == nil {
			continue
		}

		// Apply filter if provided
		if filter != nil && !filter(p.ID, p.Payload) {
			continue
		}

		dist := ivf.distCalc.Distance(query, p.Vector)

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
func (ivf *IVF) RangeSearch(query point.Vector, radius float32) ([]Candidate, error) {
	if len(query) != ivf.config.Dimension {
		return nil, ErrDimensionMismatch
	}

	ivf.mu.RLock()
	defer ivf.mu.RUnlock()

	var results []Candidate

	if !ivf.trained {
		// Brute force for untrained index
		for i, p := range ivf.points {
			if p == nil {
				continue
			}
			dist := ivf.distCalc.Distance(query, p.Vector)
			if dist <= radius {
				results = append(results, Candidate{ID: uint32(i), Distance: dist})
			}
		}
		return results, nil
	}

	// Find all clusters within radius of query
	// Start with a reasonable number and expand if needed
	allClusters := ivf.kmeans.FindNearestCentroids(query, len(ivf.clusters))

	for _, cd := range allClusters {
		// Skip clusters whose centroid is too far (optimization)
		// Points in a cluster can be at most cluster_radius away from centroid
		// We use 2*radius as a conservative estimate
		if cd.Distance > 2*radius {
			continue
		}

		cluster, exists := ivf.clusters[cd.Index]
		if !exists {
			continue
		}

		cluster.mu.RLock()
		for _, pointID := range cluster.PointIDs {
			if int(pointID) >= len(ivf.points) {
				continue
			}
			p := ivf.points[pointID]
			if p == nil {
				continue
			}

			dist := ivf.distCalc.Distance(query, p.Vector)
			if dist <= radius {
				results = append(results, Candidate{ID: pointID, Distance: dist})
			}
		}
		cluster.mu.RUnlock()
	}

	// Sort by distance
	sort.Slice(results, func(i, j int) bool {
		return results[i].Distance < results[j].Distance
	})

	return results, nil
}

// Recommend finds similar points to a given point ID
func (ivf *IVF) Recommend(id string, k int) ([]Candidate, error) {
	p, err := ivf.Get(id)
	if err != nil {
		return nil, err
	}

	// Search for k+1 since the query point will be in results
	results, err := ivf.Search(p.Vector, k+1)
	if err != nil {
		return nil, err
	}

	// Filter out the query point
	filtered := make([]Candidate, 0, k)
	for _, c := range results {
		if ivf.points[c.ID].ID != id {
			filtered = append(filtered, c)
		}
	}

	if len(filtered) > k {
		filtered = filtered[:k]
	}

	return filtered, nil
}
