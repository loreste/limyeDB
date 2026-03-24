package hnsw

import (
	"container/heap"
	cryptorand "crypto/rand"
	"encoding/binary"
	"errors"
	"math"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/limyedb/limyedb/internal/pool"
	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/distance"
	"github.com/limyedb/limyedb/pkg/point"
	"github.com/limyedb/limyedb/pkg/quantization"
	"github.com/limyedb/limyedb/pkg/storage/mmap"
)

// secureRandomSeed generates a cryptographically secure random seed
func secureRandomSeed() int64 {
	var seed int64
	if err := binary.Read(cryptorand.Reader, binary.LittleEndian, &seed); err != nil {
		// Fallback to time-based seed if crypto/rand fails
		return rand.Int63() // #nosec G404 - fallback only, primary uses crypto/rand
	}
	return seed
}

// MaxNodes is the maximum number of nodes supported (limited by uint32)
const MaxNodes = math.MaxUint32

// ErrTooManyNodes is returned when the index exceeds the maximum number of nodes
var ErrTooManyNodes = errors.New("maximum number of nodes exceeded")

// HNSW implements the Hierarchical Navigable Small World graph
type HNSW struct {
	// Configuration
	M              int     // Max connections per node
	Mmax           int     // Max connections at layer 0 (typically 2*M)
	Mmax0          int     // Alias for Mmax
	efConstruction int     // Build-time search quality
	efSearch       int     // Query-time search quality
	ml             float64 // Level generation factor (1/ln(M))
	maxLevel       int     // Current maximum level

	// Index data
	nodes       []*Node
	entryPoint  uint32
	nodeCount   atomic.Int64
	dimension   int
	idToIndex   map[string]uint32
	deletedCount atomic.Int64

	// Distance calculator
	distCalc distance.Calculator

	// Concurrency control
	mu        sync.RWMutex
	buildLock sync.Mutex

	// Random number generator
	rng *rand.Rand
	rngMu sync.Mutex

	// Object pools to completely eliminate GC stalls on search path
	visitedPool   *pool.VisitedListPool
	candidatePool *pool.CandidatePool

	quantizer     quantization.Quantizer
	graphMmap     *mmap.GraphMmap // On-disk graph pager
	vectorMmap    *mmap.Storage   // On-disk vectors pager
}

// Config holds HNSW configuration
type Config struct {
	M              int
	EfConstruction int
	EfSearch       int
	MaxElements    int
	Metric         config.MetricType
	Dimension      int
	Quantizer      quantization.Quantizer
	GraphMmap      *mmap.GraphMmap
	VectorMmap     *mmap.Storage
}

// DefaultConfig returns default HNSW configuration
func DefaultConfig() *Config {
	return &Config{
		M:              16,
		EfConstruction: 200,
		EfSearch:       100,
		MaxElements:    100000,
		Metric:         config.MetricCosine,
		Dimension:      0, // Must be set
	}
}

// New creates a new HNSW index
func New(cfg *Config) (*HNSW, error) {
	if cfg.M < 2 {
		return nil, errors.New("parameter M must be at least 2") //nolint:stylecheck // M is a proper noun (HNSW parameter)
	}
	if cfg.Dimension <= 0 {
		return nil, errors.New("dimension must be positive")
	}

	h := &HNSW{
		M:              cfg.M,
		Mmax:           2 * cfg.M,
		Mmax0:          2 * cfg.M,
		efConstruction: cfg.EfConstruction,
		efSearch:       cfg.EfSearch,
		ml:             1.0 / math.Log(float64(cfg.M)),
		maxLevel:       -1,
		nodes:          make([]*Node, 0, cfg.MaxElements),
		idToIndex:      make(map[string]uint32),
		dimension:      cfg.Dimension,
		distCalc:       distance.New(cfg.Metric),
		quantizer:      cfg.Quantizer,
		graphMmap:      cfg.GraphMmap,
		vectorMmap:     cfg.VectorMmap,
		rng:            rand.New(rand.NewSource(secureRandomSeed())), // #nosec G404 - uses crypto seed, math/rand is fine for HNSW
		visitedPool:    pool.NewVisitedListPool(cfg.MaxElements),
		candidatePool:  pool.NewCandidatePool(1000),
	}

	return h, nil
}

