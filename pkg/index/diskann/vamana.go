package diskann

import (
	"math"
	"sync"
	"sync/atomic"

	"github.com/limyedb/limyedb/pkg/distance"
	"github.com/limyedb/limyedb/pkg/point"
)

// VamanaGraph implements the Vamana algorithm for approximate nearest neighbor search
// Vamana is the core algorithm behind DiskANN, designed for billion-scale vector search
type VamanaGraph struct {
	// nodes stores all nodes in the graph indexed by their internal ID
	nodes map[uint32]*Node

	// idToNode maps user-provided string IDs to internal uint32 IDs
	idToNode map[string]uint32

	// Graph parameters
	dimension int     // Vector dimension
	maxDegree int     // R: Maximum number of neighbors per node
	alpha     float32 // Alpha: Diversity factor for RobustPrune
	searchL   int     // L: Search list size

	// Entry point for graph traversal
	entryNode uint32

	// Distance calculator
	distCalc distance.Calculator

	// Statistics
	nodeCount    atomic.Int64
	deletedCount atomic.Int64

	// Concurrency control
	mu sync.RWMutex
}

// NewVamanaGraph creates a new Vamana graph with the specified parameters
func NewVamanaGraph(dimension int, maxDegree int, alpha float32, metric string) *VamanaGraph {
	var distCalc distance.Calculator
	switch metric {
	case "cosine":
		distCalc = distance.New("cosine")
	case "euclidean", "l2":
		distCalc = distance.New("euclidean")
	case "dot", "ip":
		distCalc = distance.New("dot")
	default:
		distCalc = distance.New("euclidean")
	}

	return &VamanaGraph{
		nodes:     make(map[uint32]*Node),
		idToNode:  make(map[string]uint32),
		dimension: dimension,
		maxDegree: maxDegree,
		alpha:     alpha,
		searchL:   100, // Default search L
		distCalc:  distCalc,
	}
}

// SetSearchL sets the search list size
func (g *VamanaGraph) SetSearchL(L int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.searchL = L
}

// Insert adds a new point to the graph
func (g *VamanaGraph) Insert(p *point.Point) error {
	if len(p.Vector) != g.dimension {
		return ErrDimensionMismatch
	}

	g.mu.Lock()
	if _, exists := g.idToNode[p.ID]; exists {
		g.mu.Unlock()
		return ErrPointExists
	}

	// Create new node with overflow check
	nodeCount := len(g.nodes)
	if nodeCount >= math.MaxUint32 {
		g.mu.Unlock()
		return ErrMaxNodesExceeded
	}
	nodeID := uint32(nodeCount) // #nosec G115 - bounds checked above
	node := NewNode(p.ID, p.Vector, g.maxDegree)
	node.SetPayload(p.Payload)

	g.nodes[nodeID] = node
	g.idToNode[p.ID] = nodeID
	g.nodeCount.Add(1)

	// Handle first node
	if len(g.nodes) == 1 {
		g.entryNode = nodeID
		g.mu.Unlock()
		return nil
	}
	g.mu.Unlock()

	// Search for neighbors
	candidates := g.BeamSearch(p.Vector, g.searchL)

	// Convert to node IDs
	candIDs := make([]uint32, len(candidates))
	for i, c := range candidates {
		candIDs[i] = c.ID
	}

	// Select neighbors using RobustPrune
	g.RobustPrune(nodeID, candIDs)

	// Add reverse edges
	g.mu.Lock()
	neighbors := g.nodes[nodeID].GetNeighbors()
	for _, neighborID := range neighbors {
		neighborNode := g.nodes[neighborID]
		neighborNode.AddNeighbor(nodeID, g.maxDegree)
	}
	g.mu.Unlock()

	return nil
}

// Search performs k-NN search on the graph
func (g *VamanaGraph) Search(query point.Vector, k int) ([]Candidate, error) {
	if len(query) != g.dimension {
		return nil, ErrDimensionMismatch
	}

	if k <= 0 {
		return nil, ErrInvalidK
	}

	// Read searchL while holding the lock
	g.mu.RLock()
	L := g.searchL
	g.mu.RUnlock()

	if L < k {
		L = k
	}

	candidates := g.BeamSearch(query, L)

	// Return top k
	if len(candidates) > k {
		candidates = candidates[:k]
	}

	return candidates, nil
}

// Delete marks a point as deleted (lazy deletion)
func (g *VamanaGraph) Delete(id string) error {
	g.mu.RLock()
	nodeID, exists := g.idToNode[id]
	if !exists {
		g.mu.RUnlock()
		return ErrPointNotFound
	}
	node := g.nodes[nodeID]
	g.mu.RUnlock()

	node.MarkDeleted()
	g.deletedCount.Add(1)

	return nil
}

// Get retrieves a point by ID
func (g *VamanaGraph) Get(id string) (*point.Point, error) {
	g.mu.RLock()
	nodeID, exists := g.idToNode[id]
	if !exists {
		g.mu.RUnlock()
		return nil, ErrPointNotFound
	}
	node := g.nodes[nodeID]
	g.mu.RUnlock()

	if node.IsDeleted() {
		return nil, ErrPointNotFound
	}

	return &point.Point{
		ID:      node.ID,
		Vector:  node.Vector,
		Payload: node.GetPayload(),
	}, nil
}

