package cluster

import (
	"context"
	"testing"
	"time"
)

func TestHashRing(t *testing.T) {
	ring := NewHashRing(100)

	// Add nodes
	node1 := &Node{ID: "node1", Address: "localhost:8001", State: NodeStateActive}
	node2 := &Node{ID: "node2", Address: "localhost:8002", State: NodeStateActive}
	node3 := &Node{ID: "node3", Address: "localhost:8003", State: NodeStateActive}

	ring.AddNode(node1)
	ring.AddNode(node2)
	ring.AddNode(node3)

	// Test GetNode - same key should return same node
	key := "test-key"
	node := ring.GetNode(key)
	if node == nil {
		t.Fatal("Expected to get a node")
	}

	// Same key should return same node
	node2Result := ring.GetNode(key)
	if node.ID != node2Result.ID {
		t.Error("Same key should return same node")
	}

	// Different keys should be distributed
	nodes := make(map[string]int)
	for i := 0; i < 1000; i++ {
		n := ring.GetNode(string(rune(i)))
		if n != nil {
			nodes[n.ID]++
		}
	}

	// All nodes should have some keys
	if len(nodes) != 3 {
		t.Errorf("Expected 3 nodes to have keys, got %d", len(nodes))
	}
}

func TestHashRingGetNodes(t *testing.T) {
	ring := NewHashRing(100)

	node1 := &Node{ID: "node1", Address: "localhost:8001", State: NodeStateActive}
	node2 := &Node{ID: "node2", Address: "localhost:8002", State: NodeStateActive}
	node3 := &Node{ID: "node3", Address: "localhost:8003", State: NodeStateActive}

	ring.AddNode(node1)
	ring.AddNode(node2)
	ring.AddNode(node3)

	// Get 2 nodes for replication
	nodes := ring.GetNodes("test-key", 2)
	if len(nodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(nodes))
	}

	// Nodes should be unique
	if nodes[0].ID == nodes[1].ID {
		t.Error("Nodes should be unique")
	}
}

func TestHashRingRemoveNode(t *testing.T) {
	ring := NewHashRing(100)

	node1 := &Node{ID: "node1", Address: "localhost:8001", State: NodeStateActive}
	node2 := &Node{ID: "node2", Address: "localhost:8002", State: NodeStateActive}

	ring.AddNode(node1)
	ring.AddNode(node2)

	// Remove node1
	ring.RemoveNode("node1")

	// All keys should now go to node2
	for i := 0; i < 100; i++ {
		node := ring.GetNode(string(rune(i)))
		if node != nil && node.ID != "node2" {
			t.Errorf("After removal, all keys should go to node2, got %s", node.ID)
		}
	}
}

func TestShardManager(t *testing.T) {
	sm := NewShardManager(16, 2) // 16 shards, replication factor 2

	// Add nodes
	node1 := &Node{ID: "node1", Address: "localhost:8001", State: NodeStateActive}
	node2 := &Node{ID: "node2", Address: "localhost:8002", State: NodeStateActive}
	node3 := &Node{ID: "node3", Address: "localhost:8003", State: NodeStateActive}

	sm.AddNode(node1)
	sm.AddNode(node2)
	sm.AddNode(node3)

	// Test GetShardForKey
	shardID := sm.GetShardForKey("test-key")
	if shardID >= 16 {
		t.Errorf("Shard ID should be < 16, got %d", shardID)
	}

	// Same key should return same shard
	shardID2 := sm.GetShardForKey("test-key")
	if shardID != shardID2 {
		t.Error("Same key should return same shard")
	}

	// Get shard
	shard := sm.GetShard(shardID)
	if shard == nil {
		t.Fatal("Expected to get shard")
	}
	if shard.Primary == "" {
		t.Error("Shard should have a primary node")
	}
}

