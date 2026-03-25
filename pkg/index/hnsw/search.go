package hnsw

import (
	"container/heap"
	"sort"
	"sync"

	"github.com/limyedb/limyedb/pkg/point"
)

// SearchParams holds search parameters
type SearchParams struct {
	K                    int     // Number of results to return
	Ef                   int     // Search quality parameter
	Radius               float32 // Optional: maximum distance (for range search)
	Filter               func(id string, payload map[string]interface{}) bool
	EstimatedSelectivity float64 // Optional: estimated fraction of points passing filter (0-1)
}

// DefaultSearchParams returns default search parameters
func DefaultSearchParams(k int) *SearchParams {
	return &SearchParams{
		K:  k,
		Ef: 100,
	}
}

// ContextPair defines a positive and negative vector for context discovery
type ContextPair struct {
	Positive point.Vector
	Negative point.Vector
}

// DiscoverParams holds discovery parameters
type DiscoverParams struct {
	Target  point.Vector // Optional target
	Context []ContextPair
	K       int
	Ef      int
	Filter  func(id string, payload map[string]interface{}) bool
}

// SearchWithFilter performs filtered k-NN search
func (h *HNSW) SearchWithFilter(query point.Vector, params *SearchParams) ([]Candidate, error) {
	if len(query) != h.dimension {
		return nil, ErrDimensionMismatch
	}

	h.mu.RLock()
	if h.maxLevel == -1 {
		h.mu.RUnlock()
		return nil, nil
	}
	entryPoint := h.entryPoint
	maxLevel := h.maxLevel
	h.mu.RUnlock()

	ef := params.Ef
	if ef < params.K {
		ef = params.K
	}

	searchEf := ef
	if h.quantizer != nil {
		searchEf = ef * 2 // Over-fetch for Stage 2 Rescoring
	}

	// Traverse from top to bottom
	currNode := entryPoint
	for l := maxLevel; l > 0; l-- {
		currNode = h.greedySearchLayer(query, currNode, l)
	}

	// Search at layer 0 with filtering
	candidates := h.searchLayerWithFilter(query, currNode, searchEf, 0, params)

	// Stage 2: Exact Rescoring
	if h.quantizer != nil {
		h.mu.RLock()
		for i := range candidates {
			// Rescore with exact distance using virtual mapping natively
			exactDist := h.distCalc.Distance(query, h.getVector(candidates[i].ID))
			candidates[i].Distance = exactDist
		}
		h.mu.RUnlock()

		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].Distance < candidates[j].Distance
		})
	}

	// Return top k
	if len(candidates) > params.K {
		candidates = candidates[:params.K]
	}

	return candidates, nil
}

// calculateAdaptiveEf determines the search ef based on estimated filter selectivity
// Lower selectivity (more restrictive filter) requires higher ef to find enough candidates
func calculateAdaptiveEf(ef int, estimatedSelectivity float64) int {
	if estimatedSelectivity <= 0 || estimatedSelectivity > 1.0 {
		// Default: assume 50% selectivity
		estimatedSelectivity = 0.5
	}

	// Adaptive multiplier based on selectivity
	// High selectivity (>80%): minimal over-fetch (1.5x)
	// Medium selectivity (20-80%): moderate over-fetch (2-3x)
	// Low selectivity (<20%): aggressive over-fetch (3-10x)
	var multiplier float64
	switch {
	case estimatedSelectivity >= 0.8:
		multiplier = 1.5
	case estimatedSelectivity >= 0.5:
		multiplier = 2.0
	case estimatedSelectivity >= 0.2:
		multiplier = 3.0
	case estimatedSelectivity >= 0.1:
		multiplier = 5.0
	case estimatedSelectivity >= 0.05:
		multiplier = 7.0
	default:
		multiplier = 10.0
	}

	adaptiveEf := int(float64(ef) * multiplier)

	// Cap at reasonable maximum to prevent runaway searches
	maxEf := ef * 15
	if adaptiveEf > maxEf {
		adaptiveEf = maxEf
	}

	return adaptiveEf
}

