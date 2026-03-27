package raft

import (
	cryptorand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// NodeState represents the state of a Raft node
type NodeState int

const (
	Follower NodeState = iota
	Candidate
	Leader
)

func (s NodeState) String() string {
	switch s {
	case Follower:
		return "follower"
	case Candidate:
		return "candidate"
	case Leader:
		return "leader"
	default:
		return "unknown"
	}
}

// LogEntry represents an entry in the Raft log
type LogEntry struct {
	Term    uint64      `json:"term"`
	Index   uint64      `json:"index"`
	Command interface{} `json:"command"`
}

// CommandType represents the type of command
type CommandType string

const (
	CommandInsert     CommandType = "insert"
	CommandDelete     CommandType = "delete"
	CommandUpdate     CommandType = "update"
	CommandCreateColl CommandType = "create_collection"
	CommandDeleteColl CommandType = "delete_collection"
)

// Command represents a replicated command
type Command struct {
	Type       CommandType `json:"type"`
	Collection string      `json:"collection"`
	Data       []byte      `json:"data"`
}

// Node represents a Raft node
type Node struct {
	// Node identity
	ID      string
	Address string

	// Persistent state
	currentTerm uint64
	votedFor    string
	log         []LogEntry

	// Volatile state
	commitIndex uint64
	lastApplied uint64

	// Leader state
	nextIndex  map[string]uint64
	matchIndex map[string]uint64

	// Node state
	state          NodeState
	leaderId       string
	lastHeartbeat  time.Time

	// Cluster configuration
	peers []string

	// Communication
	transport Transport

	// State machine
	stateMachine StateMachine

	// Timing
	electionTimeout time.Duration
	heartbeatInterval time.Duration

	// Synchronization
	mu           sync.RWMutex
	stopCh       chan struct{}
	applyCh      chan LogEntry
	commitCh     chan struct{}

	// (removed math/rand in favor of crypto/rand helpers)
}

// Transport defines the interface for network communication
type Transport interface {
	SendRequestVote(target string, req *RequestVoteRequest) (*RequestVoteResponse, error)
	SendAppendEntries(target string, req *AppendEntriesRequest) (*AppendEntriesResponse, error)
}

// StateMachine defines the interface for the replicated state machine
type StateMachine interface {
	Apply(cmd *Command) error
	Snapshot() ([]byte, error)
	Restore(data []byte) error
}

// RequestVoteRequest is the RequestVote RPC request
type RequestVoteRequest struct {
	Term         uint64 `json:"term"`
	CandidateID  string `json:"candidate_id"`
	LastLogIndex uint64 `json:"last_log_index"`
	LastLogTerm  uint64 `json:"last_log_term"`
}

// RequestVoteResponse is the RequestVote RPC response
type RequestVoteResponse struct {
	Term        uint64 `json:"term"`
	VoteGranted bool   `json:"vote_granted"`
}

// AppendEntriesRequest is the AppendEntries RPC request
type AppendEntriesRequest struct {
	Term         uint64     `json:"term"`
	LeaderID     string     `json:"leader_id"`
	PrevLogIndex uint64     `json:"prev_log_index"`
	PrevLogTerm  uint64     `json:"prev_log_term"`
	Entries      []LogEntry `json:"entries"`
	LeaderCommit uint64     `json:"leader_commit"`
}

// AppendEntriesResponse is the AppendEntries RPC response
type AppendEntriesResponse struct {
	Term    uint64 `json:"term"`
	Success bool   `json:"success"`
}

// Config holds Raft configuration
type Config struct {
	ID                string
	Address           string
	Peers             []string
	ElectionTimeout   time.Duration
	HeartbeatInterval time.Duration
	Transport         Transport
	StateMachine      StateMachine
}

// DefaultConfig returns default Raft configuration
func DefaultConfig() *Config {
	return &Config{
		ElectionTimeout:   150 * time.Millisecond,
		HeartbeatInterval: 50 * time.Millisecond,
	}
}

// NewNode creates a new Raft node
func NewNode(cfg *Config) (*Node, error) {
	if cfg.ID == "" {
		return nil, errors.New("node ID required")
	}
	if cfg.Transport == nil {
		return nil, errors.New("transport required")
	}
	if cfg.StateMachine == nil {
		return nil, errors.New("state machine required")
	}

	n := &Node{
		ID:                cfg.ID,
		Address:           cfg.Address,
		peers:             cfg.Peers,
		transport:         cfg.Transport,
		stateMachine:      cfg.StateMachine,
		electionTimeout:   cfg.ElectionTimeout,
		heartbeatInterval: cfg.HeartbeatInterval,
		state:             Follower,
		log:               make([]LogEntry, 0),
		nextIndex:         make(map[string]uint64),
		matchIndex:        make(map[string]uint64),
		stopCh:            make(chan struct{}),
		applyCh:           make(chan LogEntry, 100),
		commitCh:          make(chan struct{}, 1),
	}

	return n, nil
}

// cryptoRandInt63n returns a cryptographically secure random int64 in [0, n).
func cryptoRandInt63n(n int64) int64 {
	if n <= 0 {
		return 0
	}
	v, err := cryptorand.Int(cryptorand.Reader, big.NewInt(n))
	if err != nil {
		return 0
	}
	return v.Int64()
}

// Start starts the Raft node
func (n *Node) Start() error {
	n.mu.Lock()
	n.lastHeartbeat = time.Now()
	n.mu.Unlock()

	// Start background goroutines
	go n.runElectionTimer()
	go n.runApplier()

	return nil
}

// Stop stops the Raft node
func (n *Node) Stop() error {
	close(n.stopCh)
	return nil
}

// runElectionTimer runs the election timeout timer
func (n *Node) runElectionTimer() {
	for {
		select {
		case <-n.stopCh:
			return
		default:
		}

		n.mu.RLock()
		state := n.state
		lastHeartbeat := n.lastHeartbeat
		timeout := n.electionTimeout + time.Duration(cryptoRandInt63n(int64(n.electionTimeout)))
		n.mu.RUnlock()

		if state == Leader {
			// Leaders don't need election timeout
			time.Sleep(n.heartbeatInterval)
			n.sendHeartbeats()
		} else if time.Since(lastHeartbeat) > timeout {
			// Start election
			n.startElection()
		} else {
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// startElection starts an election for this node
func (n *Node) startElection() {
	n.mu.Lock()
	n.state = Candidate
	n.currentTerm++
	n.votedFor = n.ID
	term := n.currentTerm
	lastLogIndex := uint64(len(n.log))
	var lastLogTerm uint64
	if lastLogIndex > 0 {
		lastLogTerm = n.log[lastLogIndex-1].Term
	}
	peers := n.peers
	n.mu.Unlock()

	votes := 1 // Vote for self
	votesNeeded := (len(peers)+1)/2 + 1

	var voteMu sync.Mutex
	var wg sync.WaitGroup

	for _, peer := range peers {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()

			req := &RequestVoteRequest{
				Term:         term,
				CandidateID:  n.ID,
				LastLogIndex: lastLogIndex,
				LastLogTerm:  lastLogTerm,
			}

			resp, err := n.transport.SendRequestVote(p, req)
			if err != nil {
				return
			}

			n.mu.Lock()
			if resp.Term > n.currentTerm {
				n.currentTerm = resp.Term
				n.state = Follower
				n.votedFor = ""
				n.mu.Unlock()
				return
			}
			n.mu.Unlock()

			if resp.VoteGranted {
				voteMu.Lock()
				votes++
				voteMu.Unlock()
			}
		}(peer)
	}

	wg.Wait()

	voteMu.Lock()
	wonElection := votes >= votesNeeded
	voteMu.Unlock()

	n.mu.Lock()
	if wonElection && n.state == Candidate && n.currentTerm == term {
		n.state = Leader
		n.leaderId = n.ID

		// Initialize leader state
		lastIndex := uint64(len(n.log))
		for _, peer := range n.peers {
			n.nextIndex[peer] = lastIndex + 1
			n.matchIndex[peer] = 0
		}
	}
	n.mu.Unlock()
}

// sendHeartbeats sends heartbeat AppendEntries to all peers
func (n *Node) sendHeartbeats() {
	n.mu.RLock()
	if n.state != Leader {
		n.mu.RUnlock()
		return
	}
	term := n.currentTerm
	peers := n.peers
	n.mu.RUnlock()

	for _, peer := range peers {
		go n.sendAppendEntries(peer, term)
	}
}

// sendAppendEntries sends AppendEntries RPC to a peer
func (n *Node) sendAppendEntries(peer string, term uint64) {
	n.mu.RLock()
	nextIdx := n.nextIndex[peer]
	prevLogIndex := nextIdx - 1
	var prevLogTerm uint64
	if prevLogIndex > 0 && prevLogIndex <= uint64(len(n.log)) {
		prevLogTerm = n.log[prevLogIndex-1].Term
	}

	var entries []LogEntry
	if nextIdx <= uint64(len(n.log)) {
		entries = n.log[nextIdx-1:]
	}

	req := &AppendEntriesRequest{
		Term:         term,
		LeaderID:     n.ID,
		PrevLogIndex: prevLogIndex,
		PrevLogTerm:  prevLogTerm,
		Entries:      entries,
		LeaderCommit: n.commitIndex,
	}
	n.mu.RUnlock()

	resp, err := n.transport.SendAppendEntries(peer, req)
	if err != nil {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if resp.Term > n.currentTerm {
		n.currentTerm = resp.Term
		n.state = Follower
		n.votedFor = ""
		return
	}

	if resp.Success {
		n.nextIndex[peer] = nextIdx + uint64(len(entries))
		n.matchIndex[peer] = n.nextIndex[peer] - 1
		n.updateCommitIndex()
	} else {
		// Decrement nextIndex and retry
		if n.nextIndex[peer] > 1 {
			n.nextIndex[peer]--
		}
	}
}

// updateCommitIndex updates the commit index based on match indexes
func (n *Node) updateCommitIndex() {
	// Find the highest index replicated on a majority
	for i := uint64(len(n.log)); i > n.commitIndex; i-- {
		if n.log[i-1].Term != n.currentTerm {
			continue
		}

		count := 1 // Count self
		for _, peer := range n.peers {
			if n.matchIndex[peer] >= i {
				count++
			}
		}

		if count > (len(n.peers)+1)/2 {
			n.commitIndex = i
			select {
			case n.commitCh <- struct{}{}:
			default:
			}
			break
		}
	}
}

// runApplier applies committed entries to the state machine
func (n *Node) runApplier() {
	for {
		select {
		case <-n.stopCh:
			return
		case <-n.commitCh:
			n.applyCommitted()
		}
	}
}

// applyCommitted applies all committed but unapplied entries
func (n *Node) applyCommitted() {
	n.mu.Lock()
	toApply := make([]LogEntry, 0)
	for n.lastApplied < n.commitIndex {
		n.lastApplied++
		toApply = append(toApply, n.log[n.lastApplied-1])
	}
	n.mu.Unlock()

	for _, entry := range toApply {
		if entry.Command == nil {
			continue
		}
		cmdBytes, err := json.Marshal(entry.Command)
		if err != nil {
			continue
		}
		var cmd Command
		if err := json.Unmarshal(cmdBytes, &cmd); err != nil {
			continue
		}
		_ = n.stateMachine.Apply(&cmd)
	}
}

// HandleRequestVote handles a RequestVote RPC
func (n *Node) HandleRequestVote(req *RequestVoteRequest) *RequestVoteResponse {
	n.mu.Lock()
	defer n.mu.Unlock()

	resp := &RequestVoteResponse{
		Term:        n.currentTerm,
		VoteGranted: false,
	}

	if req.Term < n.currentTerm {
		return resp
	}

	if req.Term > n.currentTerm {
		n.currentTerm = req.Term
		n.state = Follower
		n.votedFor = ""
	}

	// Check if we can vote for this candidate
	if n.votedFor == "" || n.votedFor == req.CandidateID {
		// Check log is at least as up-to-date
		lastLogIndex := uint64(len(n.log))
		var lastLogTerm uint64
		if lastLogIndex > 0 {
			lastLogTerm = n.log[lastLogIndex-1].Term
		}

		logOk := req.LastLogTerm > lastLogTerm ||
			(req.LastLogTerm == lastLogTerm && req.LastLogIndex >= lastLogIndex)

		if logOk {
			n.votedFor = req.CandidateID
			n.lastHeartbeat = time.Now()
			resp.VoteGranted = true
		}
	}

	resp.Term = n.currentTerm
	return resp
}

// HandleAppendEntries handles an AppendEntries RPC
func (n *Node) HandleAppendEntries(req *AppendEntriesRequest) *AppendEntriesResponse {
	n.mu.Lock()
	defer n.mu.Unlock()

	resp := &AppendEntriesResponse{
		Term:    n.currentTerm,
		Success: false,
	}

	if req.Term < n.currentTerm {
		return resp
	}

	n.lastHeartbeat = time.Now()

	if req.Term > n.currentTerm {
		n.currentTerm = req.Term
		n.votedFor = ""
	}

	n.state = Follower
	n.leaderId = req.LeaderID

	// Check log consistency
	if req.PrevLogIndex > 0 {
		if uint64(len(n.log)) < req.PrevLogIndex {
			return resp
		}
		if n.log[req.PrevLogIndex-1].Term != req.PrevLogTerm {
			n.log = n.log[:req.PrevLogIndex-1]
			return resp
		}
	}

	// Append new entries
	for i, entry := range req.Entries {
		idx := req.PrevLogIndex + uint64(i) + 1
		if idx <= uint64(len(n.log)) {
			if n.log[idx-1].Term != entry.Term {
				n.log = n.log[:idx-1]
				n.log = append(n.log, entry)
			}
		} else {
			n.log = append(n.log, entry)
		}
	}

	// Update commit index
	if req.LeaderCommit > n.commitIndex {
		lastNewEntry := req.PrevLogIndex + uint64(len(req.Entries))
		if req.LeaderCommit < lastNewEntry {
			n.commitIndex = req.LeaderCommit
		} else {
			n.commitIndex = lastNewEntry
		}
		select {
		case n.commitCh <- struct{}{}:
		default:
		}
	}

	resp.Success = true
	resp.Term = n.currentTerm
	return resp
}

// Submit submits a command to the Raft cluster
func (n *Node) Submit(cmd *Command) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.state != Leader {
		if n.leaderId != "" {
			return fmt.Errorf("not leader, redirect to %s", n.leaderId)
		}
		return errors.New("not leader")
	}

	entry := LogEntry{
		Term:    n.currentTerm,
		Index:   uint64(len(n.log) + 1),
		Command: cmd,
	}

	n.log = append(n.log, entry)
	return nil
}

// IsLeader returns true if this node is the leader
func (n *Node) IsLeader() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.state == Leader
}

// GetLeaderID returns the current leader ID
func (n *Node) GetLeaderID() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.leaderId
}

// GetState returns the current node state
func (n *Node) GetState() NodeState {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.state
}

// GetTerm returns the current term
func (n *Node) GetTerm() uint64 {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.currentTerm
}

// NodeInfo holds information about a node
type NodeInfo struct {
	ID       string    `json:"id"`
	Address  string    `json:"address"`
	State    string    `json:"state"`
	Term     uint64    `json:"term"`
	IsLeader bool      `json:"is_leader"`
	LeaderID string    `json:"leader_id"`
}

// GetInfo returns information about this node
func (n *Node) GetInfo() *NodeInfo {
	n.mu.RLock()
	defer n.mu.RUnlock()

	return &NodeInfo{
		ID:       n.ID,
		Address:  n.Address,
		State:    n.state.String(),
		Term:     n.currentTerm,
		IsLeader: n.state == Leader,
		LeaderID: n.leaderId,
	}
}
