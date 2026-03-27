package cluster

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// =============================================================================
// Cluster Configuration
// =============================================================================

// Config holds cluster configuration
type Config struct {
	NodeID           string        `json:"node_id"`
	ListenAddr       string        `json:"listen_addr"`
	AdvertiseAddr    string        `json:"advertise_addr"`
	SeedNodes        []string      `json:"seed_nodes"`
	ReplicationFactor int          `json:"replication_factor"`
	ShardCount       int           `json:"shard_count"`
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`
	FailureTimeout   time.Duration `json:"failure_timeout"`
}

// DefaultConfig returns default cluster configuration
func DefaultConfig() *Config {
	return &Config{
		ReplicationFactor: 2,
		ShardCount:        16,
		HeartbeatInterval: 1 * time.Second,
		FailureTimeout:    5 * time.Second,
	}
}

// =============================================================================
// Node represents a cluster node
// =============================================================================

// NodeState represents the state of a node
type NodeState string

const (
	NodeStateActive   NodeState = "active"
	NodeStateInactive NodeState = "inactive"
	NodeStateSuspect  NodeState = "suspect"
	NodeStateLeaving  NodeState = "leaving"
)

// Node represents a cluster node
type Node struct {
	ID            string            `json:"id"`
	Address       string            `json:"address"`
	State         NodeState         `json:"state"`
	LastHeartbeat time.Time         `json:"last_heartbeat"`
	Shards        []uint32          `json:"shards"`        // Primary shards
	ReplicaShards []uint32          `json:"replica_shards"` // Replica shards
	Metadata      map[string]string `json:"metadata"`
}

// IsHealthy returns true if the node is healthy
func (n *Node) IsHealthy() bool {
	return n.State == NodeStateActive
}

// =============================================================================
// Consistent Hashing Ring
// =============================================================================

// HashRing implements consistent hashing with virtual nodes
type HashRing struct {
	nodes       map[string]*Node
	ring        []hashEntry
	virtualNodes int
	mu          sync.RWMutex
}

type hashEntry struct {
	hash   uint32
	nodeID string
}

// NewHashRing creates a new hash ring
func NewHashRing(virtualNodes int) *HashRing {
	if virtualNodes < 1 {
		virtualNodes = 150 // Default virtual nodes per physical node
	}
	return &HashRing{
		nodes:        make(map[string]*Node),
		ring:         make([]hashEntry, 0),
		virtualNodes: virtualNodes,
	}
}

// AddNode adds a node to the ring
func (hr *HashRing) AddNode(node *Node) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	hr.nodes[node.ID] = node

	// Add virtual nodes
	for i := 0; i < hr.virtualNodes; i++ {
		key := fmt.Sprintf("%s-%d", node.ID, i)
		hash := hashKey(key)
		hr.ring = append(hr.ring, hashEntry{hash: hash, nodeID: node.ID})
	}

	// Sort ring by hash
	sort.Slice(hr.ring, func(i, j int) bool {
		return hr.ring[i].hash < hr.ring[j].hash
	})
}

// RemoveNode removes a node from the ring
func (hr *HashRing) RemoveNode(nodeID string) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	delete(hr.nodes, nodeID)

	// Remove virtual nodes
	newRing := hr.ring[:0]
	for _, entry := range hr.ring {
		if entry.nodeID != nodeID {
			newRing = append(newRing, entry)
		}
	}
	hr.ring = newRing
}

// GetNode returns the node responsible for a key
func (hr *HashRing) GetNode(key string) *Node {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	if len(hr.ring) == 0 {
		return nil
	}

	hash := hashKey(key)

	// Binary search for the first node with hash >= key hash
	idx := sort.Search(len(hr.ring), func(i int) bool {
		return hr.ring[i].hash >= hash
	})

	// Wrap around if necessary
	if idx >= len(hr.ring) {
		idx = 0
	}

	return hr.nodes[hr.ring[idx].nodeID]
}

// GetNodes returns N nodes responsible for a key (for replication)
func (hr *HashRing) GetNodes(key string, n int) []*Node {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	if len(hr.ring) == 0 || n <= 0 {
		return nil
	}

	hash := hashKey(key)

	// Find starting position
	idx := sort.Search(len(hr.ring), func(i int) bool {
		return hr.ring[i].hash >= hash
	})
	if idx >= len(hr.ring) {
		idx = 0
	}

	// Collect unique nodes
	nodes := make([]*Node, 0, n)
	seen := make(map[string]bool)

	for i := 0; i < len(hr.ring) && len(nodes) < n; i++ {
		entryIdx := (idx + i) % len(hr.ring)
		nodeID := hr.ring[entryIdx].nodeID

		if !seen[nodeID] {
			seen[nodeID] = true
			if node := hr.nodes[nodeID]; node != nil && node.IsHealthy() {
				nodes = append(nodes, node)
			}
		}
	}

	return nodes
}

// GetAllNodes returns all nodes in the ring
func (hr *HashRing) GetAllNodes() []*Node {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	nodes := make([]*Node, 0, len(hr.nodes))
	for _, node := range hr.nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// hashKey hashes a key to a uint32 using SHA-256 (cryptographically secure)
func hashKey(key string) uint32 {
	hash := sha256.Sum256([]byte(key))
	return binary.BigEndian.Uint32(hash[:4])
}

// =============================================================================
// Shard Management
// =============================================================================

// Shard represents a data shard
type Shard struct {
	ID       uint32   `json:"id"`
	Primary  string   `json:"primary"`   // Primary node ID
	Replicas []string `json:"replicas"`  // Replica node IDs
	State    ShardState `json:"state"`
}

// ShardState represents the state of a shard
type ShardState string

const (
	ShardStateActive      ShardState = "active"
	ShardStateRecovering  ShardState = "recovering"
	ShardStateRebalancing ShardState = "rebalancing"
)

// ShardManager manages shard assignments
type ShardManager struct {
	shards      map[uint32]*Shard
	shardCount  uint32
	replication int
	hashRing    *HashRing
	mu          sync.RWMutex
}

// NewShardManager creates a new shard manager
func NewShardManager(shardCount int, replicationFactor int) *ShardManager {
	if shardCount < 0 {
		shardCount = 16 // Default
	}
	sm := &ShardManager{
		shards:      make(map[uint32]*Shard),
		shardCount:  uint32(shardCount), // #nosec G115 - validated above
		replication: replicationFactor,
		hashRing:    NewHashRing(150),
	}

	// Initialize shards
	for i := uint32(0); i < sm.shardCount; i++ {
		sm.shards[i] = &Shard{
			ID:       i,
			State:    ShardStateActive,
			Replicas: make([]string, 0),
		}
	}

	return sm
}

// GetShardForKey returns the shard ID for a key
func (sm *ShardManager) GetShardForKey(key string) uint32 {
	hash := hashKey(key)
	return hash % sm.shardCount
}

// GetNodesForKey returns the nodes responsible for a key
func (sm *ShardManager) GetNodesForKey(key string) []*Node {
	return sm.hashRing.GetNodes(key, sm.replication)
}

// GetShard returns a shard by ID
func (sm *ShardManager) GetShard(shardID uint32) *Shard {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.shards[shardID]
}

// GetShards returns a copy of all shards
func (sm *ShardManager) GetShards() map[uint32]*Shard {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	shards := make(map[uint32]*Shard, len(sm.shards))
	for id, shard := range sm.shards {
		shards[id] = shard
	}
	return shards
}

// AddNode adds a node and rebalances shards
func (sm *ShardManager) AddNode(node *Node) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.hashRing.AddNode(node)
	sm.rebalanceShards()
}

// RemoveNode removes a node and rebalances shards
func (sm *ShardManager) RemoveNode(nodeID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.hashRing.RemoveNode(nodeID)
	sm.rebalanceShards()
}

// rebalanceShards redistributes shards across nodes
func (sm *ShardManager) rebalanceShards() {
	nodes := sm.hashRing.GetAllNodes()
	if len(nodes) == 0 {
		return
	}

	// Simple round-robin assignment for now
	for shardID := uint32(0); shardID < sm.shardCount; shardID++ {
		shard := sm.shards[shardID]

		// Assign primary
		primaryIdx := int(shardID) % len(nodes)
		shard.Primary = nodes[primaryIdx].ID

		// Assign replicas
		shard.Replicas = make([]string, 0, sm.replication-1)
		for r := 1; r < sm.replication && r < len(nodes); r++ {
			replicaIdx := (primaryIdx + r) % len(nodes)
			shard.Replicas = append(shard.Replicas, nodes[replicaIdx].ID)
		}
	}
}

// GetPrimaryNode returns the primary node for a shard
func (sm *ShardManager) GetPrimaryNode(shardID uint32) *Node {
	sm.mu.RLock()
	shard := sm.shards[shardID]
	if shard == nil {
		sm.mu.RUnlock()
		return nil
	}
	primaryID := shard.Primary
	sm.mu.RUnlock()

	nodes := sm.hashRing.GetAllNodes()
	for _, node := range nodes {
		if node.ID == primaryID {
			return node
		}
	}
	return nil
}

// =============================================================================
// Cluster Coordinator
// =============================================================================

// Coordinator manages cluster membership and coordination
type Coordinator struct {
	config       *Config
	localNode    *Node
	shardManager *ShardManager
	hashRing     *HashRing

	// Membership
	members map[string]*Node
	mu      sync.RWMutex

	// Channels
	stopCh   chan struct{}
	memberCh chan MemberEvent

	// Goroutine tracking to prevent leaks
	wg sync.WaitGroup

	// State
	isLeader bool
	leaderID string
}

// MemberEvent represents a membership change event
type MemberEvent struct {
	Type   MemberEventType
	Node   *Node
	Time   time.Time
}

// MemberEventType represents the type of membership event
type MemberEventType string

const (
	MemberEventJoin   MemberEventType = "join"
	MemberEventLeave  MemberEventType = "leave"
	MemberEventFailed MemberEventType = "failed"
)

// NewCoordinator creates a new cluster coordinator
func NewCoordinator(config *Config) *Coordinator {
	localNode := &Node{
		ID:       config.NodeID,
		Address:  config.AdvertiseAddr,
		State:    NodeStateActive,
		Metadata: make(map[string]string),
	}

	c := &Coordinator{
		config:       config,
		localNode:    localNode,
		shardManager: NewShardManager(config.ShardCount, config.ReplicationFactor),
		hashRing:     NewHashRing(150),
		members:      make(map[string]*Node),
		stopCh:       make(chan struct{}),
		memberCh:     make(chan MemberEvent, 100),
	}

	// Add local node
	c.members[localNode.ID] = localNode
	c.hashRing.AddNode(localNode)
	c.shardManager.AddNode(localNode)

	return c
}

// Start starts the coordinator
func (c *Coordinator) Start(ctx context.Context) error {
	// Join seed nodes
	for _, seedAddr := range c.config.SeedNodes {
		if err := c.joinNode(seedAddr); err != nil {
			// Log error but continue
			continue
		}
	}

	// Start heartbeat loop with goroutine tracking
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.heartbeatLoop(ctx)
	}()

	// Start failure detector with goroutine tracking
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.failureDetectorLoop(ctx)
	}()

	return nil
}

// Stop stops the coordinator and waits for goroutines to finish
func (c *Coordinator) Stop() error {
	close(c.stopCh)
	// Wait for all goroutines to complete (with timeout)
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("timeout waiting for coordinator goroutines to stop")
	}
}

// joinNode joins a cluster via a seed node
func (c *Coordinator) joinNode(addr string) error {
	// In a real implementation, this would make an RPC call
	// to the seed node to get the cluster state
	return nil
}

// heartbeatLoop sends periodic heartbeats
func (c *Coordinator) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(c.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.sendHeartbeats()
		}
	}
}

// sendHeartbeats sends heartbeats to all members
func (c *Coordinator) sendHeartbeats() {
	c.mu.Lock()
	c.localNode.LastHeartbeat = time.Now()
	c.mu.Unlock()

	// In a real implementation, this would send heartbeats via RPC
}

// failureDetectorLoop detects failed nodes
func (c *Coordinator) failureDetectorLoop(ctx context.Context) {
	ticker := time.NewTicker(c.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.detectFailures()
		}
	}
}

// detectFailures marks nodes as failed if heartbeat times out
func (c *Coordinator) detectFailures() {
	var failedEvents []MemberEvent

	c.mu.Lock()
	now := time.Now()
	for _, node := range c.members {
		if node.ID == c.localNode.ID {
			continue
		}

		if now.Sub(node.LastHeartbeat) > c.config.FailureTimeout {
			if node.State == NodeStateActive {
				node.State = NodeStateSuspect
			} else if node.State == NodeStateSuspect {
				node.State = NodeStateInactive
				failedEvents = append(failedEvents, MemberEvent{
					Type: MemberEventFailed,
					Node: node,
					Time: now,
				})
			}
		}
	}
	c.mu.Unlock()

	for _, event := range failedEvents {
		c.memberCh <- event
	}
}

// AddMember adds a member to the cluster
func (c *Coordinator) AddMember(node *Node) {
	c.mu.Lock()
	node.State = NodeStateActive
	node.LastHeartbeat = time.Now()
	c.members[node.ID] = node
	c.hashRing.AddNode(node)
	c.mu.Unlock()

	c.shardManager.AddNode(node)

	c.memberCh <- MemberEvent{
		Type: MemberEventJoin,
		Node: node,
		Time: time.Now(),
	}
}

// RemoveMember removes a member from the cluster
func (c *Coordinator) RemoveMember(nodeID string) {
	c.mu.Lock()
	node := c.members[nodeID]
	if node == nil {
		c.mu.Unlock()
		return
	}

	delete(c.members, nodeID)
	c.hashRing.RemoveNode(nodeID)
	c.mu.Unlock()

	c.shardManager.RemoveNode(nodeID)

	c.memberCh <- MemberEvent{
		Type: MemberEventLeave,
		Node: node,
		Time: time.Now(),
	}
}

// GetMembers returns all cluster members
func (c *Coordinator) GetMembers() []*Node {
	c.mu.RLock()
	defer c.mu.RUnlock()

	members := make([]*Node, 0, len(c.members))
	for _, node := range c.members {
		members = append(members, node)
	}
	return members
}

// GetLocalNode returns the local node
func (c *Coordinator) GetLocalNode() *Node {
	return c.localNode
}

// IsLeader returns true if this node is the current cluster leader
func (c *Coordinator) IsLeader() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isLeader
}

// SetLeader sets the leader status and ID for this coordinator
func (c *Coordinator) SetLeader(isLeader bool, leaderID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.isLeader = isLeader
	c.leaderID = leaderID
}

// GetShardForKey returns the shard ID for a key
func (c *Coordinator) GetShardForKey(key string) uint32 {
	return c.shardManager.GetShardForKey(key)
}

// GetNodesForKey returns nodes responsible for a key
func (c *Coordinator) GetNodesForKey(key string) []*Node {
	return c.shardManager.GetNodesForKey(key)
}

// MemberEvents returns a channel of membership events
func (c *Coordinator) MemberEvents() <-chan MemberEvent {
	return c.memberCh
}

// =============================================================================
// Distributed Search
// =============================================================================

// SearchRequest represents a distributed search request
type SearchRequest struct {
	Collection string    `json:"collection"`
	Vector     []float32 `json:"vector"`
	K          int       `json:"k"`
	Filter     string    `json:"filter,omitempty"`
}

// SearchResponse represents a distributed search response
type SearchResponse struct {
	Results []SearchResult `json:"results"`
	Shards  []ShardResult  `json:"shards"`
	TookMs  int64          `json:"took_ms"`
}

// SearchResult represents a single search result
type SearchResult struct {
	ID      string                 `json:"id"`
	Score   float32                `json:"score"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

// ShardResult represents results from a single shard
type ShardResult struct {
	ShardID uint32 `json:"shard_id"`
	NodeID  string `json:"node_id"`
	Count   int    `json:"count"`
	TookMs  int64  `json:"took_ms"`
	Error   string `json:"error,omitempty"`
}

// DistributedSearch performs a search across all shards
func (c *Coordinator) DistributedSearch(ctx context.Context, req *SearchRequest) (*SearchResponse, error) {
	start := time.Now()

	// Get all shards
	shardCount := int(c.shardManager.shardCount)
	results := make(chan shardSearchResult, shardCount)

	// Search each shard in parallel
	var wg sync.WaitGroup
	for shardID := uint32(0); shardID < c.shardManager.shardCount; shardID++ {
		wg.Add(1)
		go func(sid uint32) {
			defer wg.Done()
			result := c.searchShard(ctx, sid, req)
			results <- result
		}(shardID)
	}

	// Wait for all searches to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect and merge results
	var allResults []SearchResult
	var shardResults []ShardResult

	for result := range results {
		shardResults = append(shardResults, ShardResult{
			ShardID: result.ShardID,
			NodeID:  result.NodeID,
			Count:   len(result.Results),
			TookMs:  result.TookMs,
			Error:   result.Error,
		})

		allResults = append(allResults, result.Results...)
	}

	// Sort by score and take top K
	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].Score > allResults[j].Score
	})

	if len(allResults) > req.K {
		allResults = allResults[:req.K]
	}

	return &SearchResponse{
		Results: allResults,
		Shards:  shardResults,
		TookMs:  time.Since(start).Milliseconds(),
	}, nil
}

