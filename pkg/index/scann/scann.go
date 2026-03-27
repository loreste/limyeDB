package scann

import (
	"errors"
	"math"
	"sync"
	"sync/atomic"

	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/distance"
	"github.com/limyedb/limyedb/pkg/point"
)

// Config holds ScaNN index configuration
type Config struct {
	NumLeaves           int               // Number of partitions/leaves (default: sqrt(n))
	NumRerank           int               // Candidates for exact reranking (default: 100)
	Dimension           int               // Vector dimension
	Metric              config.MetricType // Distance metric
	QuantizationDims    int               // Reduced dimensions for AQ (default: dimension/4)
	AnisotropicThreshold float32          // Threshold for anisotropic quantization
	MaxElements         int               // Maximum number of elements
	TrainingSamples     int               // Samples for training
}

// DefaultConfig returns default ScaNN configuration
func DefaultConfig() *Config {
	return &Config{
		NumLeaves:           100,
		NumRerank:           100,
		Dimension:           0, // Must be set
		Metric:              config.MetricCosine,
		QuantizationDims:    0, // Auto-compute
		AnisotropicThreshold: 0.2,
		MaxElements:         100000,
		TrainingSamples:     10000,
	}
}

// ScaNN implements the Scalable Nearest Neighbors algorithm
type ScaNN struct {
	config       *Config
	partitioner  *TreePartitioner
	aqEncoder    *AnisotropicQuantizer
	distCalc     distance.Calculator

	// Point storage
	points       []*point.Point
	quantized    [][]byte        // Quantized representations
	idToIndex    map[string]uint32
	pointCount   atomic.Int64
	deletedCount atomic.Int64

	// Training state
	trainingBuffer []point.Vector
	trained        bool

	// Concurrency control
	mu        sync.RWMutex
	buildLock sync.Mutex
}

// New creates a new ScaNN index
func New(cfg *Config) (*ScaNN, error) {
	if cfg.Dimension <= 0 {
		return nil, errors.New("dimension must be positive")
	}
	if cfg.NumLeaves <= 0 {
		cfg.NumLeaves = 100
	}
	if cfg.NumRerank <= 0 {
		cfg.NumRerank = 100
	}
	if cfg.QuantizationDims <= 0 {
		cfg.QuantizationDims = cfg.Dimension / 4
		if cfg.QuantizationDims < 8 {
			cfg.QuantizationDims = 8
		}
	}

	distCalc := distance.New(cfg.Metric)

	// Initialize anisotropic quantizer
	aqConfig := &AnisotropicConfig{
		Dimension:        cfg.Dimension,
		QuantizationDims: cfg.QuantizationDims,
		Threshold:        cfg.AnisotropicThreshold,
		NumCodes:         256,
	}

	scann := &ScaNN{
		config:         cfg,
		aqEncoder:      NewAnisotropicQuantizer(aqConfig),
		distCalc:       distCalc,
		points:         make([]*point.Point, 0, cfg.MaxElements),
		quantized:      make([][]byte, 0, cfg.MaxElements),
		idToIndex:      make(map[string]uint32),
		trainingBuffer: make([]point.Vector, 0, cfg.TrainingSamples),
	}

	return scann, nil
}

// Insert adds a point to the index
func (s *ScaNN) Insert(p *point.Point) error {
	if len(p.Vector) != s.config.Dimension {
		return ErrDimensionMismatch
	}

	s.buildLock.Lock()
	defer s.buildLock.Unlock()

	// Check if ID already exists
	s.mu.RLock()
	if _, exists := s.idToIndex[p.ID]; exists {
		s.mu.RUnlock()
		return ErrPointExists
	}
	s.mu.RUnlock()

	// If not trained, add to training buffer
	if !s.trained {
		s.mu.Lock()
		s.trainingBuffer = append(s.trainingBuffer, copyVector(p.Vector))

		// Auto-train when buffer is full
		if len(s.trainingBuffer) >= s.config.TrainingSamples {
			s.mu.Unlock()
			if err := s.Train(s.trainingBuffer); err != nil {
				return err
			}
			// Re-insert buffered vectors
			s.mu.Lock()
			buffer := s.trainingBuffer
			s.trainingBuffer = nil
			s.mu.Unlock()

			for i, vec := range buffer {
				tempPoint := &point.Point{
					ID:     p.ID + "_temp_" + string(rune(i)),
					Vector: vec,
				}
				if err := s.insertTrained(tempPoint); err != nil {
					return err
				}
			}
			return s.insertTrained(p)
		}
		s.mu.Unlock()

		// Store point before training
		return s.storePoint(p, nil)
	}

	return s.insertTrained(p)
}

