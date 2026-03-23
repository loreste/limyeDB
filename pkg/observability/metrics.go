// Package observability provides metrics, tracing, and logging utilities for LimyeDB.
package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus metrics for LimyeDB.
type Metrics struct {
	// Request metrics
	RequestDuration *prometheus.HistogramVec
	RequestTotal    *prometheus.CounterVec
	RequestInFlight prometheus.Gauge

	// Search metrics
	SearchLatency    *prometheus.HistogramVec
	SearchTotal      *prometheus.CounterVec
	SearchResultSize *prometheus.HistogramVec

	// Insert metrics
	InsertLatency *prometheus.HistogramVec
	InsertTotal   *prometheus.CounterVec
	InsertBatch   *prometheus.HistogramVec

	// Collection metrics
	CollectionsTotal prometheus.Gauge
	VectorsTotal     *prometheus.GaugeVec
	CollectionSize   *prometheus.GaugeVec

	// HNSW index metrics
	HNSWSearchVisited *prometheus.HistogramVec
	HNSWLayers        *prometheus.GaugeVec
	HNSWConnections   *prometheus.GaugeVec

	// Cluster metrics
	RaftState          prometheus.Gauge
	RaftTerm           prometheus.Gauge
	RaftCommitIndex    prometheus.Gauge
	RaftAppliedIndex   prometheus.Gauge
	GossipMembers      prometheus.Gauge
	GossipMessagesSent *prometheus.CounterVec
	GossipMessagesRecv *prometheus.CounterVec

	// Storage metrics
	WALSize          prometheus.Gauge
	WALSegments      prometheus.Gauge
	WALWriteLatency  prometheus.Histogram
	SnapshotSize     prometheus.Gauge
	SnapshotDuration prometheus.Histogram
	MMapSize         prometheus.Gauge

	// Go runtime metrics
	GoroutinesCount prometheus.Gauge
	HeapAlloc       prometheus.Gauge
	HeapInUse       prometheus.Gauge
	GCPauseTotal    prometheus.Counter
}

// DefaultMetrics returns a new Metrics instance with default settings.
func DefaultMetrics() *Metrics {
	return NewMetrics("limyedb")
}

// NewMetrics creates a new Metrics instance with the given namespace.
func NewMetrics(namespace string) *Metrics {
	m := &Metrics{
		// Request metrics
		RequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "request_duration_seconds",
				Help:      "Duration of HTTP requests in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "path", "status"},
		),
		RequestTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "request_total",
				Help:      "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		RequestInFlight: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "request_in_flight",
				Help:      "Number of requests currently being processed",
			},
		),

		// Search metrics
		SearchLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "search_latency_seconds",
				Help:      "Search latency in seconds",
				Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
			},
			[]string{"collection"},
		),
		SearchTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "search_total",
				Help:      "Total number of search operations",
			},
			[]string{"collection"},
		),
		SearchResultSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "search_result_size",
				Help:      "Number of results returned per search",
				Buckets:   []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
			},
			[]string{"collection"},
		),

		// Insert metrics
		InsertLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "insert_latency_seconds",
				Help:      "Insert latency in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"collection"},
		),
		InsertTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "insert_total",
				Help:      "Total number of vectors inserted",
			},
			[]string{"collection"},
		),
		InsertBatch: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "insert_batch_size",
				Help:      "Number of vectors per insert batch",
				Buckets:   []float64{1, 10, 50, 100, 500, 1000, 5000, 10000},
			},
			[]string{"collection"},
		),

		// Collection metrics
		CollectionsTotal: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "collections_total",
				Help:      "Total number of collections",
			},
		),
		VectorsTotal: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "vectors_total",
				Help:      "Total number of vectors per collection",
			},
			[]string{"collection"},
		),
		CollectionSize: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "collection_size_bytes",
				Help:      "Size of collection in bytes",
			},
			[]string{"collection"},
		),

		// HNSW index metrics
		HNSWSearchVisited: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "hnsw_search_visited",
				Help:      "Number of nodes visited during HNSW search",
				Buckets:   []float64{10, 50, 100, 250, 500, 1000, 2500, 5000},
			},
			[]string{"collection"},
		),
		HNSWLayers: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "hnsw_layers",
				Help:      "Number of layers in HNSW index",
			},
			[]string{"collection"},
		),
		HNSWConnections: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "hnsw_connections_avg",
				Help:      "Average connections per node in HNSW index",
			},
			[]string{"collection"},
		),

		// Cluster metrics
		RaftState: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "raft_state",
				Help:      "Raft state (0=follower, 1=candidate, 2=leader)",
			},
		),
		RaftTerm: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "raft_term",
				Help:      "Current Raft term",
			},
		),
		RaftCommitIndex: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "raft_commit_index",
				Help:      "Raft commit index",
			},
		),
		RaftAppliedIndex: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "raft_applied_index",
				Help:      "Raft applied index",
			},
		),
		GossipMembers: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "gossip_members",
				Help:      "Number of active gossip members",
			},
		),
		GossipMessagesSent: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "gossip_messages_sent_total",
				Help:      "Total gossip messages sent",
			},
			[]string{"type"},
		),
		GossipMessagesRecv: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "gossip_messages_received_total",
				Help:      "Total gossip messages received",
			},
			[]string{"type"},
		),

		// Storage metrics
		WALSize: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "wal_size_bytes",
				Help:      "Total WAL size in bytes",
			},
		),
		WALSegments: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "wal_segments",
				Help:      "Number of WAL segments",
			},
		),
		WALWriteLatency: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "wal_write_latency_seconds",
				Help:      "WAL write latency in seconds",
				Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
			},
		),
		SnapshotSize: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "snapshot_size_bytes",
				Help:      "Size of last snapshot in bytes",
			},
		),
		SnapshotDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "snapshot_duration_seconds",
				Help:      "Snapshot creation duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
		),
		MMapSize: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "mmap_size_bytes",
				Help:      "Total memory-mapped file size in bytes",
			},
		),

		// Go runtime metrics
		GoroutinesCount: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "goroutines",
				Help:      "Number of goroutines",
			},
		),
		HeapAlloc: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "heap_alloc_bytes",
				Help:      "Heap bytes allocated",
			},
		),
		HeapInUse: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "heap_inuse_bytes",
				Help:      "Heap bytes in use",
			},
		),
		GCPauseTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "gc_pause_total_seconds",
				Help:      "Total GC pause time in seconds",
			},
		),
	}

	return m
}

