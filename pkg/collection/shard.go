package collection

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/index/hnsw"
	"github.com/limyedb/limyedb/pkg/index/payload"
	"github.com/limyedb/limyedb/pkg/point"
	"github.com/limyedb/limyedb/pkg/storage/mmap"
)

// ShardState represents the state of a shard
type ShardState string

const (
	ShardStateActive      ShardState = "active"
	ShardStateRecovering  ShardState = "recovering"
	ShardStateDead        ShardState = "dead"
	ShardStateInitializing ShardState = "initializing"
)

// Shard represents a partition of a collection
type Shard struct {
	ID           uint32     `json:"id"`
	CollectionID string     `json:"collection_id"`
	State        ShardState `json:"state"`
	NodeID       string     `json:"node_id"`       // Node hosting this shard
	ReplicaOf    uint32     `json:"replica_of"`    // If replica, ID of primary shard
	IsPrimary    bool       `json:"is_primary"`

	// Internal state
	index        *hnsw.HNSW
	payloadIndex *payload.Index
	config       *config.VectorConfig

	mu        sync.RWMutex
	createdAt time.Time
	updatedAt time.Time
}

// ShardConfig holds shard configuration
type ShardConfig struct {
	ID           uint32
	CollectionID string
	VectorConfig *config.VectorConfig
	DataDir      string
}

// NewShard creates a new shard
func NewShard(cfg *ShardConfig) (*Shard, error) {
	hnswCfg := &hnsw.Config{
		M:              cfg.VectorConfig.HNSW.M,
		EfConstruction: cfg.VectorConfig.HNSW.EfConstruction,
		EfSearch:       cfg.VectorConfig.HNSW.EfSearch,
		MaxElements:    cfg.VectorConfig.HNSW.MaxElements,
		Metric:         cfg.VectorConfig.Metric,
		Dimension:      cfg.VectorConfig.Dimension,
	}

	// Apply defaults
	if hnswCfg.M == 0 {
		hnswCfg.M = 16
	}
	if hnswCfg.EfConstruction == 0 {
		hnswCfg.EfConstruction = 200
	}
	if hnswCfg.EfSearch == 0 {
		hnswCfg.EfSearch = 100
	}
	if hnswCfg.MaxElements == 0 {
		hnswCfg.MaxElements = 100000
	}

	if cfg.VectorConfig.OnDisk {
		os.MkdirAll(cfg.DataDir, 0755)
		mmapCfg := mmap.DefaultConfig()
		mmapCfg.Path = filepath.Join(cfg.DataDir, "vectors.mmap")
		mmapCfg.Dimension = cfg.VectorConfig.Dimension
		
		store, err := mmap.Open(mmapCfg)
		if err != nil {
			return nil, err
		}
		hnswCfg.VectorMmap = store
	}

	index, err := hnsw.New(hnswCfg)
	if err != nil {
		return nil, err
	}

	shard := &Shard{
		ID:           cfg.ID,
		CollectionID: cfg.CollectionID,
		State:        ShardStateInitializing,
		IsPrimary:    true,
		index:        index,
		payloadIndex: payload.NewIndex(),
		config:       cfg.VectorConfig,
		createdAt:    time.Now(),
		updatedAt:    time.Now(),
	}

	return shard, nil
}

// Insert adds a point to the shard
func (s *Shard) Insert(p *point.Point) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State != ShardStateActive {
		return fmt.Errorf("shard %d is not active (state: %s)", s.ID, s.State)
	}

	if err := s.index.Insert(p); err != nil {
		return err
	}

	nodeID, _ := s.index.GetNodeID(p.ID)
	s.payloadIndex.IndexPoint(nodeID, p.Payload)

	s.updatedAt = time.Now()
	return nil
}

// Search performs k-NN search on this shard
func (s *Shard) Search(query point.Vector, k int, ef int) ([]hnsw.Candidate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.State != ShardStateActive && s.State != ShardStateRecovering {
		return nil, fmt.Errorf("shard %d is not searchable (state: %s)", s.ID, s.State)
	}

	return s.index.SearchWithEf(query, k, ef)
}

