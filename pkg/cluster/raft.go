package cluster

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"

	"github.com/limyedb/limyedb/pkg/collection"
	"github.com/limyedb/limyedb/pkg/storage/snapshot"
)

// RaftNode wraps the Raft backend for LimyeDB
type RaftNode struct {
	Raft           *raft.Raft
	FSM            *FSM
	manager        *collection.Manager
	LeaderRestAddr string
	mu             sync.RWMutex
	leaderCh       chan bool // leadership notification channel, closed on shutdown
}

// RaftConfig represents Raft configuration
type RaftConfig struct {
	NodeID   string
	BindAddr string
	DataDir  string
	IsLeader bool   // True if bootstrapping cluster initially
	RestAddr string // This node's exposed HTTP listening address
}

// NewRaftNode initializes a completely new Raft instance binding to the collection manager.
func NewRaftNode(cfg *RaftConfig, manager *collection.Manager, snapMgr *snapshot.Manager) (*RaftNode, error) {
	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(cfg.NodeID)

	addr, err := net.ResolveTCPAddr("tcp", cfg.BindAddr)
	if err != nil {
		return nil, err
	}

	transport, err := raft.NewTCPTransport(cfg.BindAddr, addr, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(cfg.DataDir, 0750); err != nil {
		return nil, err
	}

	logStore, err := raftboltdb.NewBoltStore(filepath.Join(cfg.DataDir, "raft-log.bolt"))
	if err != nil {
		return nil, err
	}

	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(cfg.DataDir, "raft-stable.bolt"))
	if err != nil {
		return nil, err
	}

	snapshotStore, err := raft.NewFileSnapshotStore(cfg.DataDir, 3, os.Stderr)
	if err != nil {
		return nil, err
	}

	fsm := NewFSM(manager, snapMgr)

	leaderCh := make(chan bool, 1)
	raftConfig.NotifyCh = leaderCh

	r, err := raft.NewRaft(raftConfig, fsm, logStore, stableStore, snapshotStore, transport)
	if err != nil {
		return nil, err
	}

	node := &RaftNode{
		Raft:     r,
		FSM:      fsm,
		manager:  manager,
		leaderCh: leaderCh,
	}

	fsm.SetRaftNode(node)

	// Broadcast leadership asynchronously
	go func() {
		for isLeader := range leaderCh {
			if isLeader {
				// Allow Raft consensus to settle prior to inserting the configuration
				time.Sleep(1 * time.Second)
				_ = node.Write(OpSetLeaderRest, SetLeaderRestData{RestAddr: cfg.RestAddr})
			}
		}
	}()

	if cfg.IsLeader {
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      raftConfig.LocalID,
					Address: transport.LocalAddr(),
				},
			},
		}
		r.BootstrapCluster(configuration)
	}

	return node, nil
}

// GetLeaderRestAddr safely returns the cluster's active HTTP endpoint
func (n *RaftNode) GetLeaderRestAddr() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.LeaderRestAddr
}

// SetLeaderRestAddr securely mutates the HTTP discovery tracker
func (n *RaftNode) SetLeaderRestAddr(addr string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.LeaderRestAddr = addr
}

// Write appends a JSON-encoded struct log into the Raft replication sequence
func (n *RaftNode) Write(op OpType, data interface{}) error {
	if n.Raft.State() != raft.Leader {
		return fmt.Errorf("not the leader; leader is %s", n.Raft.Leader())
	}

	cmdData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	cmd := Command{
		Op:   op,
		Data: cmdData,
	}

	b, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	applyFuture := n.Raft.Apply(b, 10*time.Second)
	if err := applyFuture.Error(); err != nil {
		return err
	}

	return nil
}

// Shutdown gracefully shuts down the Raft node and stops the leadership broadcast goroutine.
func (n *RaftNode) Shutdown() error {
	f := n.Raft.Shutdown()
	// Raft shutdown closes the NotifyCh (leaderCh), which causes the broadcast goroutine to exit.
	return f.Error()
}

// Join adds a new voter node to the distributed cluster
func (n *RaftNode) Join(nodeID string, addr string) error {
	configFuture := n.Raft.GetConfiguration()
	if err := configFuture.Error(); err != nil {
		return err
	}

	for _, srv := range configFuture.Configuration().Servers {
		if srv.ID == raft.ServerID(nodeID) || srv.Address == raft.ServerAddress(addr) {
			if srv.Address == raft.ServerAddress(addr) && srv.ID == raft.ServerID(nodeID) {
				return nil // already joined
			}
			future := n.Raft.RemoveServer(srv.ID, 0, 0)
			if err := future.Error(); err != nil {
				return err
			}
		}
	}

	f := n.Raft.AddVoter(raft.ServerID(nodeID), raft.ServerAddress(addr), 0, 0)
	return f.Error()
}