type shardSearchResult struct {
	ShardID uint32
	NodeID  string
	Results []SearchResult
	TookMs  int64
	Error   string
}

// searchShard searches a single shard
func (c *Coordinator) searchShard(ctx context.Context, shardID uint32, req *SearchRequest) shardSearchResult {
	start := time.Now()

	// Get the primary node for this shard
	node := c.shardManager.GetPrimaryNode(shardID)
	if node == nil {
		return shardSearchResult{
			ShardID: shardID,
			Error:   "no node available for shard",
		}
	}

	// In a real implementation, this would make an RPC call to the node
	// For now, return empty results
	return shardSearchResult{
		ShardID: shardID,
		NodeID:  node.ID,
		Results: []SearchResult{},
		TookMs:  time.Since(start).Milliseconds(),
	}
}

// =============================================================================
// Replication
// =============================================================================

// ReplicationManager handles data replication
type ReplicationManager struct {
	coordinator *Coordinator
	config      *Config
	mu          sync.RWMutex
}

// NewReplicationManager creates a new replication manager
func NewReplicationManager(coordinator *Coordinator, config *Config) *ReplicationManager {
	return &ReplicationManager{
		coordinator: coordinator,
		config:      config,
	}
}

// ReplicateWrite replicates a write to replica nodes
func (rm *ReplicationManager) ReplicateWrite(ctx context.Context, key string, data []byte) error {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	nodes := rm.coordinator.GetNodesForKey(key)
	if len(nodes) < 2 {
		return nil // No replicas needed
	}

	// Skip first node (primary) and replicate to others
	var errs []error
	for _, node := range nodes[1:] {
		if err := rm.sendToNode(ctx, node, data); err != nil {
			errs = append(errs, err)
		}
	}

	// Require majority acknowledgment
	successCount := len(nodes) - 1 - len(errs)
	requiredAcks := (len(nodes) + 1) / 2

	if successCount < requiredAcks {
		return errors.New("insufficient replica acknowledgments")
	}

	return nil
}

