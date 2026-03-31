package diskann

import (
	"math"
	"math/rand"

	"github.com/limyedb/limyedb/pkg/point"
)

// BuildOptions configures the index construction process
type BuildOptions struct {
	// NumPasses is the number of construction passes (typically 2)
	NumPasses int

	// RandomSeed is the seed for random number generation
	RandomSeed int64

	// ShuffleOrder randomizes insertion order in each pass
	ShuffleOrder bool
}

// DefaultBuildOptions returns default build options
func DefaultBuildOptions() *BuildOptions {
	return &BuildOptions{
		NumPasses:    2,
		RandomSeed:   42,
		ShuffleOrder: true,
	}
}

// BuildIndex constructs the Vamana index using the two-pass algorithm
// Pass 1: Random initialization with greedy search
// Pass 2: Quality refinement with RobustPrune
func (g *VamanaGraph) BuildIndex(vectors []point.Vector, ids []string, opts *BuildOptions) error {
	if len(vectors) != len(ids) {
		return ErrDimensionMismatch
	}
	if len(vectors) == 0 {
		return nil
	}

	if opts == nil {
		opts = DefaultBuildOptions()
	}

	rng := rand.New(rand.NewSource(opts.RandomSeed)) // #nosec G404 - not for crypto

	// Create nodes for all vectors
	g.mu.Lock()
	for i, vec := range vectors {
		if len(vec) != g.dimension {
			g.mu.Unlock()
			return ErrDimensionMismatch
		}
		nodeID := uint32(i)
		node := NewNode(ids[i], vec, g.maxDegree)
		g.nodes[nodeID] = node
		g.idToNode[ids[i]] = nodeID
	}

	// Initialize entry point as first node
	g.entryNode = 0
	g.mu.Unlock()

	// Build insertion order
	order := make([]uint32, len(vectors))
	for i := range order {
		order[i] = uint32(i)
	}

	// Two-pass construction
	for pass := 0; pass < opts.NumPasses; pass++ {
		if opts.ShuffleOrder {
			shuffleOrder(rng, order)
		}

		for _, nodeID := range order {
			g.mu.RLock()
			node := g.nodes[nodeID]
			vec := node.Vector
			g.mu.RUnlock()

			// Search for candidates
			candidates := g.BeamSearch(vec, g.searchL)

			// Convert to node IDs
			candIDs := make([]uint32, len(candidates))
			for i, c := range candidates {
				candIDs[i] = c.ID
			}

			// Prune and select neighbors
			if pass == 0 {
				// First pass: simple nearest neighbor selection
				g.SimplePrune(nodeID, candIDs)
			} else {
				// Later passes: use RobustPrune for diversity
				g.RobustPrune(nodeID, candIDs)
			}

			// Add reverse edges
			g.addReverseEdges(nodeID)
		}

		// Update entry point to medoid after each pass
		g.mu.Lock()
		g.entryNode = g.ComputeMedoid()
		g.mu.Unlock()
	}

	return nil
}

// addReverseEdges adds bidirectional edges for a node
func (g *VamanaGraph) addReverseEdges(nodeID uint32) {
	g.mu.Lock()
	defer g.mu.Unlock()

	node := g.nodes[nodeID]
	neighbors := node.GetNeighbors()

	for _, neighborID := range neighbors {
		neighborNode := g.nodes[neighborID]

		// Try to add reverse edge
		currentNeighbors := neighborNode.GetNeighbors()
		if len(currentNeighbors) < g.maxDegree {
			// Simple add if under capacity
			neighborNode.AddNeighbor(nodeID, g.maxDegree)
		} else {
			// Need to potentially prune
			candidates := append(currentNeighbors, nodeID)
			g.robustPruneInternal(neighborID, candidates)
		}
	}
}

// IncrementalInsert adds a new point to an existing index
func (g *VamanaGraph) IncrementalInsert(vec point.Vector, id string) error {
	if len(vec) != g.dimension {
		return ErrDimensionMismatch
	}

	g.mu.Lock()
	if _, exists := g.idToNode[id]; exists {
		g.mu.Unlock()
		return ErrPointExists
	}

	// Add node with overflow check
	nodeCount := len(g.nodes)
	if nodeCount >= math.MaxUint32 {
		g.mu.Unlock()
		return ErrMaxNodesExceeded
	}
	nodeID := uint32(nodeCount) // #nosec G115 - bounds checked above
	node := NewNode(id, vec, g.maxDegree)
	g.nodes[nodeID] = node
	g.idToNode[id] = nodeID
	g.mu.Unlock()

	// Handle first node
	g.mu.Lock()
	if len(g.nodes) == 1 {
		g.entryNode = nodeID
		g.mu.Unlock()
		return nil
	}
	g.mu.Unlock()

	// Search for neighbors
	candidates := g.BeamSearch(vec, g.searchL)

	// Convert to node IDs
	candIDs := make([]uint32, len(candidates))
	for i, c := range candidates {
		candIDs[i] = c.ID
	}

	// Select neighbors
	g.RobustPrune(nodeID, candIDs)

	// Add reverse edges
	g.addReverseEdges(nodeID)

	return nil
}

// shuffleOrder shuffles the insertion order using Fisher-Yates
func shuffleOrder(rng *rand.Rand, order []uint32) {
	for i := len(order) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		order[i], order[j] = order[j], order[i]
	}
}
