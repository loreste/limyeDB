package cluster

import (
	"context"
	"encoding/json"
	"math/rand"
	"sync"
	"time"
)

// GossipConfig holds gossip protocol configuration
type GossipConfig struct {
	GossipInterval   time.Duration // How often to gossip
	GossipFanout     int           // Number of nodes to gossip to each round
	SuspicionMult    int           // Multiplier for suspicion timeout
	ProbeInterval    time.Duration // How often to probe nodes
	ProbeTimeout     time.Duration // Timeout for probe responses
	RetransmitMult   int           // Multiplier for message retransmit
}

// DefaultGossipConfig returns default gossip configuration
func DefaultGossipConfig() *GossipConfig {
	return &GossipConfig{
		GossipInterval:   200 * time.Millisecond,
		GossipFanout:     3,
		SuspicionMult:    4,
		ProbeInterval:    1 * time.Second,
		ProbeTimeout:     500 * time.Millisecond,
		RetransmitMult:   4,
	}
}

// GossipState represents the state of a node in the gossip protocol
type GossipState struct {
	NodeID      string    `json:"node_id"`
	Address     string    `json:"address"`
	State       NodeState `json:"state"`
	Incarnation uint64    `json:"incarnation"`
	LastUpdated time.Time `json:"last_updated"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// GossipMessage represents a gossip message
type GossipMessage struct {
	Type    GossipMessageType `json:"type"`
	States  []GossipState     `json:"states"`
	From    string            `json:"from"`
	SeqNo   uint64            `json:"seq_no"`
}

// GossipMessageType represents the type of gossip message
type GossipMessageType string

const (
	GossipTypePush     GossipMessageType = "push"
	GossipTypePull     GossipMessageType = "pull"
	GossipTypePushPull GossipMessageType = "push_pull"
	GossipTypeProbe    GossipMessageType = "probe"
	GossipTypeProbeAck GossipMessageType = "probe_ack"
	GossipTypeAlive    GossipMessageType = "alive"
	GossipTypeSuspect  GossipMessageType = "suspect"
	GossipTypeDead     GossipMessageType = "dead"
)

// Gossiper implements the gossip protocol for cluster membership
type Gossiper struct {
	config      *GossipConfig
	transport   Transport
	localState  *GossipState
	states      map[string]*GossipState
	incarnation uint64
	seqNo       uint64

	// Suspicion handling
	suspicions map[string]*suspicionTimer

	// Broadcast queue
	broadcasts []GossipMessage

	// Event callbacks
	onJoin  func(nodeID, addr string)
	onLeave func(nodeID string)
	onFail  func(nodeID string)

	mu     sync.RWMutex
	stopCh chan struct{}
	rng    *rand.Rand
}

type suspicionTimer struct {
	nodeID    string
	startTime time.Time
	timeout   time.Duration
	timer     *time.Timer
}

// NewGossiper creates a new gossiper
func NewGossiper(nodeID, addr string, transport Transport, config *GossipConfig) *Gossiper {
	if config == nil {
		config = DefaultGossipConfig()
	}

	g := &Gossiper{
		config:    config,
		transport: transport,
		localState: &GossipState{
			NodeID:      nodeID,
			Address:     addr,
			State:       NodeStateActive,
			Incarnation: 1,
			LastUpdated: time.Now(),
			Metadata:    make(map[string]string),
		},
		states:     make(map[string]*GossipState),
		incarnation: 1,
		suspicions: make(map[string]*suspicionTimer),
		broadcasts: make([]GossipMessage, 0),
		stopCh:     make(chan struct{}),
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	// Add self to states
	g.states[nodeID] = g.localState

	return g
}

// Start starts the gossip protocol
func (g *Gossiper) Start() error {
	// Start gossip loop
	go g.gossipLoop()

	// Start probe loop
	go g.probeLoop()

	// Start suspicion timeout loop
	go g.suspicionLoop()

	return nil
}

// Stop stops the gossip protocol
func (g *Gossiper) Stop() error {
	close(g.stopCh)
	return nil
}

// Join joins a cluster via a seed node
func (g *Gossiper) Join(seedAddr string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Send push-pull to seed node
	msg, err := NewMessage(MsgTypeGossip, g.localState.NodeID, seedAddr, &GossipMessage{
		Type:   GossipTypePushPull,
		States: g.getAllStates(),
		From:   g.localState.NodeID,
		SeqNo:  g.nextSeqNo(),
	})
	if err != nil {
		return err
	}

	resp, err := g.transport.Send(ctx, seedAddr, msg)
	if err != nil {
		return err
	}

	// Process response
	var gossipResp GossipMessage
	if err := resp.Decode(&gossipResp); err != nil {
		return err
	}

	g.mergeStates(gossipResp.States)
	return nil
}

// Leave gracefully leaves the cluster
func (g *Gossiper) Leave() error {
	g.mu.Lock()
	g.localState.State = NodeStateLeaving
	g.incarnation++
	g.localState.Incarnation = g.incarnation
	g.localState.LastUpdated = time.Now()
	g.mu.Unlock()

	// Broadcast leave to all nodes
	g.broadcastState(g.localState)

	return nil
}

// GetMembers returns all known cluster members
func (g *Gossiper) GetMembers() []*GossipState {
	g.mu.RLock()
	defer g.mu.RUnlock()

	members := make([]*GossipState, 0, len(g.states))
	for _, state := range g.states {
		members = append(members, state)
	}
	return members
}

// GetHealthyMembers returns all healthy cluster members
func (g *Gossiper) GetHealthyMembers() []*GossipState {
	g.mu.RLock()
	defer g.mu.RUnlock()

	members := make([]*GossipState, 0)
	for _, state := range g.states {
		if state.State == NodeStateActive {
			members = append(members, state)
		}
	}
	return members
}

// OnJoin sets the callback for node join events
func (g *Gossiper) OnJoin(fn func(nodeID, addr string)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.onJoin = fn
}

// OnLeave sets the callback for node leave events
func (g *Gossiper) OnLeave(fn func(nodeID string)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.onLeave = fn
}

// OnFail sets the callback for node failure events
func (g *Gossiper) OnFail(fn func(nodeID string)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.onFail = fn
}

// SetMetadata sets local node metadata
func (g *Gossiper) SetMetadata(key, value string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.localState.Metadata[key] = value
	g.incarnation++
	g.localState.Incarnation = g.incarnation
	g.localState.LastUpdated = time.Now()
}

func (g *Gossiper) gossipLoop() {
	ticker := time.NewTicker(g.config.GossipInterval)
	defer ticker.Stop()

	for {
		select {
		case <-g.stopCh:
			return
		case <-ticker.C:
			g.doGossip()
		}
	}
}

func (g *Gossiper) doGossip() {
	targets := g.selectGossipTargets()
	if len(targets) == 0 {
		return
	}

	states := g.getAllStates()
	gossipMsg := &GossipMessage{
		Type:   GossipTypePush,
		States: states,
		From:   g.localState.NodeID,
		SeqNo:  g.nextSeqNo(),
	}

	for _, target := range targets {
		go func(addr string) {
			ctx, cancel := context.WithTimeout(context.Background(), g.config.ProbeTimeout)
			defer cancel()

			msg, _ := NewMessage(MsgTypeGossip, g.localState.NodeID, addr, gossipMsg)
			resp, err := g.transport.Send(ctx, addr, msg)
			if err != nil {
				return
			}

			var gossipResp GossipMessage
			if err := resp.Decode(&gossipResp); err != nil {
				return
			}

			g.mergeStates(gossipResp.States)
		}(target)
	}
}

func (g *Gossiper) selectGossipTargets() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Collect healthy nodes (excluding self)
	var candidates []string
	for nodeID, state := range g.states {
		if nodeID != g.localState.NodeID && state.State == NodeStateActive {
			candidates = append(candidates, state.Address)
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// Randomly select up to fanout nodes
	fanout := g.config.GossipFanout
	if fanout > len(candidates) {
		fanout = len(candidates)
	}

	// Fisher-Yates shuffle
	for i := len(candidates) - 1; i > 0; i-- {
		j := g.rng.Intn(i + 1)
		candidates[i], candidates[j] = candidates[j], candidates[i]
	}

	return candidates[:fanout]
}

func (g *Gossiper) probeLoop() {
	ticker := time.NewTicker(g.config.ProbeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-g.stopCh:
			return
		case <-ticker.C:
			g.doProbe()
		}
	}
}

func (g *Gossiper) doProbe() {
	target := g.selectProbeTarget()
	if target == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), g.config.ProbeTimeout)
	defer cancel()

	probeMsg := &GossipMessage{
		Type:  GossipTypeProbe,
		From:  g.localState.NodeID,
		SeqNo: g.nextSeqNo(),
	}

	msg, _ := NewMessage(MsgTypeGossip, g.localState.NodeID, target.Address, probeMsg)
	_, err := g.transport.Send(ctx, target.Address, msg)

	if err != nil {
		// Mark as suspect
		g.startSuspicion(target.NodeID)
	} else {
		// Clear any suspicion
		g.clearSuspicion(target.NodeID)
	}
}

func (g *Gossiper) selectProbeTarget() *GossipState {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var candidates []*GossipState
	for nodeID, state := range g.states {
		if nodeID != g.localState.NodeID && state.State == NodeStateActive {
			candidates = append(candidates, state)
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	return candidates[g.rng.Intn(len(candidates))]
}

func (g *Gossiper) suspicionLoop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-g.stopCh:
			return
		case <-ticker.C:
			g.checkSuspicions()
		}
	}
}

func (g *Gossiper) startSuspicion(nodeID string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, exists := g.suspicions[nodeID]; exists {
		return
	}

	state := g.states[nodeID]
	if state == nil || state.State != NodeStateActive {
		return
	}

	// Calculate timeout based on cluster size
	numNodes := len(g.states)
	timeout := time.Duration(g.config.SuspicionMult) * g.config.ProbeInterval * time.Duration(numNodes)

	state.State = NodeStateSuspect
	state.LastUpdated = time.Now()

	g.suspicions[nodeID] = &suspicionTimer{
		nodeID:    nodeID,
		startTime: time.Now(),
		timeout:   timeout,
	}

	// Broadcast suspicion
	g.queueBroadcast(GossipMessage{
		Type:   GossipTypeSuspect,
		States: []GossipState{*state},
		From:   g.localState.NodeID,
		SeqNo:  g.nextSeqNo(),
	})
}

func (g *Gossiper) clearSuspicion(nodeID string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if timer, exists := g.suspicions[nodeID]; exists {
		if timer.timer != nil {
			timer.timer.Stop()
		}
		delete(g.suspicions, nodeID)
	}

	if state := g.states[nodeID]; state != nil && state.State == NodeStateSuspect {
		state.State = NodeStateActive
		state.LastUpdated = time.Now()
	}
}

func (g *Gossiper) checkSuspicions() {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	for nodeID, timer := range g.suspicions {
		if now.Sub(timer.startTime) > timer.timeout {
			// Mark as dead
			if state := g.states[nodeID]; state != nil {
				state.State = NodeStateInactive
				state.LastUpdated = now

				// Trigger callback
				if g.onFail != nil {
					go g.onFail(nodeID)
				}

				// Broadcast death
				g.queueBroadcast(GossipMessage{
					Type:   GossipTypeDead,
					States: []GossipState{*state},
					From:   g.localState.NodeID,
					SeqNo:  g.nextSeqNo(),
				})
			}

			delete(g.suspicions, nodeID)
		}
	}
}

func (g *Gossiper) mergeStates(states []GossipState) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for _, incoming := range states {
		existing := g.states[incoming.NodeID]

		if existing == nil {
			// New node
			g.states[incoming.NodeID] = &incoming

			if incoming.State == NodeStateActive && g.onJoin != nil {
				go g.onJoin(incoming.NodeID, incoming.Address)
			}
			continue
		}

		// Skip if incoming is older
		if incoming.Incarnation < existing.Incarnation {
			continue
		}

		// If same incarnation, prefer more recent state
		if incoming.Incarnation == existing.Incarnation {
			// Dead > Suspect > Active
			if statePriority(incoming.State) <= statePriority(existing.State) {
				continue
			}
		}

		// Update state
		oldState := existing.State
		*existing = incoming

		// Trigger callbacks
		if oldState == NodeStateActive && incoming.State == NodeStateInactive {
			if g.onFail != nil {
				go g.onFail(incoming.NodeID)
			}
		}
		if oldState == NodeStateActive && incoming.State == NodeStateLeaving {
			if g.onLeave != nil {
				go g.onLeave(incoming.NodeID)
			}
		}
	}
}

func statePriority(state NodeState) int {
	switch state {
	case NodeStateActive:
		return 0
	case NodeStateSuspect:
		return 1
	case NodeStateLeaving:
		return 2
	case NodeStateInactive:
		return 3
	default:
		return -1
	}
}

func (g *Gossiper) getAllStates() []GossipState {
	g.mu.RLock()
	defer g.mu.RUnlock()

	states := make([]GossipState, 0, len(g.states))
	for _, state := range g.states {
		states = append(states, *state)
	}
	return states
}

func (g *Gossiper) nextSeqNo() uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.seqNo++
	return g.seqNo
}

func (g *Gossiper) queueBroadcast(msg GossipMessage) {
	g.broadcasts = append(g.broadcasts, msg)
	// Limit broadcast queue
	if len(g.broadcasts) > 100 {
		g.broadcasts = g.broadcasts[1:]
	}
}

func (g *Gossiper) broadcastState(state *GossipState) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.queueBroadcast(GossipMessage{
		Type:   GossipTypeAlive,
		States: []GossipState{*state},
		From:   g.localState.NodeID,
		SeqNo:  g.nextSeqNo(),
	})
}

// HandleMessage handles an incoming gossip message
func (g *Gossiper) HandleMessage(msg *GossipMessage) *GossipMessage {
	switch msg.Type {
	case GossipTypePush:
		g.mergeStates(msg.States)
		return nil

	case GossipTypePull:
		return &GossipMessage{
			Type:   GossipTypePush,
			States: g.getAllStates(),
			From:   g.localState.NodeID,
			SeqNo:  g.nextSeqNo(),
		}

	case GossipTypePushPull:
		g.mergeStates(msg.States)
		return &GossipMessage{
			Type:   GossipTypePush,
			States: g.getAllStates(),
			From:   g.localState.NodeID,
			SeqNo:  g.nextSeqNo(),
		}

	case GossipTypeProbe:
		return &GossipMessage{
			Type: GossipTypeProbeAck,
			From: g.localState.NodeID,
		}

	case GossipTypeAlive, GossipTypeSuspect, GossipTypeDead:
		g.mergeStates(msg.States)
		return nil

	default:
		return nil
	}
}

// Encode encodes a gossip message
func (gm *GossipMessage) Encode() ([]byte, error) {
	return json.Marshal(gm)
}

// DecodeGossipMessage decodes a gossip message
func DecodeGossipMessage(data []byte) (*GossipMessage, error) {
	var msg GossipMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}
