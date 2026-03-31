package diskann

import (
	"sort"

	"github.com/limyedb/limyedb/pkg/point"
)

// RobustPrune implements the alpha-diversity neighbor selection algorithm
// from the Vamana paper. It selects neighbors that are both close to the node
// and geometrically diverse from each other.
func (g *VamanaGraph) RobustPrune(nodeID uint32, candidates []uint32) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.robustPruneInternal(nodeID, candidates)
}

// robustPruneInternal performs pruning without holding the lock (for internal use)
func (g *VamanaGraph) robustPruneInternal(nodeID uint32, candidates []uint32) {
	node := g.nodes[nodeID]
	nodeVec := g.getVector(nodeID)

	// Collect all candidates (existing neighbors + new candidates)
	allCandidates := make(map[uint32]bool)
	for _, n := range node.GetNeighbors() {
		if n != nodeID {
			allCandidates[n] = true
		}
	}
	for _, c := range candidates {
		if c != nodeID {
			allCandidates[c] = true
		}
	}

	// Calculate distances to all candidates
	type candDist struct {
		id   uint32
		dist float32
	}

	pool := make([]candDist, 0, len(allCandidates))
	for c := range allCandidates {
		if g.nodes[c].IsDeleted() {
			continue
		}
		pool = append(pool, candDist{
			id:   c,
			dist: g.distance(nodeVec, g.getVector(c)),
		})
	}

	// Sort by distance
	sort.Slice(pool, func(i, j int) bool {
		return pool[i].dist < pool[j].dist
	})

	// Select diverse neighbors using alpha pruning
	newNeighbors := make([]uint32, 0, g.maxDegree)

	for _, p := range pool {
		if len(newNeighbors) >= g.maxDegree {
			break
		}

		// Check if candidate is alpha-diverse from selected neighbors
		valid := true
		for _, n := range newNeighbors {
			// If dist(p, n) * alpha <= dist(node, p), then p is too close to n
			distPN := g.distance(g.getVector(p.id), g.getVector(n))
			if g.alpha*distPN <= p.dist {
				valid = false
				break
			}
		}

		if valid {
			newNeighbors = append(newNeighbors, p.id)
		}
	}

	node.SetNeighbors(newNeighbors)
}

// SimplePrune selects the k nearest neighbors without diversity consideration
// Useful for quick index construction
func (g *VamanaGraph) SimplePrune(nodeID uint32, candidates []uint32) {
	g.mu.Lock()
	defer g.mu.Unlock()

	node := g.nodes[nodeID]
	nodeVec := g.getVector(nodeID)

	// Collect all candidates
	allCandidates := make(map[uint32]bool)
	for _, n := range node.GetNeighbors() {
		if n != nodeID {
			allCandidates[n] = true
		}
	}
	for _, c := range candidates {
		if c != nodeID {
			allCandidates[c] = true
		}
	}

	// Calculate distances
	type candDist struct {
		id   uint32
		dist float32
	}

	pool := make([]candDist, 0, len(allCandidates))
	for c := range allCandidates {
		if g.nodes[c].IsDeleted() {
			continue
		}
		pool = append(pool, candDist{
			id:   c,
			dist: g.distance(nodeVec, g.getVector(c)),
		})
	}

	// Sort by distance
	sort.Slice(pool, func(i, j int) bool {
		return pool[i].dist < pool[j].dist
	})

	// Take top R neighbors
	newNeighbors := make([]uint32, 0, g.maxDegree)
	for i := 0; i < len(pool) && i < g.maxDegree; i++ {
		newNeighbors = append(newNeighbors, pool[i].id)
	}

	node.SetNeighbors(newNeighbors)
}

// EnsureBidirectional ensures all edges are bidirectional
// This is useful for maintaining graph connectivity
func (g *VamanaGraph) EnsureBidirectional(nodeID uint32) {
	g.mu.Lock()
	defer g.mu.Unlock()

	node := g.nodes[nodeID]
	neighbors := node.GetNeighbors()

	for _, neighborID := range neighbors {
		neighborNode := g.nodes[neighborID]

		// Check if reverse edge exists
		hasReverse := false
		for _, n := range neighborNode.GetNeighbors() {
			if n == nodeID {
				hasReverse = true
				break
			}
		}

		// Add reverse edge if needed (up to max degree)
		if !hasReverse {
			neighborNode.AddNeighbor(nodeID, g.maxDegree)
		}
	}
}

// ComputeMedoid finds the point closest to the centroid of all vectors
// This is used as the entry point for search
func (g *VamanaGraph) ComputeMedoid() uint32 {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if len(g.nodes) == 0 {
		return 0
	}

	// Compute centroid
	dim := g.dimension
	centroid := make(point.Vector, dim)
	count := 0

	for _, node := range g.nodes {
		if node.IsDeleted() {
			continue
		}
		vec := g.getVector(node.ID)
		for i := 0; i < dim; i++ {
			centroid[i] += vec[i]
		}
		count++
	}

	if count == 0 {
		return g.entryNode
	}

	for i := 0; i < dim; i++ {
		centroid[i] /= float32(count)
	}

	// Find node closest to centroid
	var medoid uint32
	minDist := float32(1e30)

	for id, node := range g.nodes {
		if node.IsDeleted() {
			continue
		}
		dist := g.distance(centroid, g.getVector(id))
		if dist < minDist {
			minDist = dist
			medoid = id
		}
	}

	return medoid
}