// Insert adds a new point to the index
func (h *HNSW) Insert(p *point.Point) error {
	if len(p.Vector) != h.dimension {
		return ErrDimensionMismatch
	}

	h.buildLock.Lock()
	defer h.buildLock.Unlock()

	// Check if ID already exists
	h.mu.RLock()
	if _, exists := h.idToIndex[p.ID]; exists {
		h.mu.RUnlock()
		return ErrPointExists
	}
	h.mu.RUnlock()

	// Generate random level
	level := h.randomLevel()

	// Create new node
	var offset int64
	var vec point.Vector = p.Vector

	if h.vectorMmap != nil {
		offset, _ = h.vectorMmap.Allocate()
		_ = h.vectorMmap.WriteVector(offset, p.Vector)
		vec = nil // Free RAM mapping
	}

	useMmap := h.graphMmap != nil
	node := NewNode(p.ID, vec, level, h.M, useMmap)
	node.VectorOffset = offset

	// Pre-compute quantized vector if enabled
	if h.quantizer != nil {
		if qData, err := h.quantizer.Encode(p.Vector); err == nil {
			node.Quantized = qData
		}
	}
	node.SetPayload(p.Payload)

	h.mu.Lock()
	if len(h.nodes) >= MaxNodes {
		h.mu.Unlock()
		return ErrTooManyNodes
	}
	nodeID := uint32(len(h.nodes)) // #nosec G115 - bounds checked above

	if h.graphMmap != nil {
		if err := h.graphMmap.AddNode(nodeID, level); err != nil {
			h.mu.Unlock()
			return err
		}
	}

	h.nodes = append(h.nodes, node)
	h.idToIndex[p.ID] = nodeID
	h.nodeCount.Add(1)

	// Handle first node
	if h.maxLevel == -1 {
		h.maxLevel = level
		h.entryPoint = nodeID
		h.mu.Unlock()
		return nil
	}

	entryPoint := h.entryPoint
	currentMaxLevel := h.maxLevel

	// Update entry point if new node has higher level
	if level > currentMaxLevel {
		h.maxLevel = level
		h.entryPoint = nodeID
	}
	h.mu.Unlock()

	// Find entry point by traversing from top to bottom
	currNode := entryPoint
	for l := currentMaxLevel; l > level; l-- {
		currNode = h.greedySearchLayer(p.Vector, currNode, l)
	}

	// Insert at each layer
	for l := min(level, currentMaxLevel); l >= 0; l-- {
		neighbors := h.searchLayer(p.Vector, currNode, h.efConstruction, l)
		h.selectNeighbors(nodeID, neighbors, l)
		currNode = neighbors[0].ID
	}

	return nil
}

// randomLevel generates a random level for a new node
func (h *HNSW) randomLevel() int {
	h.rngMu.Lock()
	r := h.rng.Float64()
	h.rngMu.Unlock()

	return int(-math.Log(r) * h.ml)
}

// greedySearchLayer performs greedy search within a single layer
func (h *HNSW) greedySearchLayer(query point.Vector, entryID uint32, layer int) uint32 {
	h.mu.RLock()
	defer h.mu.RUnlock()

	currentID := entryID
	var currentDist float32

	if h.quantizer != nil && h.nodes[currentID].Quantized != nil {
		currentDist = h.quantizer.Distance(query, h.nodes[currentID].Quantized)
	} else {
		currentDist = h.distCalc.Distance(query, h.getVector(currentID))
	}

	for {
		changed := false
		neighbors := h.getConnections(currentID, layer)

		for _, neighborID := range neighbors {
			if h.nodes[neighborID].IsDeleted() {
				continue
			}
			
			var dist float32
			if h.quantizer != nil && h.nodes[neighborID].Quantized != nil {
				dist = h.quantizer.Distance(query, h.nodes[neighborID].Quantized)
			} else {
				dist = h.distCalc.Distance(query, h.getVector(neighborID))
			}
			
			if dist < currentDist {
				currentID = neighborID
				currentDist = dist
				changed = true
			}
		}

		if !changed {
			break
		}
	}

	return currentID
}

