package raft

import (
	"sync"
	"testing"
	"time"
)

// MockTransport implements Transport for testing
type MockTransport struct {
	nodes map[string]*Node
	mu    sync.Mutex
}

func NewMockTransport() *MockTransport {
	return &MockTransport{
		nodes: make(map[string]*Node),
	}
}

func (m *MockTransport) RegisterNode(node *Node) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodes[node.ID] = node
}

func (m *MockTransport) SendRequestVote(target string, req *RequestVoteRequest) (*RequestVoteResponse, error) {
	m.mu.Lock()
	node, ok := m.nodes[target]
	m.mu.Unlock()

	if !ok {
		return &RequestVoteResponse{Term: 0, VoteGranted: false}, nil
	}

	return node.HandleRequestVote(req), nil
}

func (m *MockTransport) SendAppendEntries(target string, req *AppendEntriesRequest) (*AppendEntriesResponse, error) {
	m.mu.Lock()
	node, ok := m.nodes[target]
	m.mu.Unlock()

	if !ok {
		return &AppendEntriesResponse{Term: 0, Success: false}, nil
	}

	return node.HandleAppendEntries(req), nil
}

// MockStateMachine implements StateMachine for testing
type MockStateMachine struct {
	commands []*Command
	mu       sync.Mutex
}

func NewMockStateMachine() *MockStateMachine {
	return &MockStateMachine{
		commands: make([]*Command, 0),
	}
}

func (m *MockStateMachine) Apply(cmd *Command) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commands = append(m.commands, cmd)
	return nil
}

func (m *MockStateMachine) Snapshot() ([]byte, error) {
	return nil, nil
}

func (m *MockStateMachine) Restore(data []byte) error {
	return nil
}

func (m *MockStateMachine) CommandCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.commands)
}

func TestRaftNodeCreation(t *testing.T) {
	transport := NewMockTransport()
	sm := NewMockStateMachine()

	cfg := &Config{
		ID:                "node1",
		Address:           "localhost:8001",
		Peers:             []string{"node2", "node3"},
		ElectionTimeout:   150 * time.Millisecond,
		HeartbeatInterval: 50 * time.Millisecond,
		Transport:         transport,
		StateMachine:      sm,
	}

	node, err := NewNode(cfg)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	if node.ID != "node1" {
		t.Errorf("Expected ID 'node1', got '%s'", node.ID)
	}

	if node.GetState() != Follower {
		t.Errorf("Expected initial state Follower, got %v", node.GetState())
	}
}

func TestRequestVoteHandling(t *testing.T) {
	transport := NewMockTransport()
	sm := NewMockStateMachine()

	cfg := &Config{
		ID:                "node1",
		Address:           "localhost:8001",
		Peers:             []string{},
		ElectionTimeout:   150 * time.Millisecond,
		HeartbeatInterval: 50 * time.Millisecond,
		Transport:         transport,
		StateMachine:      sm,
	}

	node, err := NewNode(cfg)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	// Request vote from a candidate with higher term
	req := &RequestVoteRequest{
		Term:         1,
		CandidateID:  "node2",
		LastLogIndex: 0,
		LastLogTerm:  0,
	}

	resp := node.HandleRequestVote(req)

	if !resp.VoteGranted {
		t.Error("Expected vote to be granted")
	}

	if resp.Term != 1 {
		t.Errorf("Expected term 1, got %d", resp.Term)
	}

	// Second request should be denied (already voted)
	req2 := &RequestVoteRequest{
		Term:         1,
		CandidateID:  "node3",
		LastLogIndex: 0,
		LastLogTerm:  0,
	}

	resp2 := node.HandleRequestVote(req2)

	if resp2.VoteGranted {
		t.Error("Expected vote to be denied (already voted for node2)")
	}
}

func TestAppendEntriesHandling(t *testing.T) {
	transport := NewMockTransport()
	sm := NewMockStateMachine()

	cfg := &Config{
		ID:                "node1",
		Address:           "localhost:8001",
		Peers:             []string{},
		ElectionTimeout:   150 * time.Millisecond,
		HeartbeatInterval: 50 * time.Millisecond,
		Transport:         transport,
		StateMachine:      sm,
	}

	node, err := NewNode(cfg)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	// Heartbeat from leader
	req := &AppendEntriesRequest{
		Term:         1,
		LeaderID:     "leader1",
		PrevLogIndex: 0,
		PrevLogTerm:  0,
		Entries:      nil,
		LeaderCommit: 0,
	}

	resp := node.HandleAppendEntries(req)

	if !resp.Success {
		t.Error("Expected append entries to succeed")
	}

	if node.GetLeaderID() != "leader1" {
		t.Errorf("Expected leader ID 'leader1', got '%s'", node.GetLeaderID())
	}

	// Append some entries
	req2 := &AppendEntriesRequest{
		Term:         1,
		LeaderID:     "leader1",
		PrevLogIndex: 0,
		PrevLogTerm:  0,
		Entries: []LogEntry{
			{Term: 1, Index: 1, Command: &Command{Type: CommandInsert, Collection: "test"}},
		},
		LeaderCommit: 1,
	}

	resp2 := node.HandleAppendEntries(req2)

	if !resp2.Success {
		t.Error("Expected append entries with log entry to succeed")
	}
}

func TestNodeInfo(t *testing.T) {
	transport := NewMockTransport()
	sm := NewMockStateMachine()

	cfg := &Config{
		ID:                "node1",
		Address:           "localhost:8001",
		Peers:             []string{"node2", "node3"},
		ElectionTimeout:   150 * time.Millisecond,
		HeartbeatInterval: 50 * time.Millisecond,
		Transport:         transport,
		StateMachine:      sm,
	}

	node, err := NewNode(cfg)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	info := node.GetInfo()

	if info.ID != "node1" {
		t.Errorf("Expected ID 'node1', got '%s'", info.ID)
	}

	if info.State != "follower" {
		t.Errorf("Expected state 'follower', got '%s'", info.State)
	}

	if info.IsLeader {
		t.Error("Node should not be leader initially")
	}
}

func TestTermComparison(t *testing.T) {
	transport := NewMockTransport()
	sm := NewMockStateMachine()

	cfg := &Config{
		ID:                "node1",
		Address:           "localhost:8001",
		Peers:             []string{},
		ElectionTimeout:   150 * time.Millisecond,
		HeartbeatInterval: 50 * time.Millisecond,
		Transport:         transport,
		StateMachine:      sm,
	}

	node, err := NewNode(cfg)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	// Set node's term to 5
	node.HandleAppendEntries(&AppendEntriesRequest{
		Term:     5,
		LeaderID: "leader",
	})

	// Request vote with lower term should be rejected
	req := &RequestVoteRequest{
		Term:         3,
		CandidateID:  "node2",
		LastLogIndex: 0,
		LastLogTerm:  0,
	}

	resp := node.HandleRequestVote(req)

	if resp.VoteGranted {
		t.Error("Vote should be rejected for lower term")
	}

	if resp.Term != 5 {
		t.Errorf("Response should contain current term 5, got %d", resp.Term)
	}
}
