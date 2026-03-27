package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"
)

// RecoveryConfig holds recovery configuration
type RecoveryConfig struct {
	RecoveryInterval   time.Duration
	StreamBufferSize   int
	MaxConcurrentRecov int
	ChecksumEnabled    bool
}

// DefaultRecoveryConfig returns default recovery configuration
func DefaultRecoveryConfig() *RecoveryConfig {
	return &RecoveryConfig{
		RecoveryInterval:   30 * time.Second,
		StreamBufferSize:   1024 * 1024, // 1MB
		MaxConcurrentRecov: 2,
		ChecksumEnabled:    true,
	}
}

// RecoveryManager handles shard recovery and data repair
type RecoveryManager struct {
	config       *RecoveryConfig
	coordinator  *Coordinator
	transport    Transport
	dataProvider DataProvider

	// Recovery state
	recovering    map[uint32]*recoveryJob
	recoveryQueue []uint32

	mu     sync.RWMutex
	stopCh chan struct{}
}

// DataProvider provides access to local data for recovery
type DataProvider interface {
	// GetShardData returns an iterator over all data in a shard
	GetShardData(shardID uint32) (DataIterator, error)

	// ImportData imports data into a shard
	ImportData(shardID uint32, data []byte) error

	// GetChecksum returns the checksum for a shard
	GetChecksum(shardID uint32) (string, error)

	// GetShardSize returns the size of a shard in bytes
	GetShardSize(shardID uint32) (int64, error)
}

// DataIterator iterates over shard data
type DataIterator interface {
	Next() bool
	Data() []byte
	Error() error
	Close() error
}

// recoveryJob tracks a recovery operation
type recoveryJob struct {
	ShardID     uint32
	SourceNode  string
	StartTime   time.Time
	BytesRecv   int64
	Status      RecoveryStatus
	Error       error
	Progress    float64
}

// RecoveryStatus represents the status of a recovery job
type RecoveryStatus string

const (
	RecoveryStatusPending   RecoveryStatus = "pending"
	RecoveryStatusStreaming RecoveryStatus = "streaming"
	RecoveryStatusApplying  RecoveryStatus = "applying"
	RecoveryStatusComplete  RecoveryStatus = "complete"
	RecoveryStatusFailed    RecoveryStatus = "failed"
)

// NewRecoveryManager creates a new recovery manager
func NewRecoveryManager(config *RecoveryConfig, coordinator *Coordinator, transport Transport, dataProvider DataProvider) *RecoveryManager {
	if config == nil {
		config = DefaultRecoveryConfig()
	}

	return &RecoveryManager{
		config:        config,
		coordinator:   coordinator,
		transport:     transport,
		dataProvider:  dataProvider,
		recovering:    make(map[uint32]*recoveryJob),
		recoveryQueue: make([]uint32, 0),
		stopCh:        make(chan struct{}),
	}
}

// Start starts the recovery manager
func (rm *RecoveryManager) Start() error {
	go rm.recoveryLoop()
	go rm.antiEntropyLoop()
	return nil
}

// Stop stops the recovery manager
func (rm *RecoveryManager) Stop() error {
	close(rm.stopCh)
	return nil
}

// RecoverShard initiates recovery for a shard
func (rm *RecoveryManager) RecoverShard(shardID uint32) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Check if already recovering
	if _, exists := rm.recovering[shardID]; exists {
		return errors.New("shard already recovering")
	}

	// Add to recovery queue
	rm.recoveryQueue = append(rm.recoveryQueue, shardID)
	return nil
}

// GetRecoveryStatus returns the status of all recovery jobs
func (rm *RecoveryManager) GetRecoveryStatus() map[uint32]*recoveryJob {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	status := make(map[uint32]*recoveryJob)
	for shardID, job := range rm.recovering {
		status[shardID] = job
	}
	return status
}

func (rm *RecoveryManager) recoveryLoop() {
	ticker := time.NewTicker(rm.config.RecoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rm.stopCh:
			return
		case <-ticker.C:
			rm.processRecoveryQueue()
		}
	}
}

