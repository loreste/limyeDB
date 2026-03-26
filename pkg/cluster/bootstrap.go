package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// BootstrapConfig holds bootstrap configuration
type BootstrapConfig struct {
	// Cluster name for identification
	ClusterName string `json:"cluster_name"`

	// Node configuration
	NodeID       string `json:"node_id"`
	BindAddr     string `json:"bind_addr"`
	AdvertiseAddr string `json:"advertise_addr"`

	// Bootstrap mode
	Bootstrap     bool     `json:"bootstrap"`       // True if this is the first node
	BootstrapExpect int    `json:"bootstrap_expect"` // Number of nodes to wait for
	SeedNodes     []string `json:"seed_nodes"`      // Seed nodes for joining

	// Discovery
	DiscoveryType DiscoveryType `json:"discovery_type"`
	DNSName       string        `json:"dns_name,omitempty"`      // For DNS discovery
	ConsulAddr    string        `json:"consul_addr,omitempty"`   // For Consul discovery
	K8sNamespace  string        `json:"k8s_namespace,omitempty"` // For Kubernetes discovery
	K8sService    string        `json:"k8s_service,omitempty"`

	// Data directory
	DataDir string `json:"data_dir"`

	// Timeouts
	JoinTimeout   time.Duration `json:"join_timeout"`
	RetryInterval time.Duration `json:"retry_interval"`
}

// DiscoveryType represents the type of node discovery
type DiscoveryType string

const (
	DiscoveryTypeStatic    DiscoveryType = "static"     // Static seed list
	DiscoveryTypeDNS       DiscoveryType = "dns"        // DNS SRV records
	DiscoveryTypeConsul    DiscoveryType = "consul"     // Consul service discovery
	DiscoveryTypeK8s       DiscoveryType = "kubernetes" // Kubernetes service discovery
	DiscoveryTypeMulticast DiscoveryType = "multicast"  // Multicast for local networks
)

// DefaultBootstrapConfig returns default bootstrap configuration
func DefaultBootstrapConfig() *BootstrapConfig {
	hostname, _ := os.Hostname()

	return &BootstrapConfig{
		ClusterName:     "limyedb",
		NodeID:          hostname,
		BindAddr:        "0.0.0.0:7946",
		AdvertiseAddr:   "",
		Bootstrap:       false,
		BootstrapExpect: 3,
		DiscoveryType:   DiscoveryTypeStatic,
		DataDir:         "./data",
		JoinTimeout:     30 * time.Second,
		RetryInterval:   5 * time.Second,
	}
}

// Bootstrapper handles cluster bootstrapping and node discovery
type Bootstrapper struct {
	config      *BootstrapConfig
	coordinator *Coordinator
	gossiper    *Gossiper
	transport   Transport
	discovery   NodeDiscovery

	// State
	bootstrapped bool
	joined       bool

	// State file
	stateFile string

	mu     sync.RWMutex
	stopCh chan struct{}
}

// ClusterState persisted to disk
type BootstrapState struct {
	ClusterID   string    `json:"cluster_id"`
	ClusterName string    `json:"cluster_name"`
	NodeID      string    `json:"node_id"`
	Bootstrapped bool     `json:"bootstrapped"`
	JoinedAt    time.Time `json:"joined_at"`
	Members     []string  `json:"members"`
}

// NodeDiscovery discovers cluster nodes
type NodeDiscovery interface {
	// Discover returns a list of node addresses
	Discover(ctx context.Context) ([]string, error)

	// Type returns the discovery type
	Type() DiscoveryType
}

// NewBootstrapper creates a new bootstrapper
func NewBootstrapper(config *BootstrapConfig) (*Bootstrapper, error) {
	if config == nil {
		config = DefaultBootstrapConfig()
	}

	// Resolve advertise address if not set
	if config.AdvertiseAddr == "" {
		addr, err := resolveAdvertiseAddr(config.BindAddr)
		if err != nil {
			return nil, err
		}
		config.AdvertiseAddr = addr
	}

	// Create discovery
	discovery, err := createDiscovery(config)
	if err != nil {
		return nil, err
	}

	// Create transport
	transport := NewTCPTransport(config.BindAddr)

	// Create coordinator
	coordConfig := &Config{
		NodeID:            config.NodeID,
		ListenAddr:        config.BindAddr,
		AdvertiseAddr:     config.AdvertiseAddr,
		SeedNodes:         config.SeedNodes,
		ReplicationFactor: 2,
		ShardCount:        16,
		HeartbeatInterval: 1 * time.Second,
		FailureTimeout:    5 * time.Second,
	}
	coordinator := NewCoordinator(coordConfig)

	// Create gossiper
	gossiper := NewGossiper(config.NodeID, config.AdvertiseAddr, transport, nil)

	b := &Bootstrapper{
		config:      config,
		coordinator: coordinator,
		gossiper:    gossiper,
		transport:   transport,
		discovery:   discovery,
		stateFile:   filepath.Join(config.DataDir, "cluster_state.json"),
		stopCh:      make(chan struct{}),
	}

	return b, nil
}

