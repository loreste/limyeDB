package hnsw

import (
	"sync"
	"sync/atomic"

	"github.com/limyedb/limyedb/pkg/point"
)

// Node represents a node in the HNSW graph
type Node struct {
	ID          string
	Vector      point.Vector
	Quantized   []byte
	Level       int
	Connections [][]uint32 // Connections per layer
	mu          sync.RWMutex
	deleted     atomic.Bool
	payload     map[string]interface{}
}

// NewNode creates a new HNSW node
func NewNode(id string, vector point.Vector, level int, m int, useMmap bool) *Node {
	var connections [][]uint32
	if !useMmap {
		connections = make([][]uint32, level+1)
		for i := 0; i <= level; i++ {
			// Layer 0 has 2*M connections, upper layers have M
			capacity := m
			if i == 0 {
				capacity = 2 * m
			}
			connections[i] = make([]uint32, 0, capacity)
		}
	}

	return &Node{
		ID:          id,
		Vector:      vector,
		Level:       level,
		Connections: connections,
	}
}

// GetConnections returns connections at a specific layer (thread-safe)
func (n *Node) GetConnections(layer int) []uint32 {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if layer > n.Level || layer < 0 {
		return nil
	}

	// Return a copy to avoid race conditions
	result := make([]uint32, len(n.Connections[layer]))
	copy(result, n.Connections[layer])
	return result
}

// SetConnections sets connections at a specific layer (thread-safe)
func (n *Node) SetConnections(layer int, connections []uint32) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if layer > n.Level || layer < 0 {
		return
	}

	n.Connections[layer] = make([]uint32, len(connections))
	copy(n.Connections[layer], connections)
}

// AddConnection adds a connection at a specific layer
func (n *Node) AddConnection(layer int, nodeID uint32) bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	if layer > n.Level || layer < 0 {
		return false
	}

	// Check if connection already exists
	for _, conn := range n.Connections[layer] {
		if conn == nodeID {
			return false
		}
	}

	n.Connections[layer] = append(n.Connections[layer], nodeID)
	return true
}

// RemoveConnection removes a connection at a specific layer
func (n *Node) RemoveConnection(layer int, nodeID uint32) bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	if layer > n.Level || layer < 0 {
		return false
	}

	for i, conn := range n.Connections[layer] {
		if conn == nodeID {
			// Remove by swapping with last element
			n.Connections[layer][i] = n.Connections[layer][len(n.Connections[layer])-1]
			n.Connections[layer] = n.Connections[layer][:len(n.Connections[layer])-1]
			return true
		}
	}
	return false
}

// ConnectionCount returns the number of connections at a layer
func (n *Node) ConnectionCount(layer int) int {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if layer > n.Level || layer < 0 {
		return 0
	}
	return len(n.Connections[layer])
}

// IsDeleted returns whether the node is marked as deleted
func (n *Node) IsDeleted() bool {
	return n.deleted.Load()
}

// MarkDeleted marks the node as deleted
func (n *Node) MarkDeleted() {
	n.deleted.Store(true)
}

// SetPayload sets the node's payload
func (n *Node) SetPayload(payload map[string]interface{}) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.payload = payload
}

// GetPayload returns the node's payload
func (n *Node) GetPayload() map[string]interface{} {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.payload == nil {
		return nil
	}

	// Return a copy
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
	*h = append(*h, x.(Candidate))
}

func (h *CandidateHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// Top returns the smallest element without removing it
func (h CandidateHeap) Top() Candidate {
	return h[0]
}

// MaxCandidateHeap is a max-heap of candidates (sorted by distance, descending)
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

// Top returns the largest element without removing it
func (h MaxCandidateHeap) Top() Candidate {
	return h[0]
}

// VisitedSet is a thread-safe set for tracking visited nodes
type VisitedSet struct {
	visited map[uint32]struct{}
	mu      sync.RWMutex
}

// NewVisitedSet creates a new visited set
func NewVisitedSet() *VisitedSet {
	return &VisitedSet{
		visited: make(map[uint32]struct{}),
	}
}

// Add adds a node to the visited set, returns true if already visited
func (v *VisitedSet) Add(id uint32) bool {
	v.mu.Lock()
	defer v.mu.Unlock()

	if _, exists := v.visited[id]; exists {
		return true
	}
	v.visited[id] = struct{}{}
	return false
}

// Contains checks if a node has been visited
func (v *VisitedSet) Contains(id uint32) bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	_, exists := v.visited[id]
	return exists
}

// Reset clears the visited set
func (v *VisitedSet) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.visited = make(map[uint32]struct{})
}

// Size returns the number of visited nodes
func (v *VisitedSet) Size() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return len(v.visited)
}

// BitSet is a memory-efficient visited set using bit manipulation
type BitSet struct {
	bits []uint64
	size int
}

// NewBitSet creates a new bit set with the given capacity
func NewBitSet(size int) *BitSet {
	numWords := (size + 63) / 64
	return &BitSet{
		bits: make([]uint64, numWords),
		size: size,
	}
}

// Set sets a bit at the given index
func (b *BitSet) Set(index int) {
	if index < 0 || index >= b.size {
		return
	}
	wordIndex := index / 64
	bitIndex := uint(index % 64)
	b.bits[wordIndex] |= 1 << bitIndex
}

// Test checks if a bit is set
func (b *BitSet) Test(index int) bool {
	if index < 0 || index >= b.size {
		return false
	}
	wordIndex := index / 64
	bitIndex := uint(index % 64)
	return (b.bits[wordIndex] & (1 << bitIndex)) != 0
}

// Clear clears all bits
func (b *BitSet) Clear() {
	for i := range b.bits {
		b.bits[i] = 0
	}
}