func (rm *RecoveryManager) processRecoveryQueue() {
	rm.mu.Lock()

	// Check if we can start more recoveries
	activeCount := 0
	for _, job := range rm.recovering {
		if job.Status == RecoveryStatusStreaming || job.Status == RecoveryStatusApplying {
			activeCount++
		}
	}

	if activeCount >= rm.config.MaxConcurrentRecov || len(rm.recoveryQueue) == 0 {
		rm.mu.Unlock()
		return
	}

	// Get next shard to recover
	shardID := rm.recoveryQueue[0]
	rm.recoveryQueue = rm.recoveryQueue[1:]

	// Find source node
	shard := rm.coordinator.shardManager.GetShard(shardID)
	if shard == nil {
		rm.mu.Unlock()
		return
	}

	sourceNode := rm.findSourceNode(shardID, shard)
	if sourceNode == nil {
		rm.mu.Unlock()
		return
	}

	// Create recovery job
	job := &recoveryJob{
		ShardID:    shardID,
		SourceNode: sourceNode.ID,
		StartTime:  time.Now(),
		Status:     RecoveryStatusStreaming,
	}
	rm.recovering[shardID] = job
	rm.mu.Unlock()

	// Start recovery in background
	go rm.executeRecovery(job, sourceNode)
}

func (rm *RecoveryManager) findSourceNode(shardID uint32, shard *Shard) *Node {
	// Try primary first
	if shard.Primary != "" {
		nodes := rm.coordinator.hashRing.GetAllNodes()
		for _, node := range nodes {
			if node.ID == shard.Primary && node.IsHealthy() {
				return node
			}
		}
	}

	// Try replicas
	for _, replicaID := range shard.Replicas {
		nodes := rm.coordinator.hashRing.GetAllNodes()
		for _, node := range nodes {
			if node.ID == replicaID && node.IsHealthy() {
				return node
			}
		}
	}

	return nil
}

func (rm *RecoveryManager) executeRecovery(job *recoveryJob, sourceNode *Node) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Request data stream from source
	stream, err := rm.requestDataStream(ctx, sourceNode, job.ShardID)
	if err != nil {
		rm.failRecovery(job, err)
		return
	}
	defer func() { _ = stream.Close() }()

	// Receive and apply data
	var totalBytes int64
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			rm.failRecovery(job, err)
			return
		}

		// Apply data
		if err := rm.dataProvider.ImportData(job.ShardID, msg.Payload); err != nil {
			rm.failRecovery(job, err)
			return
		}

		totalBytes += int64(len(msg.Payload))
		job.BytesRecv = totalBytes
	}

	// Verify checksum if enabled
	if rm.config.ChecksumEnabled {
		localChecksum, err := rm.dataProvider.GetChecksum(job.ShardID)
		if err != nil {
			rm.failRecovery(job, err)
			return
		}

		// Request remote checksum
		remoteChecksum, err := rm.requestChecksum(ctx, sourceNode, job.ShardID)
		if err != nil {
			rm.failRecovery(job, err)
			return
		}

		if localChecksum != remoteChecksum {
			rm.failRecovery(job, errors.New("checksum mismatch"))
			return
		}
	}

	// Mark complete
	rm.mu.Lock()
	job.Status = RecoveryStatusComplete
	job.Progress = 100
	rm.mu.Unlock()
}

func (rm *RecoveryManager) failRecovery(job *recoveryJob, err error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	job.Status = RecoveryStatusFailed
	job.Error = err
}

func (rm *RecoveryManager) requestDataStream(ctx context.Context, node *Node, shardID uint32) (Stream, error) {
	stream, err := rm.transport.Stream(ctx, node.Address)
	if err != nil {
		return nil, err
	}

	// Send stream request
	req := &StreamRequest{
		Type:    StreamTypeRecovery,
		ShardID: shardID,
	}
	reqData, _ := json.Marshal(req)

	if err := stream.Send(&Message{
		Type:    MsgTypeStreamData,
		Payload: reqData,
	}); err != nil {
		_ = stream.Close() // Best effort close on send error
		return nil, err
	}

	return stream, nil
}

func (rm *RecoveryManager) requestChecksum(ctx context.Context, node *Node, shardID uint32) (string, error) {
	req := &ChecksumRequest{
		ShardID: shardID,
	}

	msg, err := NewMessage(MsgTypeRepairData, rm.coordinator.localNode.ID, node.Address, req)
	if err != nil {
		return "", err
	}

	resp, err := rm.transport.Send(ctx, node.Address, msg)
	if err != nil {
		return "", err
	}

	var checksumResp ChecksumResponse
	if err := resp.Decode(&checksumResp); err != nil {
		return "", err
	}

	return checksumResp.Checksum, nil
}

