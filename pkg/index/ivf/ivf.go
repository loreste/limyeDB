package ivf

import (
	"errors"
	"math"
	"sync"
	"sync/atomic"

	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/distance"
	"github.com/limyedb/limyedb/pkg/point"
	"github.com/limyedb/limyedb/pkg/quantization"
)

// Config holds IVF index configuration
type Config struct {
	NumClusters     int                  // Number of clusters (K), default: sqrt(n)
	Nprobe          int                  // Number of clusters to search (default: 10)
	Metric          config.MetricType    // Distance metric
	Dimension       int                  // Vector dimension
	Quantizer       quantization.Quantizer
	TrainingSamples int                  // Samples for k-means training
	MaxElements     int                  // Maximum number of elements
}

// DefaultConfig returns default IVF configuration
func DefaultConfig() *Config {
	return &Config{
		NumClusters:     100,
		Nprobe:          10,
		Metric:          config.MetricCosine,
		Dimension:       0, // Must be set
		TrainingSamples: 10000,
		MaxElements:     100000,
	}
}

// IVF implements the Inverted File Index
type IVF struct {
	config    *Config
	kmeans    *KMeans
	clusters  map[int]*Cluster // cluster_id -> cluster
	distCalc  distance.Calculator
	quantizer quantization.Quantizer

	// Point storage
	points      []*point.Point
	idToIndex   map[string]uint32
	pointCount  atomic.Int64
	deletedCount atomic.Int64

	// Training state
	trainingBuffer []point.Vector
	trained        bool

	// Concurrency control
	mu        sync.RWMutex
	buildLock sync.Mutex
}

// Cluster holds vectors assigned to a centroid
type Cluster struct {
	ID       int
	PointIDs []uint32 // Indices into IVF.points
	mu       sync.RWMutex
}

// New creates a new IVF index
func New(cfg *Config) (*IVF, error) {
	if cfg.Dimension <= 0 {
		return nil, errors.New("dimension must be positive")
	}
	if cfg.NumClusters <= 0 {
		cfg.NumClusters = 100
	}
	if cfg.Nprobe <= 0 {
		cfg.Nprobe = 10
	}
	if cfg.Nprobe > cfg.NumClusters {
		cfg.Nprobe = cfg.NumClusters
	}

	distCalc := distance.New(cfg.Metric)

	kmeansCfg := &KMeansConfig{
		NumClusters:   cfg.NumClusters,
		MaxIterations: 50,
		Tolerance:     1e-4,
		NumInitRuns:   3,
	}

	ivf := &IVF{
		config:         cfg,
		kmeans:         NewKMeans(kmeansCfg, distCalc, cfg.Dimension),
		clusters:       make(map[int]*Cluster),
		distCalc:       distCalc,
		quantizer:      cfg.Quantizer,
		points:         make([]*point.Point, 0, cfg.MaxElements),
		idToIndex:      make(map[string]uint32),
		trainingBuffer: make([]point.Vector, 0, cfg.TrainingSamples),
	}

	// Initialize clusters
	for i := 0; i < cfg.NumClusters; i++ {
		ivf.clusters[i] = &Cluster{
			ID:       i,
			PointIDs: make([]uint32, 0),
		}
	}

	return ivf, nil
}

// Insert adds a point to the index
func (ivf *IVF) Insert(p *point.Point) error {
	if len(p.Vector) != ivf.config.Dimension {
		return ErrDimensionMismatch
	}

	ivf.buildLock.Lock()
	defer ivf.buildLock.Unlock()

	// Check if ID already exists
	ivf.mu.RLock()
	if _, exists := ivf.idToIndex[p.ID]; exists {
		ivf.mu.RUnlock()
		return ErrPointExists
	}
	ivf.mu.RUnlock()

	// If not trained, add to training buffer
	if !ivf.trained {
		ivf.mu.Lock()
		ivf.trainingBuffer = append(ivf.trainingBuffer, copyVector(p.Vector))

		// Auto-train when buffer is full
		if len(ivf.trainingBuffer) >= ivf.config.TrainingSamples {
			buffer := ivf.trainingBuffer
			ivf.trainingBuffer = nil
			ivf.mu.Unlock()

			// Train with buffered vectors
			if err := ivf.Train(buffer); err != nil {
				return err
			}

			// The current point still needs to be inserted (it wasn't stored yet)
			return ivf.insertTrained(p)
		}
		ivf.mu.Unlock()

		// Store point even before training
		return ivf.storePoint(p, -1)
	}

	return ivf.insertTrained(p)
}

// insertTrained inserts a point into the trained index
func (ivf *IVF) insertTrained(p *point.Point) error {
	// Find nearest centroid
	clusterID := ivf.kmeans.FindNearestCentroid(p.Vector)
	if clusterID < 0 {
		return errors.New("failed to find cluster")
	}

	return ivf.storePoint(p, clusterID)
}

// storePoint stores a point and optionally assigns it to a cluster
func (ivf *IVF) storePoint(p *point.Point, clusterID int) error {
	ivf.mu.Lock()
	defer ivf.mu.Unlock()

	// Check again under write lock
	if _, exists := ivf.idToIndex[p.ID]; exists {
		return ErrPointExists
	}

	// Store the point with safe integer conversion
	n := len(ivf.points)
	if n < 0 || n > math.MaxUint32 {
		return errors.New("point count exceeds uint32 range")
	}
	pointID := uint32(n)
	ivf.points = append(ivf.points, p)
	ivf.idToIndex[p.ID] = pointID
	ivf.pointCount.Add(1)

	// Assign to cluster if trained
	if clusterID >= 0 {
		cluster := ivf.clusters[clusterID]
		cluster.mu.Lock()
		cluster.PointIDs = append(cluster.PointIDs, pointID)
		cluster.mu.Unlock()
	}

	return nil
}

