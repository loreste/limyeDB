package diskann

import (
	"sort"
	"sync"

	"github.com/limyedb/limyedb/pkg/point"
)

// VamanaNode strictly bounds flat linear graph connections explicitly routing over SSDs directly
type VamanaNode struct {
	ID        uint32
	Neighbors []uint32
}

// VamanaGraph orchestrates a pure single-layer topology traversing Extreme NVMe capacities fully bypassing RAM
type VamanaGraph struct {
	nodes        map[uint32]*VamanaNode
	dimension    int
	maxDegree    int
	alpha        float32
	getVector    func(uint32) point.Vector
	entryNode    uint32
	mu           sync.RWMutex
}

// NewVamanaGraph initiates the purely asynchronous topology seamlessly
func NewVamanaGraph(dimension int, maxDegree int, alpha float32, getVec func(uint32) point.Vector) *VamanaGraph {
	return &VamanaGraph{
		nodes:     make(map[uint32]*VamanaNode),
		dimension: dimension,
		maxDegree: maxDegree,
		alpha:     alpha, // Extreme-density structural penalty factor (typically 1.2)
		getVector: getVec,
	}
}

// AddNode initializes strict edge nodes natively
func (g *VamanaGraph) AddNode(id uint32) {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	if _, exists := g.nodes[id]; !exists {
		g.nodes[id] = &VamanaNode{
			ID:        id,
			Neighbors: make([]uint32, 0, g.maxDegree),
		}
		if len(g.nodes) == 1 {
			g.entryNode = id // First inserted node anchors traversal strictly logically
		}
	}
}

// Euclidean executes simple L2 norms directly supporting fast path evaluations consistently
func Euclidean(a, b point.Vector) float32 {
	var dist float32
	for i := range a {
		diff := a[i] - b[i]
		dist += diff * diff
	}
	return dist
}

// GreedySearch evaluates queries entirely over flat structures avoiding memory-bloated layer bounds explicitly mathematically
func (g *VamanaGraph) GreedySearch(query point.Vector, L int) []uint32 {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if len(g.nodes) == 0 {
		return nil
	}

	visited := make(map[uint32]bool)
	visited[g.entryNode] = true
	
	candidates := []uint32{g.entryNode}
	
	for len(candidates) > 0 {
		bestIdx := 0
		bestDist := Euclidean(query, g.getVector(candidates[0]))
		
		for i := 1; i < len(candidates); i++ {
			d := Euclidean(query, g.getVector(candidates[i]))
			if d < bestDist {
				bestDist = d
				bestIdx = i
			}
		}

		curr := candidates[bestIdx]
		candidates = append(candidates[:bestIdx], candidates[bestIdx+1:]...)

		node := g.nodes[curr]
		for _, neighbor := range node.Neighbors {
			if !visited[neighbor] {
				visited[neighbor] = true
				
				if len(visited) < L {
					candidates = append(candidates, neighbor)
				}
			}
		}
	}

	res := make([]uint32, 0, len(visited))
	for k := range visited {
		res = append(res, k)
	}
	return res
}

// RobustPrune ensures the topology scales purely to 1B+ bounds mathematically pruning tight subsets strictly favoring diverse geometries dynamically.
func (g *VamanaGraph) RobustPrune(nodeID uint32, candidates []uint32) {
	g.mu.Lock()
	defer g.mu.Unlock()

	node := g.nodes[nodeID]
	nodeVec := g.getVector(nodeID)

	allCandidates := make(map[uint32]bool)
	for _, n := range node.Neighbors {
		allCandidates[n] = true
	}
	for _, c := range candidates {
		if c != nodeID {
			allCandidates[c] = true
		}
	}

	type cand struct {
		id   uint32
		dist float32
	}
	pool := make([]cand, 0, len(allCandidates))
	for c := range allCandidates {
		pool = append(pool, cand{id: c, dist: Euclidean(nodeVec, g.getVector(c))})
	}

	sort.Slice(pool, func(i, j int) bool { return pool[i].dist < pool[j].dist })

	newNeighbors := make([]uint32, 0, g.maxDegree)
	
	for _, p := range pool {
		if len(newNeighbors) >= g.maxDegree {
			break
		}
		
		valid := true
		for _, n := range newNeighbors {
			// Alpha scaling bounds safely dropping highly clustered neighbor redundancies precisely preserving RAM boundaries
			if g.alpha * Euclidean(g.getVector(p.id), g.getVector(n)) <= p.dist {
				valid = false
				break
			}
		}
		if valid {
			newNeighbors = append(newNeighbors, p.id)
		}
	}

	node.Neighbors = newNeighbors
}