// Get retrieves a point by ID
func (s *Shard) Get(id string) (*point.Point, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.index.Get(id)
}

// Delete removes a point
func (s *Shard) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State != ShardStateActive {
		return fmt.Errorf("shard %d is not active", s.ID)
	}

	return s.index.Delete(id)
}

// Size returns the number of points in this shard
func (s *Shard) Size() int64 {
	return s.index.Size()
}

// Activate marks the shard as active
func (s *Shard) Activate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = ShardStateActive
	s.updatedAt = time.Now()
}

// RecoveryHandler defines the callback for streaming shard data from a healthy replica
type RecoveryHandler func(shardID uint32, dest *Shard) error

// ShardManager manages shards for a collection
type ShardManager struct {
	shards         map[uint32]*Shard
	shardCount     int
	replicaFactor  int
	collectionName string
	dataDir        string
	recoveryHandler RecoveryHandler

	mu sync.RWMutex
}

// ShardManagerConfig holds shard manager configuration
type ShardManagerConfig struct {
	CollectionName  string
	ShardCount      int
	ReplicaFactor   int
	VectorConfig    *config.VectorConfig
	DataDir         string
	RecoveryHandler RecoveryHandler
}

// NewShardManager creates a new shard manager
func NewShardManager(cfg *ShardManagerConfig) (*ShardManager, error) {
	if cfg.ShardCount <= 0 {
		cfg.ShardCount = 1
	}
	if cfg.ReplicaFactor <= 0 {
		cfg.ReplicaFactor = 1
	}

	sm := &ShardManager{
		shards:          make(map[uint32]*Shard),
		shardCount:      cfg.ShardCount,
		replicaFactor:   cfg.ReplicaFactor,
		collectionName:  cfg.CollectionName,
		dataDir:         cfg.DataDir,
		recoveryHandler: cfg.RecoveryHandler,
	}

	// Create shards
	for i := 0; i < cfg.ShardCount; i++ {
		shardCfg := &ShardConfig{
			ID:           uint32(i),
			CollectionID: cfg.CollectionName,
			VectorConfig: cfg.VectorConfig,
			DataDir:      filepath.Join(cfg.DataDir, fmt.Sprintf("shard_%d", i)),
		}

		shard, err := NewShard(shardCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create shard %d: %w", i, err)
		}

		sm.shards[uint32(i)] = shard
	}

	// Activate all shards
	for _, shard := range sm.shards {
		shard.Activate()
	}

	return sm, nil
}

// GetShard returns the shard for a given point ID
func (sm *ShardManager) GetShard(pointID string) *Shard {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	shardID := sm.computeShardID(pointID)
	return sm.shards[shardID]
}

// GetAllShards returns all shards
func (sm *ShardManager) GetAllShards() []*Shard {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	shards := make([]*Shard, 0, len(sm.shards))
	for _, s := range sm.shards {
		shards = append(shards, s)
	}
	return shards
}

// computeShardID computes which shard a point belongs to
func (sm *ShardManager) computeShardID(pointID string) uint32 {
	hash := sha256.Sum256([]byte(pointID))
	num := binary.BigEndian.Uint32(hash[:4])
	return num % uint32(sm.shardCount)
}

// Insert inserts a point into the appropriate shard
func (sm *ShardManager) Insert(p *point.Point) error {
	shard := sm.GetShard(p.ID)
	if shard == nil {
		return fmt.Errorf("no shard found for point %s", p.ID)
	}
	return shard.Insert(p)
}

// Search performs distributed search across all shards
func (sm *ShardManager) Search(query point.Vector, k int, ef int) ([]hnsw.Candidate, error) {
	sm.mu.RLock()
	shards := make([]*Shard, 0, len(sm.shards))
	for _, s := range sm.shards {
		shards = append(shards, s)
	}
	sm.mu.RUnlock()

	// Search each shard in parallel
	type shardResult struct {
		candidates []hnsw.Candidate
		err        error
	}

	results := make(chan shardResult, len(shards))

	for _, shard := range shards {
		go func(s *Shard) {
			candidates, err := s.Search(query, k, ef)
			results <- shardResult{candidates: candidates, err: err}
		}(shard)
	}

	// Collect and merge results
	var allCandidates []hnsw.Candidate
	for range shards {
		result := <-results
		if result.err != nil {
			continue // Skip failed shards
		}
		allCandidates = append(allCandidates, result.candidates...)
	}

	// Sort and truncate
	sortCandidates(allCandidates)
	if len(allCandidates) > k {
		allCandidates = allCandidates[:k]
	}

	return allCandidates, nil
}