// searchLayerWithFilter searches with post-filtering
func (h *HNSW) searchLayerWithFilter(query point.Vector, entryID uint32, ef int, layer int, params *SearchParams) []Candidate {
	h.mu.RLock()
	defer h.mu.RUnlock()

	visited := h.visitedPool.Get()
	defer h.visitedPool.Put(visited)
	visited.Add(entryID)

	// Adaptive ef based on estimated filter selectivity
	// Use provided estimate or default to 33% selectivity (3x multiplier)
	selectivity := params.EstimatedSelectivity
	if selectivity <= 0 {
		selectivity = 0.33
	}
	searchEf := calculateAdaptiveEf(ef, selectivity)

	candidates := &CandidateHeap{}
	heap.Init(candidates)
	results := &MaxCandidateHeap{}
	heap.Init(results)

	var entryDist float32
	if h.quantizer != nil && h.nodes[entryID].Quantized != nil {
		entryDist = h.quantizer.Distance(query, h.nodes[entryID].Quantized)
	} else {
		entryDist = h.distCalc.Distance(query, h.getVector(entryID))
	}

	// Check if entry passes filter
	entryNode := h.nodes[entryID]
	passesFilter := params.Filter == nil || params.Filter(entryNode.ID, entryNode.GetPayload())

	heap.Push(candidates, Candidate{ID: entryID, Distance: entryDist})
	if passesFilter && !entryNode.IsDeleted() {
		heap.Push(results, Candidate{ID: entryID, Distance: entryDist})
	}

	for candidates.Len() > 0 {
		curr := heap.Pop(candidates).(Candidate)

		// Early termination
		if results.Len() >= ef && curr.Distance > (*results)[0].Distance {
			break
		}

		neighbors := h.getConnections(curr.ID, layer)
		for _, neighborID := range neighbors {
			if visited.Add(neighborID) {
				continue
			}

			neighborNode := h.nodes[neighborID]
			if neighborNode.IsDeleted() {
				continue
			}

			var dist float32
			if h.quantizer != nil && neighborNode.Quantized != nil {
				dist = h.quantizer.Distance(query, neighborNode.Quantized)
			} else {
				dist = h.distCalc.Distance(query, h.getVector(neighborID))
			}

			// Always add to candidates for exploration
			if results.Len() < searchEf || dist < (*results)[0].Distance {
				heap.Push(candidates, Candidate{ID: neighborID, Distance: dist})
			}

			// Apply filter for results
			if params.Filter != nil && !params.Filter(neighborNode.ID, neighborNode.GetPayload()) {
				continue
			}

			// Apply radius filter
			if params.Radius > 0 && dist > params.Radius {
				continue
			}

			if results.Len() < ef || dist < (*results)[0].Distance {
				heap.Push(results, Candidate{ID: neighborID, Distance: dist})
				for results.Len() > ef {
					heap.Pop(results)
				}
			}
		}
	}

	// Convert to sorted slice
	result := make([]Candidate, results.Len())
	for i := len(result) - 1; i >= 0; i-- {
		result[i] = heap.Pop(results).(Candidate)
	}
	return result
}

// contextDistance calculates the Discovery API distance penalty.
// Lower is better. If distance to positive is smaller than negative, penalty is 0.
func (h *HNSW) contextDistance(vec point.Vector, target point.Vector, context []ContextPair) float32 {
	var totalPenalty float32 = 0

	for _, pair := range context {
		posDist := h.distCalc.Distance(vec, pair.Positive)
		negDist := h.distCalc.Distance(vec, pair.Negative)
		
		// If closer to negative than positive, add penalty
		if posDist > negDist {
			totalPenalty += posDist - negDist
		}
	}

	if len(target) > 0 {
		return totalPenalty + h.distCalc.Distance(vec, target)
	}
	
	// If no target, we just use the penalty (often points that fit the context perfectly tie at 0)
	// We might add a small regularizer to distance it from positives
	var regDist float32 = 0
	if len(context) > 0 {
		for _, pair := range context {
			regDist += h.distCalc.Distance(vec, pair.Positive)
		}
		regDist /= float32(len(context))
	}
	
	return totalPenalty + 0.1*regDist // slight preference to be near positives even if penalty is 0
}

// DiscoverWithFilter performs context discovery search
func (h *HNSW) DiscoverWithFilter(params *DiscoverParams) ([]Candidate, error) {
	h.mu.RLock()
	if h.maxLevel == -1 {
		h.mu.RUnlock()
		return nil, nil
	}
	entryPoint := h.entryPoint
	h.mu.RUnlock()

	ef := params.Ef
	if ef < params.K {
		ef = params.K
	}

	// We start at layer 0 from the entrypoint since context distance is highly non-convex
	// Upper layers would drop us far away. This is true to Qdrant's approach for Discover.
	candidates := h.discoverLayer0(entryPoint, ef, params)

	if len(candidates) > params.K {
		candidates = candidates[:params.K]
	}

	return candidates, nil
}

