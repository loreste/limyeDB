package cluster

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func testCoordinator() *Coordinator {
	cfg := &Config{
		NodeID:            "local-test",
		ListenAddr:        "127.0.0.1:7000",
		AdvertiseAddr:     "127.0.0.1:7000",
		ReplicationFactor: 2,
		ShardCount:        16,
		HeartbeatInterval: 1 * time.Second,
		FailureTimeout:    5 * time.Second,
	}
	return NewCoordinator(cfg)
}

// TestRaceAddRemoveMember hammers AddMember and RemoveMember concurrently.
func TestRaceAddRemoveMember(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	coord := testCoordinator()

	const goroutines = 20
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				nodeID := fmt.Sprintf("node-%d-%d", id, i)
				node := &Node{
					ID:       nodeID,
					Address:  fmt.Sprintf("127.0.0.1:%d", 8000+id*1000+i),
					State:    NodeStateActive,
					Metadata: make(map[string]string),
				}
				coord.AddMember(node)
				// Drain the event channel to avoid blocking
				select {
				case <-coord.MemberEvents():
				default:
				}
				coord.RemoveMember(nodeID)
				select {
				case <-coord.MemberEvents():
				default:
				}
			}
		}(g)
	}
	wg.Wait()
}

// TestRaceGetStateWhileMutating reads GetState while members are being added/removed.
func TestRaceGetStateWhileMutating(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	coord := testCoordinator()

	const goroutines = 10
	const ops = 200

	var wg sync.WaitGroup

	// Drain event channel to prevent blocking
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-coord.MemberEvents():
			case <-done:
				return
			}
		}
	}()

	// Writers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				nodeID := fmt.Sprintf("state-node-%d-%d", id, i)
				node := &Node{
					ID:       nodeID,
					Address:  fmt.Sprintf("127.0.0.1:%d", 9000+id*1000+i),
					State:    NodeStateActive,
					Metadata: make(map[string]string),
				}
				coord.AddMember(node)
				coord.RemoveMember(nodeID)
			}
		}(g)
	}

	// State readers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops*2; i++ {
				state := coord.GetState()
				if state == nil {
					t.Error("GetState returned nil")
					return
				}
				// Exercise serialization
				_, _ = state.Encode()
			}
		}()
	}

	// GetMembers readers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops*2; i++ {
				_ = coord.GetMembers()
				_ = coord.IsLeader()
			}
		}()
	}

	wg.Wait()
	close(done)
}

// TestRaceSetLeaderWhileReading tests SetLeader and IsLeader concurrently.
func TestRaceSetLeaderWhileReading(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	coord := testCoordinator()

	const goroutines = 20
	const ops = 500

	var wg sync.WaitGroup

	// Leader writers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				isLeader := (i % 2) == 0
				coord.SetLeader(isLeader, fmt.Sprintf("leader-%d-%d", id, i))
			}
		}(g)
	}

	// Leader readers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				_ = coord.IsLeader()
				_ = coord.GetLocalNode()
			}
		}()
	}

	wg.Wait()
}

// TestRaceHashRing hammers AddNode, RemoveNode, GetNode, and GetNodes on the HashRing.
func TestRaceHashRing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	hr := NewHashRing(10)

	const goroutines = 10
	const ops = 100

	var wg sync.WaitGroup

	// Add nodes
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				node := &Node{
					ID:       fmt.Sprintf("hr-node-%d-%d", id, i),
					Address:  fmt.Sprintf("127.0.0.1:%d", 10000+id*1000+i),
					State:    NodeStateActive,
					Metadata: make(map[string]string),
				}
				hr.AddNode(node)
			}
		}(g)
	}

	// Remove nodes
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				hr.RemoveNode(fmt.Sprintf("hr-node-%d-%d", id, i))
			}
		}(g)
	}

	// Lookups
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				_ = hr.GetNode(fmt.Sprintf("key-%d-%d", id, i))
				_ = hr.GetNodes(fmt.Sprintf("key-%d-%d", id, i), 3)
				_ = hr.GetAllNodes()
			}
		}(g)
	}

	wg.Wait()
}

// TestRaceShardManager hammers AddNode, RemoveNode, and shard lookups.
func TestRaceShardManager(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	sm := NewShardManager(16, 2)

	const goroutines = 10
	const ops = 200

	var wg sync.WaitGroup

	// Add/Remove nodes
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				nodeID := fmt.Sprintf("sm-node-%d-%d", id, i)
				node := &Node{
					ID:       nodeID,
					Address:  fmt.Sprintf("127.0.0.1:%d", 11000+id*1000+i),
					State:    NodeStateActive,
					Metadata: make(map[string]string),
				}
				sm.AddNode(node)
				sm.RemoveNode(nodeID)
			}
		}(g)
	}

	// Shard lookups
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				key := fmt.Sprintf("data-%d-%d", id, i)
				_ = sm.GetShardForKey(key)
				_ = sm.GetNodesForKey(key)
				_ = sm.GetShard(uint32(i % 16))
				_ = sm.GetShards()
			}
		}(g)
	}

	wg.Wait()
}

// TestRaceCoordinatorShardLookup exercises GetShardForKey and GetNodesForKey with membership changes.
func TestRaceCoordinatorShardLookup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	coord := testCoordinator()

	// Pre-add some members
	for i := 0; i < 5; i++ {
		coord.AddMember(&Node{
			ID:       fmt.Sprintf("pre-node-%d", i),
			Address:  fmt.Sprintf("127.0.0.1:%d", 12000+i),
			State:    NodeStateActive,
			Metadata: make(map[string]string),
		})
	}

	// Drain events
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-coord.MemberEvents():
			case <-done:
				return
			}
		}
	}()

	const goroutines = 10
	const ops = 500

	var wg sync.WaitGroup

	// Shard/node lookups
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				key := fmt.Sprintf("lookup-%d-%d", id, i)
				_ = coord.GetShardForKey(key)
				_ = coord.GetNodesForKey(key)
			}
		}(g)
	}

	// Concurrent membership mutations
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops/5; i++ {
				nodeID := fmt.Sprintf("dyn-node-%d-%d", id, i)
				coord.AddMember(&Node{
					ID:       nodeID,
					Address:  fmt.Sprintf("127.0.0.1:%d", 13000+id*1000+i),
					State:    NodeStateActive,
					Metadata: make(map[string]string),
				})
				coord.RemoveMember(nodeID)
			}
		}(g)
	}

	wg.Wait()
	close(done)
}
