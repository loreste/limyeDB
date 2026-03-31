package diskann

import (
	"container/heap"

	"github.com/limyedb/limyedb/pkg/point"
)

// SearchParams holds parameters for search operations
type SearchParams struct {
	// K is the number of results to return
	K int

	// L is the search list size (beam width)
	L int

	// Filter is an optional filter function
	Filter func(id string, payload map[string]interface{}) bool
}

// BeamSearch performs greedy beam search on the Vamana graph
// Returns up to L candidates, sorted by distance
func (g *VamanaGraph) BeamSearch(query point.Vector, L int) []Candidate {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if len(g.nodes) == 0 {
		return nil
	}

	if L <= 0 {
		L = g.searchL
	}

	visited := make(map[uint32]bool)
	visited[g.entryNode] = true

	// Initialize with entry point
	entryDist := g.distance(query, g.getVector(g.entryNode))

	// Candidates to explore (min-heap)
	candidates := &CandidateHeap{}
	heap.Init(candidates)
	heap.Push(candidates, Candidate{ID: g.entryNode, Distance: entryDist})

	// Best results (max-heap for easy pruning)
	results := &MaxCandidateHeap{}
	heap.Init(results)
	heap.Push(results, Candidate{ID: g.entryNode, Distance: entryDist})

	for candidates.Len() > 0 {
		// Get closest unvisited candidate
		curr := heap.Pop(candidates).(Candidate)

		// Early termination: if current is worse than worst in results and we have L results
		if results.Len() >= L && curr.Distance > (*results)[0].Distance {
			break
		}

		// Explore neighbors
		node := g.nodes[curr.ID]
		neighbors := node.GetNeighbors()

		for _, neighborID := range neighbors {
			if visited[neighborID] {
				continue
			}
			visited[neighborID] = true

			if g.nodes[neighborID].IsDeleted() {
				continue
			}

			dist := g.distance(query, g.getVector(neighborID))

			// Add to results if better than worst or we don't have enough
			if results.Len() < L || dist < (*results)[0].Distance {
				heap.Push(candidates, Candidate{ID: neighborID, Distance: dist})
				heap.Push(results, Candidate{ID: neighborID, Distance: dist})

				// Keep only L best results
				if results.Len() > L {
					heap.Pop(results)
				}
			}
		}
	}

	// Convert max-heap to sorted slice
	result := make([]Candidate, results.Len())
	for i := len(result) - 1; i >= 0; i-- {
		result[i] = heap.Pop(results).(Candidate)
	}

	return result
}

// SearchWithFilter performs filtered beam search
func (g *VamanaGraph) SearchWithFilter(query point.Vector, params *SearchParams) []Candidate {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if len(g.nodes) == 0 {
		return nil
	}

	L := params.L
	if L <= 0 {
		L = g.searchL
	}

	visited := make(map[uint32]bool)
	visited[g.entryNode] = true

	// Initialize with entry point
	entryDist := g.distance(query, g.getVector(g.entryNode))

	candidates := &CandidateHeap{}
	heap.Init(candidates)
	heap.Push(candidates, Candidate{ID: g.entryNode, Distance: entryDist})

	// Results that pass the filter
	var filteredResults []Candidate

	// All explored candidates for graph traversal
	results := &MaxCandidateHeap{}
	heap.Init(results)
	heap.Push(results, Candidate{ID: g.entryNode, Distance: entryDist})

	for candidates.Len() > 0 && len(filteredResults) < params.K*2 {
		curr := heap.Pop(candidates).(Candidate)

		// Check filter
		node := g.nodes[curr.ID]
		if !node.IsDeleted() {
			if params.Filter == nil || params.Filter(node.ID, node.GetPayload()) {
				filteredResults = append(filteredResults, curr)
			}
		}

		// Early termination
		if results.Len() >= L && curr.Distance > (*results)[0].Distance {
			break
		}

		// Explore neighbors
		neighbors := node.GetNeighbors()
		for _, neighborID := range neighbors {
			if visited[neighborID] {
				continue
			}
			visited[neighborID] = true

			if g.nodes[neighborID].IsDeleted() {
				continue
			}

			dist := g.distance(query, g.getVector(neighborID))

			if results.Len() < L || dist < (*results)[0].Distance {
				heap.Push(candidates, Candidate{ID: neighborID, Distance: dist})
				heap.Push(results, Candidate{ID: neighborID, Distance: dist})

				if results.Len() > L {
					heap.Pop(results)
				}
			}
		}
	}

	// Sort filtered results and return top K
	sortCandidates(filteredResults)
	if len(filteredResults) > params.K {
		filteredResults = filteredResults[:params.K]
	}

	return filteredResults
}

// BatchSearch performs batch beam search for multiple queries
func (g *VamanaGraph) BatchSearch(queries []point.Vector, k, L int) [][]Candidate {
	results := make([][]Candidate, len(queries))
	for i, query := range queries {
		candidates := g.BeamSearch(query, L)
		if len(candidates) > k {
			candidates = candidates[:k]
		}
		results[i] = candidates
	}
	return results
}

// RangeSearch finds all points within a given radius
func (g *VamanaGraph) RangeSearch(query point.Vector, radius float32, maxResults int) []Candidate {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if len(g.nodes) == 0 {
		return nil
	}

	visited := make(map[uint32]bool)
	visited[g.entryNode] = true

	var results []Candidate

	// BFS-like traversal
	queue := []uint32{g.entryNode}

	for len(queue) > 0 && (maxResults <= 0 || len(results) < maxResults) {
		curr := queue[0]
		queue = queue[1:]

		node := g.nodes[curr]
		if node.IsDeleted() {
			continue
		}

		dist := g.distance(query, g.getVector(curr))
		if dist <= radius {
			results = append(results, Candidate{ID: curr, Distance: dist})
		}

		// Explore neighbors that might be within radius
		neighbors := node.GetNeighbors()
		for _, neighborID := range neighbors {
			if !visited[neighborID] {
				visited[neighborID] = true
				queue = append(queue, neighborID)
			}
		}
	}

	sortCandidates(results)
	return results
}

// sortCandidates sorts candidates by distance (ascending)
func sortCandidates(candidates []Candidate) {
	for i := 1; i < len(candidates); i++ {
		key := candidates[i]
		j := i - 1
		for j >= 0 && candidates[j].Distance > key.Distance {
			candidates[j+1] = candidates[j]
			j--
		}
		candidates[j+1] = key
	}
}