// Start starts the bootstrapper
func (b *Bootstrapper) Start(ctx context.Context) error {
	// Create data directory
	if err := os.MkdirAll(b.config.DataDir, 0750); err != nil {
		return err
	}

	// Load existing state
	if err := b.loadState(); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Start transport
	if err := b.transport.Start(); err != nil {
		return err
	}

	// Start gossiper
	if err := b.gossiper.Start(); err != nil {
		return err
	}

	// Bootstrap or join
	if b.config.Bootstrap {
		return b.bootstrapCluster(ctx)
	}

	return b.joinCluster(ctx)
}

// Stop stops the bootstrapper
func (b *Bootstrapper) Stop() error {
	close(b.stopCh)

	_ = b.saveState()         // Best effort during shutdown
	_ = b.gossiper.Leave()    // Best effort during shutdown
	_ = b.gossiper.Stop()     // Best effort during shutdown
	_ = b.transport.Stop()    // Best effort during shutdown
	_ = b.coordinator.Stop()  // Best effort during shutdown

	return nil
}

// GetCoordinator returns the cluster coordinator
func (b *Bootstrapper) GetCoordinator() *Coordinator {
	return b.coordinator
}

// GetGossiper returns the gossiper
func (b *Bootstrapper) GetGossiper() *Gossiper {
	return b.gossiper
}

// IsBootstrapped returns true if the cluster is bootstrapped
func (b *Bootstrapper) IsBootstrapped() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.bootstrapped
}

// IsJoined returns true if this node has joined the cluster
func (b *Bootstrapper) IsJoined() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.joined
}

func (b *Bootstrapper) bootstrapCluster(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Wait for expected number of nodes if configured
	if b.config.BootstrapExpect > 1 {
		if err := b.waitForNodes(ctx, b.config.BootstrapExpect); err != nil {
			return err
		}
	}

	// Start coordinator
	if err := b.coordinator.Start(ctx); err != nil {
		return err
	}

	b.bootstrapped = true
	b.joined = true

	return b.saveState()
}

func (b *Bootstrapper) joinCluster(ctx context.Context) error {
	// Discover seed nodes
	seeds, err := b.discoverSeeds(ctx)
	if err != nil {
		return err
	}

	if len(seeds) == 0 {
		return errors.New("no seed nodes found")
	}

	// Try to join via each seed
	var lastErr error
	for _, seed := range seeds {
		if err := b.gossiper.Join(seed); err != nil {
			lastErr = err
			continue
		}

		// Successfully joined
		b.mu.Lock()
		b.joined = true
		b.bootstrapped = true
		b.mu.Unlock()

		// Start coordinator
		if err := b.coordinator.Start(ctx); err != nil {
			return err
		}

		return b.saveState()
	}

	return fmt.Errorf("failed to join cluster: %w", lastErr)
}

func (b *Bootstrapper) waitForNodes(ctx context.Context, expected int) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	timeout := time.After(b.config.JoinTimeout)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return errors.New("timeout waiting for nodes")
		case <-ticker.C:
			members := b.gossiper.GetMembers()
			if len(members) >= expected {
				return nil
			}
		}
	}
}

func (b *Bootstrapper) discoverSeeds(ctx context.Context) ([]string, error) {
	// Use static seeds first
	if len(b.config.SeedNodes) > 0 {
		return b.config.SeedNodes, nil
	}

	// Use discovery
	if b.discovery != nil {
		return b.discovery.Discover(ctx)
	}

	return nil, errors.New("no seeds configured and no discovery method")
}

func (b *Bootstrapper) loadState() error {
	data, err := os.ReadFile(b.stateFile)
	if err != nil {
		return err
	}

	var state BootstrapState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	b.bootstrapped = state.Bootstrapped
	b.joined = state.Bootstrapped

	return nil
}

func (b *Bootstrapper) saveState() error {
	state := BootstrapState{
		ClusterName:  b.config.ClusterName,
		NodeID:       b.config.NodeID,
		Bootstrapped: b.bootstrapped,
		JoinedAt:     time.Now(),
	}

	// Collect member list
	members := b.gossiper.GetMembers()
	for _, m := range members {
		state.Members = append(state.Members, m.Address)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(b.stateFile, data, 0600)
}

func resolveAdvertiseAddr(bindAddr string) (string, error) {
	host, port, err := net.SplitHostPort(bindAddr)
	if err != nil {
		return "", err
	}

	if host == "" || host == "0.0.0.0" {
		// Get the first non-loopback IP
		addrs, err := net.InterfaceAddrs()
		if err != nil {
			return "", err
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					return net.JoinHostPort(ipnet.IP.String(), port), nil
				}
			}
		}

		return "", errors.New("no suitable address found")
	}

	return bindAddr, nil
}

func createDiscovery(config *BootstrapConfig) (NodeDiscovery, error) {
	switch config.DiscoveryType {
	case DiscoveryTypeStatic:
		return &StaticDiscovery{seeds: config.SeedNodes}, nil
	case DiscoveryTypeDNS:
		return &DNSDiscovery{name: config.DNSName}, nil
	case DiscoveryTypeConsul:
		return NewConsulDiscovery(config.ConsulAddr, config.ClusterName)
	case DiscoveryTypeK8s:
		return NewK8sDiscovery(config.K8sNamespace, config.K8sService)
	default:
		return &StaticDiscovery{seeds: config.SeedNodes}, nil
	}
}