// sendToNode sends data to a replica node
func (rm *ReplicationManager) sendToNode(ctx context.Context, node *Node, data []byte) error {
	// In a real implementation, this would make an RPC call
	return nil
}

// =============================================================================
// Cluster State Serialization
// =============================================================================

// ClusterState represents the serializable cluster state
type ClusterState struct {
	Version   uint64            `json:"version"`
	Members   []*Node           `json:"members"`
	Shards    map[uint32]*Shard `json:"shards"`
	LeaderID  string            `json:"leader_id"`
	Timestamp time.Time         `json:"timestamp"`
}

// GetState returns the current cluster state
func (c *Coordinator) GetState() *ClusterState {
	c.mu.RLock()
	members := make([]*Node, 0, len(c.members))
	for _, node := range c.members {
		members = append(members, node)
	}
	leaderID := c.leaderID
	c.mu.RUnlock()

	shards := c.shardManager.GetShards()

	return &ClusterState{
		Version:   1,
		Members:   members,
		Shards:    shards,
		LeaderID:  leaderID,
		Timestamp: time.Now(),
	}
}

// EncodeState encodes cluster state to JSON
func (cs *ClusterState) Encode() ([]byte, error) {
	return json.Marshal(cs)
}

// DecodeState decodes cluster state from JSON
func DecodeState(data []byte) (*ClusterState, error) {
	var state ClusterState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}