// insertTrained inserts a point into the trained index
func (s *ScaNN) insertTrained(p *point.Point) error {
	// Quantize the vector
	quantized, err := s.aqEncoder.Encode(p.Vector)
	if err != nil {
		// Store without quantization if encoding fails
		return s.storePoint(p, nil)
	}

	return s.storePoint(p, quantized)
}

// storePoint stores a point
func (s *ScaNN) storePoint(p *point.Point, quantized []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check again under write lock
	if _, exists := s.idToIndex[p.ID]; exists {
		return ErrPointExists
	}

	// Store the point with safe integer conversion
	n := len(s.points)
	if n < 0 || n > math.MaxUint32 {
		return errors.New("point count exceeds uint32 range")
	}
	pointID := uint32(n)
	s.points = append(s.points, p)
	s.quantized = append(s.quantized, quantized)
	s.idToIndex[p.ID] = pointID
	s.pointCount.Add(1)

	// Add to partition tree if trained
	if s.trained && s.partitioner != nil {
		s.partitioner.Add(pointID, p.Vector)
	}

	return nil
}

// Train trains the ScaNN index
func (s *ScaNN) Train(vectors []point.Vector) error {
	if len(vectors) == 0 {
		return errors.New("no vectors to train on")
	}

	// Adjust number of leaves if needed
	numLeaves := s.config.NumLeaves
	if numLeaves > len(vectors) {
		numLeaves = int(math.Sqrt(float64(len(vectors))))
		if numLeaves < 1 {
			numLeaves = 1
		}
	}

	// Train the anisotropic quantizer
	if err := s.aqEncoder.Train(vectors); err != nil {
		return err
	}

	// Create partitioner
	s.partitioner = NewTreePartitioner(s.config.Dimension, numLeaves, s.distCalc)
	if err := s.partitioner.Train(vectors); err != nil {
		return err
	}

	s.mu.Lock()
	s.trained = true

	// Quantize all existing points
	for i, p := range s.points {
		if s.quantized[i] == nil {
			quantized, err := s.aqEncoder.Encode(p.Vector)
			if err == nil {
				s.quantized[i] = quantized
			}
		}

		// Add to partitioner
		s.partitioner.Add(uint32(i), p.Vector)
	}
	s.mu.Unlock()

	return nil
}

// Delete marks a point as deleted
func (s *ScaNN) Delete(id string) error {
	s.mu.RLock()
	_, exists := s.idToIndex[id]
	s.mu.RUnlock()

	if !exists {
		return ErrPointNotFound
	}

	s.deletedCount.Add(1)
	return nil
}

// Get retrieves a point by ID
func (s *ScaNN) Get(id string) (*point.Point, error) {
	s.mu.RLock()
	idx, exists := s.idToIndex[id]
	s.mu.RUnlock()

	if !exists {
		return nil, ErrPointNotFound
	}

	return s.points[idx], nil
}

// Size returns the number of points (excluding deleted)
func (s *ScaNN) Size() int64 {
	return s.pointCount.Load() - s.deletedCount.Load()
}

// TotalSize returns total number of points
func (s *ScaNN) TotalSize() int64 {
	return s.pointCount.Load()
}

// IsTrained returns whether the index has been trained
func (s *ScaNN) IsTrained() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.trained
}

// GetNodeID returns the internal node ID for a point ID
func (s *ScaNN) GetNodeID(pointID string) (uint32, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	nodeID, exists := s.idToIndex[pointID]
	return nodeID, exists
}

// GetPointID returns the point ID for an internal node ID
func (s *ScaNN) GetPointID(nodeID uint32) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if int(nodeID) >= len(s.points) {
		return ""
	}
	return s.points[nodeID].ID
}

// Iterate iterates over all non-deleted point IDs
func (s *ScaNN) Iterate(fn func(id string) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, p := range s.points {
		if err := fn(p.ID); err != nil {
			return err
		}
	}
	return nil
}