// Size returns the number of points (excluding deleted)
func (g *VamanaGraph) Size() int64 {
	return g.nodeCount.Load() - g.deletedCount.Load()
}

// TotalSize returns total number of points including deleted
func (g *VamanaGraph) TotalSize() int64 {
	return g.nodeCount.Load()
}

// GetNodeID returns the internal node ID for a point ID
func (g *VamanaGraph) GetNodeID(pointID string) (uint32, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	nodeID, exists := g.idToNode[pointID]
	return nodeID, exists
}

// GetPointID returns the point ID for an internal node ID
func (g *VamanaGraph) GetPointID(nodeID uint32) string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if node, exists := g.nodes[nodeID]; exists {
		return node.ID
	}
	return ""
}

// Iterate iterates over all non-deleted point IDs
func (g *VamanaGraph) Iterate(fn func(id string) error) error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	for _, node := range g.nodes {
		if node.IsDeleted() {
			continue
		}
		if err := fn(node.ID); err != nil {
			return err
		}
	}
	return nil
}

// GetAllPoints returns all non-deleted points
func (g *VamanaGraph) GetAllPoints() []*point.Point {
	g.mu.RLock()
	defer g.mu.RUnlock()

	points := make([]*point.Point, 0, len(g.nodes))
	for _, node := range g.nodes {
		if node.IsDeleted() {
			continue
		}
		points = append(points, &point.Point{
			ID:      node.ID,
			Vector:  node.Vector,
			Payload: node.GetPayload(),
		})
	}
	return points
}

// distance calculates distance between two vectors
func (g *VamanaGraph) distance(a, b point.Vector) float32 {
	if g.distCalc != nil {
		return g.distCalc.Distance(a, b)
	}
	return Euclidean(a, b)
}

// getVector returns the vector for a node ID (supports both string and uint32)
func (g *VamanaGraph) getVector(id interface{}) point.Vector {
	switch v := id.(type) {
	case uint32:
		if node, exists := g.nodes[v]; exists {
			return node.Vector
		}
	case string:
		if nodeID, exists := g.idToNode[v]; exists {
			if node, exists := g.nodes[nodeID]; exists {
				return node.Vector
			}
		}
	}
	return nil
}

// Euclidean computes L2 squared distance between two vectors
func Euclidean(a, b point.Vector) float32 {
	var dist float32
	for i := range a {
		diff := a[i] - b[i]
		dist += diff * diff
	}
	return dist
}

// Cosine computes cosine distance between two vectors
func Cosine(a, b point.Vector) float32 {
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA < 1e-8 || normB < 1e-8 {
		return 1.0
	}
	return 1.0 - dot/(float32(math.Sqrt(float64(normA)))*float32(math.Sqrt(float64(normB))))
}

// Recommend finds similar points to a given point
func (g *VamanaGraph) Recommend(id string, k int) ([]Candidate, error) {
	g.mu.RLock()
	nodeID, exists := g.idToNode[id]
	if !exists {
		g.mu.RUnlock()
		return nil, ErrPointNotFound
	}
	node := g.nodes[nodeID]
	g.mu.RUnlock()

	if node.IsDeleted() {
		return nil, ErrPointNotFound
	}

	// Search for similar points
	candidates, err := g.Search(node.Vector, k+1)
	if err != nil {
		return nil, err
	}

	// Filter out the query point itself
	result := make([]Candidate, 0, k)
	for _, c := range candidates {
		pointID := g.GetPointID(c.ID)
		if pointID != id {
			result = append(result, c)
		}
		if len(result) >= k {
			break
		}
	}

	return result, nil
}

// SearchWithEf performs search with custom search list size
func (g *VamanaGraph) SearchWithEf(query point.Vector, k int, ef int) ([]Candidate, error) {
	if len(query) != g.dimension {
		return nil, ErrDimensionMismatch
	}

	if k <= 0 {
		return nil, ErrInvalidK
	}

	if ef < k {
		ef = k
	}

	candidates := g.BeamSearch(query, ef)

	if len(candidates) > k {
		candidates = candidates[:k]
	}

	return candidates, nil
}

// Stats returns graph statistics
type Stats struct {
	TotalNodes   int64
	DeletedNodes int64
	ActiveNodes  int64
	Dimension    int
	MaxDegree    int
	Alpha        float32
	AvgDegree    float64
}

// GetStats returns statistics about the graph
func (g *VamanaGraph) GetStats() Stats {
	g.mu.RLock()
	defer g.mu.RUnlock()

	totalDegree := 0
	activeNodes := 0
	for _, node := range g.nodes {
		if !node.IsDeleted() {
			totalDegree += node.Degree()
			activeNodes++
		}
	}

	avgDegree := 0.0
	if activeNodes > 0 {
		avgDegree = float64(totalDegree) / float64(activeNodes)
	}

	return Stats{
		TotalNodes:   g.nodeCount.Load(),
		DeletedNodes: g.deletedCount.Load(),
		ActiveNodes:  int64(activeNodes),
		Dimension:    g.dimension,
		MaxDegree:    g.maxDegree,
		Alpha:        g.alpha,
		AvgDegree:    avgDegree,
	}
}