// StreamRequest represents a request to stream data
type StreamRequest struct {
	Type    StreamType `json:"type"`
	ShardID uint32     `json:"shard_id"`
	FromKey string     `json:"from_key,omitempty"`
}

// StreamType represents the type of stream
type StreamType string

const (
	StreamTypeRecovery   StreamType = "recovery"
	StreamTypeAntiEntropy StreamType = "anti_entropy"
	StreamTypeSnapshot   StreamType = "snapshot"
)

// ChecksumRequest represents a checksum request
type ChecksumRequest struct {
	ShardID uint32 `json:"shard_id"`
}

// ChecksumResponse represents a checksum response
type ChecksumResponse struct {
	ShardID  uint32 `json:"shard_id"`
	Checksum string `json:"checksum"`
	Size     int64  `json:"size"`
}

// AntiEntropy performs anti-entropy repair
func (rm *RecoveryManager) antiEntropyLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-rm.stopCh:
			return
		case <-ticker.C:
			rm.runAntiEntropy()
		}
	}
}

func (rm *RecoveryManager) runAntiEntropy() {
	// Get shards we're responsible for
	localShards := rm.getLocalShards()

	for _, shardID := range localShards {
		rm.repairShard(shardID)
	}
}

func (rm *RecoveryManager) getLocalShards() []uint32 {
	localNodeID := rm.coordinator.localNode.ID
	var shards []uint32

	rm.coordinator.shardManager.mu.RLock()
	defer rm.coordinator.shardManager.mu.RUnlock()

	for shardID, shard := range rm.coordinator.shardManager.shards {
		if shard.Primary == localNodeID {
			shards = append(shards, shardID)
		}
	}

	return shards
}

func (rm *RecoveryManager) repairShard(shardID uint32) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	shard := rm.coordinator.shardManager.GetShard(shardID)
	if shard == nil {
		return
	}

	// Get local checksum
	localChecksum, err := rm.dataProvider.GetChecksum(shardID)
	if err != nil {
		return
	}

	// Compare with replicas
	for _, replicaID := range shard.Replicas {
		nodes := rm.coordinator.hashRing.GetAllNodes()
		for _, node := range nodes {
			if node.ID != replicaID || !node.IsHealthy() {
				continue
			}

			remoteChecksum, err := rm.requestChecksum(ctx, node, shardID)
			if err != nil {
				continue
			}

			if localChecksum != remoteChecksum {
				// Repair needed - push our data to the replica
				go rm.pushDataToReplica(node, shardID)
			}
		}
	}
}

func (rm *RecoveryManager) pushDataToReplica(node *Node, shardID uint32) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	stream, err := rm.transport.Stream(ctx, node.Address)
	if err != nil {
		return
	}
	defer func() { _ = stream.Close() }()

	// Get local data iterator
	iter, err := rm.dataProvider.GetShardData(shardID)
	if err != nil {
		return
	}
	defer func() { _ = iter.Close() }()

	// Stream data to replica
	for iter.Next() {
		if err := stream.Send(&Message{
			Type:    MsgTypeStreamData,
			Payload: iter.Data(),
		}); err != nil {
			return
		}
	}
}

// HandleStreamRequest handles an incoming stream request
func (rm *RecoveryManager) HandleStreamRequest(stream Stream, req *StreamRequest) error {
	// Get local data iterator
	iter, err := rm.dataProvider.GetShardData(req.ShardID)
	if err != nil {
		return err
	}
	defer func() { _ = iter.Close() }()

	// Stream data
	for iter.Next() {
		if err := stream.Send(&Message{
			Type:    MsgTypeStreamData,
			Payload: iter.Data(),
		}); err != nil {
			return err
		}
	}

	return iter.Error()
}

// HandleChecksumRequest handles a checksum request
func (rm *RecoveryManager) HandleChecksumRequest(req *ChecksumRequest) (*ChecksumResponse, error) {
	checksum, err := rm.dataProvider.GetChecksum(req.ShardID)
	if err != nil {
		return nil, err
	}

	size, err := rm.dataProvider.GetShardSize(req.ShardID)
	if err != nil {
		return nil, err
	}

	return &ChecksumResponse{
		ShardID:  req.ShardID,
		Checksum: checksum,
		Size:     size,
	}, nil
}