// searchLayer searches for ef nearest neighbors within a layer
func (h *HNSW) searchLayer(query point.Vector, entryID uint32, ef int, layer int) []Candidate {
	h.mu.RLock()
	defer h.mu.RUnlock()

	visited := h.visitedPool.Get()
	defer h.visitedPool.Put(visited)
	visited.Add(entryID)

	// Candidates to explore (min-heap)
	candidates := &CandidateHeap{}
	heap.Init(candidates)
	// Best results so far (max-heap for easy worst-case access)
	results := &MaxCandidateHeap{}
	heap.Init(results)

	var entryDist float32
	if h.quantizer != nil && h.nodes[entryID].Quantized != nil {
		entryDist = h.quantizer.Distance(query, h.nodes[entryID].Quantized)
	} else {
		entryDist = h.distCalc.Distance(query, h.getVector(entryID))
	}

	heap.Push(candidates, Candidate{ID: entryID, Distance: entryDist})
	heap.Push(results, Candidate{ID: entryID, Distance: entryDist})

	for candidates.Len() > 0 {
		// Get closest unexplored candidate
		curr := heap.Pop(candidates).(Candidate)

		// Stop if this candidate is worse than the worst result
		if results.Len() >= ef && curr.Distance > (*results)[0].Distance {
			break
		}

		// Explore neighbors
		neighbors := h.getConnections(curr.ID, layer)
		for _, neighborID := range neighbors {
			if visited.Add(neighborID) {
				continue // Already visited
			}

			if h.nodes[neighborID].IsDeleted() {
				continue
			}
			
			var dist float32
			if h.quantizer != nil && h.nodes[neighborID].Quantized != nil {
				dist = h.quantizer.Distance(query, h.nodes[neighborID].Quantized)
			} else {
				dist = h.distCalc.Distance(query, h.getVector(neighborID))
			}

			// Add to results if better than worst or we don't have enough
			if results.Len() < ef || dist < (*results)[0].Distance {
				heap.Push(candidates, Candidate{ID: neighborID, Distance: dist})
				heap.Push(results, Candidate{ID: neighborID, Distance: dist})

				// Keep only ef best results
				if results.Len() > ef {
					heap.Pop(results)
				}
			}
		}
	}

	// Convert to slice
	result := make([]Candidate, results.Len())
	for i := len(result) - 1; i >= 0; i-- {
		result[i] = heap.Pop(results).(Candidate)
	}
	return result
}

// getConnections is a unified connection getter backing RAM or MMap
func (h *HNSW) getConnections(nodeID uint32, layer int) []uint32 {
	if h.graphMmap != nil {
		return h.graphMmap.GetConnections(nodeID, layer)
	}
	return h.nodes[nodeID].GetConnections(layer)
}

// setConnections is a unified connection setter backing RAM or MMap
func (h *HNSW) setConnections(nodeID uint32, layer int, connections []uint32) {
	if h.graphMmap != nil {
		h.graphMmap.SetConnections(nodeID, layer, connections)
	} else {
		h.nodes[nodeID].SetConnections(layer, connections)
	}
}

// selectNeighbors selects the best neighbors for a node
func (h *HNSW) selectNeighbors(nodeID uint32, candidates []Candidate, layer int) {
	h.mu.Lock()
	defer h.mu.Unlock()

	maxConn := h.M
	if layer == 0 {
		maxConn = h.Mmax0
	}

	// Simple heuristic: connect to closest ones
	var selected []Candidate
	if len(candidates) > maxConn {
		// Sort by distance
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].Distance < candidates[j].Distance
		})
		selected = candidates[:maxConn]
	} else {
		selected = candidates
	}

	connections := make([]uint32, len(selected))
	for i, c := range selected {
		connections[i] = c.ID
	}
	h.setConnections(nodeID, layer, connections)

	// Add reverse connections
	for _, c := range selected {
		h.addConnection(c.ID, nodeID, layer, maxConn)
	}
}

// addConnection adds a bidirectional connection
func (h *HNSW) addConnection(fromID, toID uint32, layer, maxConn int) {
	// Check if already connected
	conns := h.getConnections(fromID, layer)
	for _, c := range conns {
		if c == toID {
			return
		}
	}

	// Add connection
	if len(conns) < maxConn {
		// Optimization: if using MMap, we just append to the slice and set it
		newConns := append(conns, toID)
		h.setConnections(fromID, layer, newConns)
		return
	}

	// Need to prune: find worst connection and replace if new is better
	// Simple optimization: select closest nodes
	query := h.getVector(toID)
	newDist := h.distCalc.Distance(h.getVector(fromID), query)

	worstIdx := -1
	worstDist := newDist
	for i, connID := range conns {
		dist := h.distCalc.Distance(h.getVector(fromID), h.getVector(connID))
		if dist > worstDist {
			worstDist = dist
			worstIdx = i
		}
	}

	if worstIdx >= 0 {
		// Replace worst connection
		conns[worstIdx] = conns[len(conns)-1]
		conns = conns[:len(conns)-1]
		conns = append(conns, toID)
		h.setConnections(fromID, layer, conns)
	}
}

// Search performs k-NN search
func (h *HNSW) Search(query point.Vector, k int) ([]Candidate, error) {
	return h.SearchWithEf(query, k, h.efSearch)
}