// Handler returns an HTTP handler for the metrics endpoint.
func (m *Metrics) Handler() http.Handler {
	return promhttp.Handler()
}

// ObserveSearch records search metrics.
func (m *Metrics) ObserveSearch(collection string, duration time.Duration, resultCount int) {
	m.SearchLatency.WithLabelValues(collection).Observe(duration.Seconds())
	m.SearchTotal.WithLabelValues(collection).Inc()
	m.SearchResultSize.WithLabelValues(collection).Observe(float64(resultCount))
}

// ObserveInsert records insert metrics.
func (m *Metrics) ObserveInsert(collection string, duration time.Duration, count int) {
	m.InsertLatency.WithLabelValues(collection).Observe(duration.Seconds())
	m.InsertTotal.WithLabelValues(collection).Add(float64(count))
	m.InsertBatch.WithLabelValues(collection).Observe(float64(count))
}

// ObserveRequest records HTTP request metrics.
func (m *Metrics) ObserveRequest(method, path string, status int, duration time.Duration) {
	statusStr := strconv.Itoa(status)
	m.RequestDuration.WithLabelValues(method, path, statusStr).Observe(duration.Seconds())
	m.RequestTotal.WithLabelValues(method, path, statusStr).Inc()
}

// SetCollectionMetrics updates collection-level metrics.
func (m *Metrics) SetCollectionMetrics(collection string, vectors int64, sizeBytes int64) {
	m.VectorsTotal.WithLabelValues(collection).Set(float64(vectors))
	m.CollectionSize.WithLabelValues(collection).Set(float64(sizeBytes))
}

// SetHNSWMetrics updates HNSW index metrics.
func (m *Metrics) SetHNSWMetrics(collection string, layers int, avgConnections float64) {
	m.HNSWLayers.WithLabelValues(collection).Set(float64(layers))
	m.HNSWConnections.WithLabelValues(collection).Set(avgConnections)
}

// SetRaftState updates Raft cluster state metrics.
func (m *Metrics) SetRaftState(state int, term, commitIndex, appliedIndex uint64) {
	m.RaftState.Set(float64(state))
	m.RaftTerm.Set(float64(term))
	m.RaftCommitIndex.Set(float64(commitIndex))
	m.RaftAppliedIndex.Set(float64(appliedIndex))
}

// SetGossipMembers updates gossip membership count.
func (m *Metrics) SetGossipMembers(count int) {
	m.GossipMembers.Set(float64(count))
}