// GetAllPoints returns all points
func (s *ScaNN) GetAllPoints() []*point.Point {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*point.Point, len(s.points))
	copy(result, s.points)
	return result
}

// SetNumRerank sets the number of candidates for reranking
func (s *ScaNN) SetNumRerank(numRerank int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if numRerank > 0 {
		s.config.NumRerank = numRerank
	}
}

// Errors
var (
	ErrDimensionMismatch = errors.New("vector dimension mismatch")
	ErrPointExists       = errors.New("point with this ID already exists")
	ErrPointNotFound     = errors.New("point not found")
	ErrNotTrained        = errors.New("index not trained")
)

// copyVector creates a copy of a vector
func copyVector(v point.Vector) point.Vector {
	c := make(point.Vector, len(v))
	copy(c, v)
	return c
}

// TreePartitioner implements tree-based space partitioning for ScaNN
type TreePartitioner struct {
	dimension  int
	numLeaves  int
	distCalc   distance.Calculator

	// Tree structure
	root       *treeNode
	leaves     []*leafNode

	trained    bool
	mu         sync.RWMutex
}

type treeNode struct {
	isLeaf      bool
	splitDim    int
	splitValue  float32
	left        *treeNode
	right       *treeNode
	leafIndex   int
}

type leafNode struct {
	index    int
	pointIDs []uint32
	centroid point.Vector
	mu       sync.RWMutex
}

// NewTreePartitioner creates a new tree partitioner
func NewTreePartitioner(dimension, numLeaves int, distCalc distance.Calculator) *TreePartitioner {
	if distCalc == nil {
		distCalc = distance.New(config.MetricCosine) // Default to cosine distance
	}
	return &TreePartitioner{
		dimension: dimension,
		numLeaves: numLeaves,
		distCalc:  distCalc,
		leaves:    make([]*leafNode, 0, numLeaves),
	}
}

// Train builds the tree structure
func (tp *TreePartitioner) Train(vectors []point.Vector) error {
	if len(vectors) == 0 {
		return errors.New("no vectors to train on")
	}

	tp.mu.Lock()
	defer tp.mu.Unlock()

	// Build tree recursively
	indices := make([]int, len(vectors))
	for i := range indices {
		indices[i] = i
	}

	tp.leaves = make([]*leafNode, 0, tp.numLeaves)
	tp.root = tp.buildTree(vectors, indices, 0)
	tp.trained = true

	return nil
}

// buildTree recursively builds the tree
func (tp *TreePartitioner) buildTree(vectors []point.Vector, indices []int, depth int) *treeNode {
	// Create leaf if we have few enough points or reached max depth
	if len(indices) <= len(vectors)/tp.numLeaves || len(tp.leaves) >= tp.numLeaves-1 {
		leafIdx := len(tp.leaves)
		leaf := &leafNode{
			index:    leafIdx,
			pointIDs: make([]uint32, 0),
			centroid: tp.computeCentroid(vectors, indices),
		}
		tp.leaves = append(tp.leaves, leaf)

		return &treeNode{
			isLeaf:    true,
			leafIndex: leafIdx,
		}
	}

	// Find best split dimension (max variance)
	splitDim := tp.findBestSplitDim(vectors, indices)

	// Find median value
	values := make([]float32, len(indices))
	for i, idx := range indices {
		values[i] = vectors[idx][splitDim]
	}
	splitValue := median(values)

	// Partition indices
	var leftIndices, rightIndices []int
	for _, idx := range indices {
		if vectors[idx][splitDim] <= splitValue {
			leftIndices = append(leftIndices, idx)
		} else {
			rightIndices = append(rightIndices, idx)
		}
	}

	// Handle edge case where all points go to one side
	if len(leftIndices) == 0 {
		leftIndices = rightIndices[:len(rightIndices)/2]
		rightIndices = rightIndices[len(rightIndices)/2:]
	} else if len(rightIndices) == 0 {
		rightIndices = leftIndices[len(leftIndices)/2:]
		leftIndices = leftIndices[:len(leftIndices)/2]
	}

	return &treeNode{
		isLeaf:     false,
		splitDim:   splitDim,
		splitValue: splitValue,
		left:       tp.buildTree(vectors, leftIndices, depth+1),
		right:      tp.buildTree(vectors, rightIndices, depth+1),
	}
}