// SearchWithEf performs k-NN search with custom ef parameter
func (h *HNSW) SearchWithEf(query point.Vector, k int, ef int) ([]Candidate, error) {
	if len(query) != h.dimension {
		return nil, ErrDimensionMismatch
	}

	h.mu.RLock()
	if h.maxLevel == -1 {
		h.mu.RUnlock()
		return nil, nil // Empty index
	}
	entryPoint := h.entryPoint
	maxLevel := h.maxLevel
	h.mu.RUnlock()

	if ef < k {
		ef = k
	}

	searchEf := ef
	if h.quantizer != nil {
		// Over-fetch by 2x for Stage 2 Rescoring
		searchEf = ef * 2
	}

	// Traverse from top to bottom
	currNode := entryPoint
	for l := maxLevel; l > 0; l-- {
		currNode = h.greedySearchLayer(query, currNode, l)
	}

	// Search at layer 0 with ef candidates
	candidates := h.searchLayer(query, currNode, searchEf, 0)

	// Stage 2: Rescoring for Quantized precision
	if h.quantizer != nil {
		h.mu.RLock() // Lock for node retrieval
		for i := range candidates {
			exactDist := h.distCalc.Distance(query, h.getVector(candidates[i].ID))
			candidates[i].Distance = exactDist
		}
		h.mu.RUnlock()

		// Re-sort candidates purely on exact distances
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].Distance < candidates[j].Distance
		})
	}

	// Return top k
	if len(candidates) > k {
		candidates = candidates[:k]
	}

	return candidates, nil
}

// Delete marks a point as deleted (lazy deletion)
func (h *HNSW) Delete(id string) error {
	h.mu.RLock()
	nodeID, exists := h.idToIndex[id]
	h.mu.RUnlock()

	if !exists {
		return ErrPointNotFound
	}

	h.nodes[nodeID].MarkDeleted()
	h.deletedCount.Add(1)

	return nil
}

// Get retrieves a point by ID
func (h *HNSW) Get(id string) (*point.Point, error) {
	h.mu.RLock()
	nodeID, exists := h.idToIndex[id]
	h.mu.RUnlock()

	if !exists {
		return nil, ErrPointNotFound
	}

	node := h.nodes[nodeID]
	if node.IsDeleted() {
		return nil, ErrPointNotFound
	}

	return &point.Point{
		ID:      node.ID,
		Vector:  h.getVector(nodeID),
		Payload: node.GetPayload(),
	}, nil
}

// Size returns the number of points (excluding deleted)
func (h *HNSW) Size() int64 {
	return h.nodeCount.Load() - h.deletedCount.Load()
}

// TotalSize returns total number of points including deleted
func (h *HNSW) TotalSize() int64 {
	return h.nodeCount.Load()
}

// SetEfSearch sets the search quality parameter
func (h *HNSW) SetEfSearch(ef int) {
	h.efSearch = ef
}

// GetNodeID returns the internal node ID for a point ID
func (h *HNSW) GetNodeID(pointID string) (uint32, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	nodeID, exists := h.idToIndex[pointID]
	return nodeID, exists
}

// GetPointID returns the point ID for an internal node ID
func (h *HNSW) GetPointID(nodeID uint32) string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if int(nodeID) >= len(h.nodes) {
		return ""
	}
	return h.nodes[nodeID].ID
}

// IteratePoints applies a function to all points in the index (used for linear scanning)
func (h *HNSW) IteratePoints(cb func(*point.Point) bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, n := range h.nodes {
		if n != nil && !n.IsDeleted() {
			p, err := h.Get(n.ID)
			if err == nil && p != nil {
				if !cb(p) {
					break
				}
			}
		}
	}
}

// Iterate iterates over all non-deleted point IDs
func (h *HNSW) Iterate(fn func(id string) error) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, node := range h.nodes {
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
func (h *HNSW) GetAllPoints() []*point.Point {
	h.mu.RLock()
	defer h.mu.RUnlock()

	points := make([]*point.Point, 0, len(h.nodes))
	for i, node := range h.nodes {
		if node.IsDeleted() {
			continue
		}
		points = append(points, &point.Point{
			ID:      node.ID,
			Vector:  h.getVector(uint32(i)),
			Payload: node.GetPayload(),
		})
	}
	return points
}

// getVector extracts the node vector from either RAM or Mmap Storage dynamically 
func (h *HNSW) getVector(id uint32) point.Vector {
	node := h.nodes[id]
	if node.Vector != nil {
		return node.Vector
	}
	if h.vectorMmap != nil {
		// Reads from the dynamic OS virtual page securely mapped over RAM
		vec, err := h.vectorMmap.ReadVector(node.VectorOffset)
		if err == nil {
			return vec
		}
	}
	return nil
}

// Errors
var (
	ErrDimensionMismatch = errors.New("vector dimension mismatch")
	ErrPointExists       = errors.New("point with this ID already exists")
	ErrPointNotFound     = errors.New("point not found")
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