// sortCandidates sorts candidates by distance (ascending)
func sortCandidates(candidates []hnsw.Candidate) {
	for i := 0; i < len(candidates)-1; i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].Distance < candidates[i].Distance {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}
}

// Get retrieves a point from the appropriate shard
func (sm *ShardManager) Get(id string) (*point.Point, error) {
	shard := sm.GetShard(id)
	if shard == nil {
		return nil, fmt.Errorf("no shard found for point %s", id)
	}
	return shard.Get(id)
}

// Delete removes a point from the appropriate shard
func (sm *ShardManager) Delete(id string) error {
	shard := sm.GetShard(id)
	if shard == nil {
		return fmt.Errorf("no shard found for point %s", id)
	}
	return shard.Delete(id)
}

// Size returns total points across all shards
func (sm *ShardManager) Size() int64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var total int64
	for _, shard := range sm.shards {
		total += shard.Size()
	}
	return total
}

// ShardInfo returns information about all shards
func (sm *ShardManager) ShardInfo() []ShardInfo {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	infos := make([]ShardInfo, 0, len(sm.shards))
	for _, shard := range sm.shards {
		infos = append(infos, ShardInfo{
			ID:        shard.ID,
			State:     shard.State,
			NodeID:    shard.NodeID,
			IsPrimary: shard.IsPrimary,
			Size:      shard.Size(),
		})
	}
	return infos
}

// ShardInfo holds information about a shard
type ShardInfo struct {
	ID        uint32     `json:"id"`
	State     ShardState `json:"state"`
	NodeID    string     `json:"node_id"`
	IsPrimary bool       `json:"is_primary"`
	Size      int64      `json:"size"`
}

// RecoverShard initiates recovery for a failed shard
func (sm *ShardManager) RecoverShard(shardID uint32) error {
	sm.mu.Lock()
	shard, ok := sm.shards[shardID]
	sm.mu.Unlock()

	if !ok {
		return fmt.Errorf("shard %d not found", shardID)
	}

	shard.mu.Lock()
	shard.State = ShardStateRecovering
	shard.mu.Unlock()

	// Fully implemented recovery logic delegating to a configured RecoveryHandler
	// which handles streaming from external nodes/replicas in a cluster.
	if sm.recoveryHandler != nil {
		if err := sm.recoveryHandler(shardID, shard); err != nil {
			shard.mu.Lock()
			shard.State = ShardStateDead
			shard.mu.Unlock()
			return fmt.Errorf("shard recovery stream failed: %w", err)
		}
	} else {
		// No handler configured, cannot fetch data from external replicas
		shard.mu.Lock()
		shard.State = ShardStateDead
		shard.mu.Unlock()
		return fmt.Errorf("shard recovery failed: no recovery handler configured for cluster")
	}

	shard.mu.Lock()
	shard.State = ShardStateActive
	shard.mu.Unlock()

	return nil
}

// SaveMetadata saves shard metadata to disk
func (sm *ShardManager) SaveMetadata() error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	metadata := struct {
		CollectionName string       `json:"collection_name"`
		ShardCount     int          `json:"shard_count"`
		ReplicaFactor  int          `json:"replica_factor"`
		Shards         []ShardInfo  `json:"shards"`
	}{
		CollectionName: sm.collectionName,
		ShardCount:     sm.shardCount,
		ReplicaFactor:  sm.replicaFactor,
		Shards:         sm.ShardInfo(),
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}

	metaPath := filepath.Join(sm.dataDir, "shards.json")
	return os.WriteFile(metaPath, data, 0600)
}