func (h *HNSW) discoverLayer0(entryID uint32, ef int, params *DiscoverParams) []Candidate {
	h.mu.RLock()
	defer h.mu.RUnlock()

	visited := h.visitedPool.Get()
	defer h.visitedPool.Put(visited)
	visited.Add(entryID)

	searchEf := ef * 3 // Over-fetch for filtering and discovery non-monotonicity

	candidates := &CandidateHeap{}
	heap.Init(candidates)
	results := &MaxCandidateHeap{}
	// Evaluate entry point dynamically natively
	heap.Init(results)

	entryNode := h.nodes[entryID]
	entryDist := h.contextDistance(h.getVector(entryID), params.Target, params.Context)

	passesFilter := params.Filter == nil || params.Filter(entryNode.ID, entryNode.GetPayload())

	heap.Push(candidates, Candidate{ID: entryID, Distance: entryDist})
	if passesFilter && !entryNode.IsDeleted() {
		heap.Push(results, Candidate{ID: entryID, Distance: entryDist})
	}

	for candidates.Len() > 0 {
		curr := heap.Pop(candidates).(Candidate)

		if results.Len() >= searchEf && curr.Distance > (*results)[0].Distance {
			break
		}

		neighbors := h.getConnections(curr.ID, 0)
		for _, neighborID := range neighbors {
			if visited.Add(neighborID) {
				continue
			}

			neighborNode := h.nodes[neighborID]
			if neighborNode.IsDeleted() {
				continue
			}

			dist := h.contextDistance(h.getVector(neighborID), params.Target, params.Context)

			if results.Len() < searchEf || dist < (*results)[0].Distance {
				heap.Push(candidates, Candidate{ID: neighborID, Distance: dist})
			}

			if params.Filter != nil && !params.Filter(neighborNode.ID, neighborNode.GetPayload()) {
				continue
			}

			if results.Len() < ef || dist < (*results)[0].Distance {
				heap.Push(results, Candidate{ID: neighborID, Distance: dist})
				for results.Len() > ef {
					heap.Pop(results)
				}
			}
		}
	}

	result := make([]Candidate, results.Len())
	for i := len(result) - 1; i >= 0; i-- {
		result[i] = heap.Pop(results).(Candidate)
	}
	return result
}

// RangeSearch finds all points within a given distance
func (h *HNSW) RangeSearch(query point.Vector, radius float32) ([]Candidate, error) {
	if len(query) != h.dimension {
		return nil, ErrDimensionMismatch
	}

	h.mu.RLock()
	if h.maxLevel == -1 {
		h.mu.RUnlock()
		return nil, nil
	}
	entryPoint := h.entryPoint
	maxLevel := h.maxLevel
	h.mu.RUnlock()

	// Start from top layer
	currNode := entryPoint
	for l := maxLevel; l > 0; l-- {
		currNode = h.greedySearchLayer(query, currNode, l)
	}

	// BFS-like search at layer 0
	return h.rangeSearchLayer0(query, currNode, radius), nil
}

// rangeSearchLayer0 performs range search at layer 0
func (h *HNSW) rangeSearchLayer0(query point.Vector, entryID uint32, radius float32) []Candidate {
	h.mu.RLock()
	defer h.mu.RUnlock()

	visited := h.visitedPool.Get()
	defer h.visitedPool.Put(visited)
	visited.Add(entryID)

	var results []Candidate
	queue := []uint32{entryID}

	for len(queue) > 0 {
		currID := queue[0]
		queue = queue[1:]

		node := h.nodes[currID]
		if node.IsDeleted() {
			continue
		}

		// Process dynamically mapped bounds natively 
		dist := h.distCalc.Distance(query, h.getVector(currID))
		if dist <= radius {
			results = append(results, Candidate{ID: currID, Distance: dist})
		}

		// Explore neighbors
		neighbors := h.getConnections(currID, 0)
		for _, neighborID := range neighbors {
			if visited.Add(neighborID) {
				continue
			}
			queue = append(queue, neighborID)
		}
	}

	return results
}

// BatchSearch performs k-NN search for multiple queries in parallel
func (h *HNSW) BatchSearch(queries []point.Vector, k int) ([][]Candidate, error) {
	return h.BatchSearchWithEf(queries, k, h.efSearch)
}

// BatchSearchWithEf performs batch search with custom ef
func (h *HNSW) BatchSearchWithEf(queries []point.Vector, k int, ef int) ([][]Candidate, error) {
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

			res, err := h.SearchWithEf(q, k, ef)
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

// Recommend finds similar points to a given point ID
func (h *HNSW) Recommend(id string, k int) ([]Candidate, error) {
	p, err := h.Get(id)
	if err != nil {
		return nil, err
	}

	// Search for k+1 since the query point will be in results
	results, err := h.Search(p.Vector, k+1)
	if err != nil {
		return nil, err
	}

	// Filter out the query point
	filtered := make([]Candidate, 0, k)
	for _, c := range results {
		if h.nodes[c.ID].ID != id {
			filtered = append(filtered, c)
		}
	}

	if len(filtered) > k {
		filtered = filtered[:k]
	}

	return filtered, nil
}

// GetNeighbors returns the direct neighbors of a point
func (h *HNSW) GetNeighbors(id string, layer int) ([]string, error) {
	h.mu.RLock()
	nodeID, exists := h.idToIndex[id]
	h.mu.RUnlock()

	if !exists {
		return nil, ErrPointNotFound
	}

	connections := h.getConnections(nodeID, layer)

	neighbors := make([]string, 0, len(connections))
	for _, connID := range connections {
		neighbors = append(neighbors, h.nodes[connID].ID)
	}

	return neighbors, nil
}

// GetLevel returns the level of a point in the graph
func (h *HNSW) GetLevel(id string) (int, error) {
	h.mu.RLock()
	nodeID, exists := h.idToIndex[id]
	h.mu.RUnlock()

	if !exists {
		return -1, ErrPointNotFound
	}

	return h.nodes[nodeID].Level, nil
}
