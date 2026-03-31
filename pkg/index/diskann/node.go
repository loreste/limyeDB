package diskann

import (
	"sync"
	"sync/atomic"

	"github.com/limyedb/limyedb/pkg/point"
)

// Node represents a node in the DiskANN graph
type Node struct {
	// ID is the user-provided point identifier
	ID string

	// Vector is the point's vector (in RAM)
	Vector point.Vector

	// VectorOffset is the offset for disk-stored vectors
	VectorOffset int64

	// Neighbors stores the node's outgoing edges
	Neighbors []uint32

	// payload stores metadata associated with this point
	payload map[string]interface{}

	// deleted marks the node as logically deleted
	deleted atomic.Bool

	// mu protects concurrent access to mutable fields
	mu sync.RWMutex
}

// NewNode creates a new DiskANN node
func NewNode(id string, vector point.Vector, maxDegree int) *Node {
	return &Node{
		ID:        id,
		Vector:    vector,
		Neighbors: make([]uint32, 0, maxDegree),
	}
}

// GetNeighbors returns a copy of the node's neighbors (thread-safe)
func (n *Node) GetNeighbors() []uint32 {
	n.mu.RLock()
	defer n.mu.RUnlock()

	result := make([]uint32, len(n.Neighbors))
	copy(result, n.Neighbors)
	return result
}

// SetNeighbors sets the node's neighbors (thread-safe)
func (n *Node) SetNeighbors(neighbors []uint32) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.Neighbors = make([]uint32, len(neighbors))
	copy(n.Neighbors, neighbors)
}

// AddNeighbor adds a neighbor if not already present
func (n *Node) AddNeighbor(nodeID uint32, maxDegree int) bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Check if already connected
	for _, neighbor := range n.Neighbors {
		if neighbor == nodeID {
			return false
		}
	}

	// Add if under max degree
	if len(n.Neighbors) < maxDegree {
		n.Neighbors = append(n.Neighbors, nodeID)
		return true
	}
	return false
}

// RemoveNeighbor removes a neighbor from the node
func (n *Node) RemoveNeighbor(nodeID uint32) bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	for i, neighbor := range n.Neighbors {
		if neighbor == nodeID {
			// Swap with last element and truncate
			n.Neighbors[i] = n.Neighbors[len(n.Neighbors)-1]
			n.Neighbors = n.Neighbors[:len(n.Neighbors)-1]
			return true
		}
	}
	return false
}

// Degree returns the current number of neighbors
func (n *Node) Degree() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.Neighbors)
}

// IsDeleted returns whether the node is marked as deleted
func (n *Node) IsDeleted() bool {
	return n.deleted.Load()
}

// MarkDeleted marks the node as deleted
func (n *Node) MarkDeleted() {
	n.deleted.Store(true)
}

// Undelete unmarks the node as deleted
func (n *Node) Undelete() {
	n.deleted.Store(false)
}

// SetPayload sets the node's payload
func (n *Node) SetPayload(payload map[string]interface{}) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.payload = payload
}

// GetPayload returns a copy of the node's payload
func (n *Node) GetPayload() map[string]interface{} {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.payload == nil {
		return nil
	}

	result := make(map[string]interface{})
	for k, v := range n.payload {
		result[k] = v
	}
	return result
}

// Candidate represents a search candidate with distance
type Candidate struct {
	ID       uint32
	Distance float32
}

// CandidateHeap is a min-heap of candidates (sorted by distance, ascending)
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

// MaxCandidateHeap is a max-heap of candidates (sorted by distance, descending)
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