// Train trains the IVF index using k-means clustering
func (ivf *IVF) Train(vectors []point.Vector) error {
	if len(vectors) == 0 {
		return errors.New("no vectors to train on")
	}

	// Adjust number of clusters if needed
	numClusters := ivf.config.NumClusters
	if numClusters > len(vectors) {
		numClusters = int(math.Sqrt(float64(len(vectors))))
		if numClusters < 1 {
			numClusters = 1
		}
	}

	// Update k-means config
	ivf.kmeans.config.NumClusters = numClusters

	// Train k-means
	if err := ivf.kmeans.Train(vectors); err != nil {
		return err
	}

	ivf.mu.Lock()
	ivf.trained = true

	// Reinitialize clusters with correct count
	ivf.clusters = make(map[int]*Cluster)
	for i := 0; i < numClusters; i++ {
		ivf.clusters[i] = &Cluster{
			ID:       i,
			PointIDs: make([]uint32, 0),
		}
	}

	// Reassign all existing points to clusters
	for i, p := range ivf.points {
		clusterID := ivf.kmeans.FindNearestCentroid(p.Vector)
		if clusterID >= 0 && clusterID < numClusters {
			cluster := ivf.clusters[clusterID]
			cluster.PointIDs = append(cluster.PointIDs, uint32(i))
		}
	}
	ivf.mu.Unlock()

	// Train quantizer if available
	if ivf.quantizer != nil {
		return ivf.quantizer.Train(vectors)
	}

	return nil
}

// Delete marks a point as deleted
func (ivf *IVF) Delete(id string) error {
	ivf.mu.RLock()
	_, exists := ivf.idToIndex[id]
	ivf.mu.RUnlock()

	if !exists {
		return ErrPointNotFound
	}

	// Mark as deleted (lazy deletion)
	ivf.deletedCount.Add(1)
	return nil
}

// Get retrieves a point by ID
func (ivf *IVF) Get(id string) (*point.Point, error) {
	ivf.mu.RLock()
	idx, exists := ivf.idToIndex[id]
	ivf.mu.RUnlock()

	if !exists {
		return nil, ErrPointNotFound
	}

	return ivf.points[idx], nil
}

// Size returns the number of points (excluding deleted)
func (ivf *IVF) Size() int64 {
	return ivf.pointCount.Load() - ivf.deletedCount.Load()
}

// TotalSize returns total number of points including deleted
func (ivf *IVF) TotalSize() int64 {
	return ivf.pointCount.Load()
}

// IsTrained returns whether the index has been trained
func (ivf *IVF) IsTrained() bool {
	ivf.mu.RLock()
	defer ivf.mu.RUnlock()
	return ivf.trained
}

// Centroids returns the cluster centroids
func (ivf *IVF) Centroids() []point.Vector {
	return ivf.kmeans.Centroids()
}

// NumClusters returns the number of clusters
func (ivf *IVF) NumClusters() int {
	ivf.mu.RLock()
	defer ivf.mu.RUnlock()
	return len(ivf.clusters)
}

// ClusterSizes returns the number of points in each cluster
func (ivf *IVF) ClusterSizes() map[int]int {
	ivf.mu.RLock()
	defer ivf.mu.RUnlock()

	sizes := make(map[int]int)
	for id, cluster := range ivf.clusters {
		cluster.mu.RLock()
		sizes[id] = len(cluster.PointIDs)
		cluster.mu.RUnlock()
	}
	return sizes
}

// GetNodeID returns the internal node ID for a point ID
func (ivf *IVF) GetNodeID(pointID string) (uint32, bool) {
	ivf.mu.RLock()
	defer ivf.mu.RUnlock()
	nodeID, exists := ivf.idToIndex[pointID]
	return nodeID, exists
}

// GetPointID returns the point ID for an internal node ID
func (ivf *IVF) GetPointID(nodeID uint32) string {
	ivf.mu.RLock()
	defer ivf.mu.RUnlock()
	if int(nodeID) >= len(ivf.points) {
		return ""
	}
	return ivf.points[nodeID].ID
}

// Iterate iterates over all non-deleted point IDs
func (ivf *IVF) Iterate(fn func(id string) error) error {
	ivf.mu.RLock()
	defer ivf.mu.RUnlock()

	for _, p := range ivf.points {
		if err := fn(p.ID); err != nil {
			return err
		}
	}
	return nil
}

// GetAllPoints returns all points
func (ivf *IVF) GetAllPoints() []*point.Point {
	ivf.mu.RLock()
	defer ivf.mu.RUnlock()

	result := make([]*point.Point, len(ivf.points))
	copy(result, ivf.points)
	return result
}

// SetNprobe sets the number of clusters to search
func (ivf *IVF) SetNprobe(nprobe int) {
	ivf.mu.Lock()
	defer ivf.mu.Unlock()

	if nprobe > 0 && nprobe <= len(ivf.clusters) {
		ivf.config.Nprobe = nprobe
	}
}

// Errors
var (
	ErrDimensionMismatch = errors.New("vector dimension mismatch")
	ErrPointExists       = errors.New("point with this ID already exists")
	ErrPointNotFound     = errors.New("point not found")
	ErrNotTrained        = errors.New("index not trained")
)