// findBestSplitDim finds the dimension with maximum variance
func (tp *TreePartitioner) findBestSplitDim(vectors []point.Vector, indices []int) int {
	bestDim := 0
	maxVariance := float32(0)

	for d := 0; d < tp.dimension; d++ {
		// Compute variance
		var sum, sumSq float32
		for _, idx := range indices {
			val := vectors[idx][d]
			sum += val
			sumSq += val * val
		}
		n := float32(len(indices))
		mean := sum / n
		variance := sumSq/n - mean*mean

		if variance > maxVariance {
			maxVariance = variance
			bestDim = d
		}
	}

	return bestDim
}

// computeCentroid computes the centroid of vectors
func (tp *TreePartitioner) computeCentroid(vectors []point.Vector, indices []int) point.Vector {
	centroid := make(point.Vector, tp.dimension)

	for _, idx := range indices {
		for d := range centroid {
			centroid[d] += vectors[idx][d]
		}
	}

	n := float32(len(indices))
	for d := range centroid {
		centroid[d] /= n
	}

	return centroid
}

// Add adds a point to the appropriate partition
func (tp *TreePartitioner) Add(pointID uint32, vec point.Vector) {
	tp.mu.RLock()
	if !tp.trained {
		tp.mu.RUnlock()
		return
	}
	tp.mu.RUnlock()

	leafIdx := tp.findLeaf(vec)
	if leafIdx >= 0 && leafIdx < len(tp.leaves) {
		leaf := tp.leaves[leafIdx]
		leaf.mu.Lock()
		leaf.pointIDs = append(leaf.pointIDs, pointID)
		leaf.mu.Unlock()
	}
}

// findLeaf finds the leaf node for a vector
func (tp *TreePartitioner) findLeaf(vec point.Vector) int {
	node := tp.root
	for node != nil && !node.isLeaf {
		if vec[node.splitDim] <= node.splitValue {
			node = node.left
		} else {
			node = node.right
		}
	}

	if node != nil {
		return node.leafIndex
	}
	return -1
}

// FindNearestLeaves finds the n nearest leaf partitions
func (tp *TreePartitioner) FindNearestLeaves(vec point.Vector, n int) []int {
	tp.mu.RLock()
	defer tp.mu.RUnlock()

	if !tp.trained || len(tp.leaves) == 0 {
		return nil
	}

	if n > len(tp.leaves) {
		n = len(tp.leaves)
	}

	// Compute distance to each leaf centroid
	type leafDist struct {
		index    int
		distance float32
	}

	distances := make([]leafDist, len(tp.leaves))
	for i, leaf := range tp.leaves {
		distances[i] = leafDist{
			index:    i,
			distance: tp.distCalc.Distance(vec, leaf.centroid),
		}
	}

	// Sort by distance
	for i := 0; i < n && i < len(distances); i++ {
		minIdx := i
		for j := i + 1; j < len(distances); j++ {
			if distances[j].distance < distances[minIdx].distance {
				minIdx = j
			}
		}
		distances[i], distances[minIdx] = distances[minIdx], distances[i]
	}

	result := make([]int, n)
	for i := 0; i < n; i++ {
		result[i] = distances[i].index
	}

	return result
}

// GetLeaf returns the leaf at the given index
func (tp *TreePartitioner) GetLeaf(index int) *leafNode {
	tp.mu.RLock()
	defer tp.mu.RUnlock()

	if index >= 0 && index < len(tp.leaves) {
		return tp.leaves[index]
	}
	return nil
}

// NumLeaves returns the number of leaves
func (tp *TreePartitioner) NumLeaves() int {
	tp.mu.RLock()
	defer tp.mu.RUnlock()
	return len(tp.leaves)
}

// Helper function to compute median
func median(values []float32) float32 {
	if len(values) == 0 {
		return 0
	}

	// Make a copy to avoid modifying original
	sorted := make([]float32, len(values))
	copy(sorted, values)

	// Simple selection for median
	n := len(sorted)
	mid := n / 2

	for i := 0; i <= mid; i++ {
		minIdx := i
		for j := i + 1; j < n; j++ {
			if sorted[j] < sorted[minIdx] {
				minIdx = j
			}
		}
		sorted[i], sorted[minIdx] = sorted[minIdx], sorted[i]
	}

	return sorted[mid]
}