// StaticDiscovery uses a static list of seeds
type StaticDiscovery struct {
	seeds []string
}

func (d *StaticDiscovery) Discover(ctx context.Context) ([]string, error) {
	return d.seeds, nil
}

func (d *StaticDiscovery) Type() DiscoveryType {
	return DiscoveryTypeStatic
}

// DNSDiscovery uses DNS SRV records
type DNSDiscovery struct {
	name string
}

func (d *DNSDiscovery) Discover(ctx context.Context) ([]string, error) {
	_, srvs, err := net.LookupSRV("", "", d.name)
	if err != nil {
		return nil, err
	}

	var addrs []string
	for _, srv := range srvs {
		addr := fmt.Sprintf("%s:%d", srv.Target, srv.Port)
		addrs = append(addrs, addr)
	}

	return addrs, nil
}

func (d *DNSDiscovery) Type() DiscoveryType {
	return DiscoveryTypeDNS
}

// ConsulDiscovery uses Consul for service discovery
type ConsulDiscovery struct {
	addr        string
	serviceName string
}

func NewConsulDiscovery(addr, serviceName string) (*ConsulDiscovery, error) {
	return &ConsulDiscovery{
		addr:        addr,
		serviceName: serviceName,
	}, nil
}

func (d *ConsulDiscovery) Discover(ctx context.Context) ([]string, error) {
	url := fmt.Sprintf("http://%s/v1/catalog/service/%s", d.addr, d.serviceName)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("consul returned status: %d", resp.StatusCode)
	}

	var services []struct {
		ServiceAddress string `json:"ServiceAddress"`
		Address        string `json:"Address"`
		ServicePort    int    `json:"ServicePort"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&services); err != nil {
		return nil, err
	}

	var addrs []string
	for _, srv := range services {
		host := srv.ServiceAddress
		if host == "" {
			host = srv.Address
		}
		port := srv.ServicePort
		if port == 0 {
			port = 7946 // Default Gossip/Raft port
		}
		addrs = append(addrs, fmt.Sprintf("%s:%d", host, port))
	}

	if len(addrs) == 0 {
		return nil, errors.New("no nodes found via consul")
	}

	return addrs, nil
}

func (d *ConsulDiscovery) Type() DiscoveryType {
	return DiscoveryTypeConsul
}

// K8sDiscovery uses Kubernetes for service discovery
type K8sDiscovery struct {
	namespace   string
	serviceName string
}

func NewK8sDiscovery(namespace, serviceName string) (*K8sDiscovery, error) {
	return &K8sDiscovery{
		namespace:   namespace,
		serviceName: serviceName,
	}, nil
}

func (d *K8sDiscovery) Discover(ctx context.Context) ([]string, error) {
	// Use headless service DNS
	// Format: <service>.<namespace>.svc.cluster.local
	name := fmt.Sprintf("%s.%s.svc.cluster.local", d.serviceName, d.namespace)

	ips, err := net.LookupHost(name)
	if err != nil {
		return nil, err
	}

	var addrs []string
	for _, ip := range ips {
		// Default to cluster port 7946
		addrs = append(addrs, fmt.Sprintf("%s:7946", ip))
	}

	return addrs, nil
}

func (d *K8sDiscovery) Type() DiscoveryType {
	return DiscoveryTypeK8s
}

// ClusterInfo returns information about the cluster
type ClusterInfo struct {
	ClusterName string            `json:"cluster_name"`
	NodeID      string            `json:"node_id"`
	State       string            `json:"state"`
	Members     []MemberInfo      `json:"members"`
	Shards      map[uint32]*Shard `json:"shards"`
	Leader      string            `json:"leader,omitempty"`
}

// MemberInfo holds information about a cluster member
type MemberInfo struct {
	ID      string    `json:"id"`
	Address string    `json:"address"`
	State   NodeState `json:"state"`
	Role    string    `json:"role"` // leader, follower, candidate
}

// GetClusterInfo returns information about the cluster
func (b *Bootstrapper) GetClusterInfo() *ClusterInfo {
	info := &ClusterInfo{
		ClusterName: b.config.ClusterName,
		NodeID:      b.config.NodeID,
		State:       "unknown",
	}

	if b.IsJoined() {
		info.State = "healthy"
	} else {
		info.State = "joining"
	}

	// Get members
	members := b.gossiper.GetMembers()
	for _, m := range members {
		info.Members = append(info.Members, MemberInfo{
			ID:      m.NodeID,
			Address: m.Address,
			State:   m.State,
			Role:    "follower",
		})
	}

	// Get shards
	if b.coordinator != nil {
		state := b.coordinator.GetState()
		info.Shards = state.Shards
		info.Leader = state.LeaderID

		// Update leader role
		for i := range info.Members {
			if info.Members[i].ID == state.LeaderID {
				info.Members[i].Role = "leader"
			}
		}
	}

	return info
}