func TestCoordinator(t *testing.T) {
	config := &Config{
		NodeID:            "node1",
		AdvertiseAddr:     "localhost:8001",
		ReplicationFactor: 2,
		ShardCount:        16,
		HeartbeatInterval: 100 * time.Millisecond,
		FailureTimeout:    500 * time.Millisecond,
	}

	coordinator := NewCoordinator(config)

	// Local node should be added
	members := coordinator.GetMembers()
	if len(members) != 1 {
		t.Errorf("Expected 1 member, got %d", len(members))
	}

	// Add another member
	node2 := &Node{
		ID:      "node2",
		Address: "localhost:8002",
		State:   NodeStateActive,
	}
	coordinator.AddMember(node2)

	members = coordinator.GetMembers()
	if len(members) != 2 {
		t.Errorf("Expected 2 members, got %d", len(members))
	}

	// Remove member
	coordinator.RemoveMember("node2")
	members = coordinator.GetMembers()
	if len(members) != 1 {
		t.Errorf("Expected 1 member after removal, got %d", len(members))
	}
}

func TestDistributedSearch(t *testing.T) {
	config := &Config{
		NodeID:            "node1",
		AdvertiseAddr:     "localhost:8001",
		ReplicationFactor: 2,
		ShardCount:        4,
		HeartbeatInterval: 100 * time.Millisecond,
		FailureTimeout:    500 * time.Millisecond,
	}

	coordinator := NewCoordinator(config)

	// Add another node
	node2 := &Node{ID: "node2", Address: "localhost:8002", State: NodeStateActive}
	coordinator.AddMember(node2)

	// Perform distributed search
	ctx := context.Background()
	req := &SearchRequest{
		Collection: "test",
		Vector:     []float32{1.0, 2.0, 3.0},
		K:          10,
	}

	resp, err := coordinator.DistributedSearch(ctx, req)
	if err != nil {
		t.Fatalf("DistributedSearch failed: %v", err)
	}

	// Should have results from all shards
	if len(resp.Shards) != 4 {
		t.Errorf("Expected 4 shard results, got %d", len(resp.Shards))
	}
}

func TestClusterState(t *testing.T) {
	config := &Config{
		NodeID:            "node1",
		AdvertiseAddr:     "localhost:8001",
		ReplicationFactor: 2,
		ShardCount:        8,
		HeartbeatInterval: 100 * time.Millisecond,
		FailureTimeout:    500 * time.Millisecond,
	}

	coordinator := NewCoordinator(config)

	// Get state
	state := coordinator.GetState()
	if state == nil {
		t.Fatal("Expected non-nil state")
	}

	if len(state.Members) != 1 {
		t.Errorf("Expected 1 member, got %d", len(state.Members))
	}

	if len(state.Shards) != 8 {
		t.Errorf("Expected 8 shards, got %d", len(state.Shards))
	}

	// Encode/decode
	encoded, err := state.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decoded, err := DecodeState(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if len(decoded.Members) != len(state.Members) {
		t.Error("Member count mismatch after decode")
	}
}

func TestNodeHealth(t *testing.T) {
	node := &Node{
		ID:      "node1",
		Address: "localhost:8001",
		State:   NodeStateActive,
	}

	if !node.IsHealthy() {
		t.Error("Active node should be healthy")
	}

	node.State = NodeStateSuspect
	if node.IsHealthy() {
		t.Error("Suspect node should not be healthy")
	}

	node.State = NodeStateInactive
	if node.IsHealthy() {
		t.Error("Inactive node should not be healthy")
	}
}

func BenchmarkHashRingGetNode(b *testing.B) {
	ring := NewHashRing(150)

	for i := 0; i < 10; i++ {
		node := &Node{
			ID:      string(rune('A' + i)),
			Address: "localhost:800" + string(rune('0'+i)),
			State:   NodeStateActive,
		}
		ring.AddNode(node)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ring.GetNode("test-key-" + string(rune(i%1000)))
	}
}

func BenchmarkShardManagerGetShardForKey(b *testing.B) {
	sm := NewShardManager(64, 3)

	for i := 0; i < 10; i++ {
		node := &Node{
			ID:      string(rune('A' + i)),
			Address: "localhost:800" + string(rune('0'+i)),
			State:   NodeStateActive,
		}
		sm.AddNode(node)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sm.GetShardForKey("test-key-" + string(rune(i%1000)))
	}
}
