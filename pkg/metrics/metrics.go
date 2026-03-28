// Package metrics provides Prometheus metrics for observability across LimyeDB components.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ============================================================================
// CDC (Change Data Capture) Metrics
// ============================================================================

var (
	// CDCEventsPublished tracks total CDC events published
	CDCEventsPublished = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "limyedb",
			Subsystem: "cdc",
			Name:      "events_published_total",
			Help:      "Total number of CDC events published",
		},
		[]string{"collection", "event_type"},
	)

	// CDCEventsDropped tracks dropped CDC events (buffer full)
	CDCEventsDropped = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "limyedb",
			Subsystem: "cdc",
			Name:      "events_dropped_total",
			Help:      "Total number of CDC events dropped due to full buffer",
		},
		[]string{"collection"},
	)

	// CDCWebhookDeliveries tracks webhook delivery attempts
	CDCWebhookDeliveries = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "limyedb",
			Subsystem: "cdc",
			Name:      "webhook_deliveries_total",
			Help:      "Total number of webhook delivery attempts",
		},
		[]string{"collection", "status"}, // status: success, error, timeout
	)

	// CDCWebhookLatency tracks webhook delivery latency
	CDCWebhookLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "limyedb",
			Subsystem: "cdc",
			Name:      "webhook_latency_seconds",
			Help:      "Webhook delivery latency in seconds",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"collection"},
	)

	// CDCBufferSize tracks current CDC event buffer size
	CDCBufferSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "limyedb",
			Subsystem: "cdc",
			Name:      "buffer_size",
			Help:      "Current number of events in CDC buffer",
		},
	)

	// CDCSubscriptions tracks number of active webhook subscriptions
	CDCSubscriptions = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "limyedb",
			Subsystem: "cdc",
			Name:      "subscriptions_active",
			Help:      "Number of active webhook subscriptions per collection",
		},
		[]string{"collection"},
	)
)

// ============================================================================
// WebSocket / Real-time Metrics
// ============================================================================

var (
	// WebSocketConnections tracks active WebSocket connections
	WebSocketConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "limyedb",
			Subsystem: "websocket",
			Name:      "connections_active",
			Help:      "Number of active WebSocket connections",
		},
	)

	// WebSocketConnectionsTotal tracks total WebSocket connections
	WebSocketConnectionsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "limyedb",
			Subsystem: "websocket",
			Name:      "connections_total",
			Help:      "Total number of WebSocket connections established",
		},
	)

	// WebSocketMessagesReceived tracks messages received from clients
	WebSocketMessagesReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "limyedb",
			Subsystem: "websocket",
			Name:      "messages_received_total",
			Help:      "Total number of messages received from WebSocket clients",
		},
		[]string{"message_type"},
	)

	// WebSocketMessagesSent tracks messages sent to clients
	WebSocketMessagesSent = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "limyedb",
			Subsystem: "websocket",
			Name:      "messages_sent_total",
			Help:      "Total number of messages sent to WebSocket clients",
		},
		[]string{"event_type"},
	)

	// WebSocketMessagesDropped tracks messages dropped due to full client buffer
	WebSocketMessagesDropped = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "limyedb",
			Subsystem: "websocket",
			Name:      "messages_dropped_total",
			Help:      "Total number of messages dropped due to full client buffer",
		},
	)

	// WebSocketSubscriptions tracks active subscriptions
	WebSocketSubscriptions = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "limyedb",
			Subsystem: "websocket",
			Name:      "subscriptions_active",
			Help:      "Number of active WebSocket subscriptions",
		},
		[]string{"collection"},
	)

	// WebSocketErrors tracks WebSocket errors
	WebSocketErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "limyedb",
			Subsystem: "websocket",
			Name:      "errors_total",
			Help:      "Total number of WebSocket errors",
		},
		[]string{"error_type"},
	)

	// WebSocketBroadcastLatency tracks time to broadcast an event
	WebSocketBroadcastLatency = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "limyedb",
			Subsystem: "websocket",
			Name:      "broadcast_latency_seconds",
			Help:      "Event broadcast latency in seconds",
			Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
		},
	)
)

// ============================================================================
// Embedder Metrics
// ============================================================================

var (
	// EmbedderRequests tracks embedding API requests
	EmbedderRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "limyedb",
			Subsystem: "embedder",
			Name:      "requests_total",
			Help:      "Total number of embedding API requests",
		},
		[]string{"provider", "model", "status"}, // status: success, error, rate_limited
	)

	// EmbedderLatency tracks embedding API latency
	EmbedderLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "limyedb",
			Subsystem: "embedder",
			Name:      "latency_seconds",
			Help:      "Embedding API latency in seconds",
			Buckets:   []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		},
		[]string{"provider", "model"},
	)

	// EmbedderTokensUsed tracks tokens consumed by embedding APIs
	EmbedderTokensUsed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "limyedb",
			Subsystem: "embedder",
			Name:      "tokens_used_total",
			Help:      "Total number of tokens used by embedding APIs",
		},
		[]string{"provider", "model"},
	)

	// EmbedderBatchSize tracks batch sizes for embedding requests
	EmbedderBatchSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "limyedb",
			Subsystem: "embedder",
			Name:      "batch_size",
			Help:      "Batch sizes for embedding requests",
			Buckets:   []float64{1, 5, 10, 25, 50, 100, 250, 500},
		},
		[]string{"provider"},
	)

	// EmbedderRetries tracks retry attempts
	EmbedderRetries = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "limyedb",
			Subsystem: "embedder",
			Name:      "retries_total",
			Help:      "Total number of embedding API retry attempts",
		},
		[]string{"provider", "model"},
	)

	// EmbedderCacheHits tracks embedding cache hits (if caching enabled)
	EmbedderCacheHits = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "limyedb",
			Subsystem: "embedder",
			Name:      "cache_hits_total",
			Help:      "Total number of embedding cache hits",
		},
	)

	// EmbedderCacheMisses tracks embedding cache misses
	EmbedderCacheMisses = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "limyedb",
			Subsystem: "embedder",
			Name:      "cache_misses_total",
			Help:      "Total number of embedding cache misses",
		},
	)
)

// ============================================================================
// Vectorizer Metrics
// ============================================================================

var (
	// VectorizerRequests tracks vectorizer requests
	VectorizerRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "limyedb",
			Subsystem: "vectorizer",
			Name:      "requests_total",
			Help:      "Total number of vectorizer requests",
		},
		[]string{"vectorizer_type", "status"},
	)

	// VectorizerLatency tracks vectorizer processing latency
	VectorizerLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "limyedb",
			Subsystem: "vectorizer",
			Name:      "latency_seconds",
			Help:      "Vectorizer processing latency in seconds",
			Buckets:   []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		},
		[]string{"vectorizer_type"},
	)

	// VectorizerTextsProcessed tracks total texts vectorized
	VectorizerTextsProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "limyedb",
			Subsystem: "vectorizer",
			Name:      "texts_processed_total",
			Help:      "Total number of texts vectorized",
		},
		[]string{"vectorizer_type"},
	)
)

// ============================================================================
// Search Metrics
// ============================================================================

var (
	// SearchQueries tracks search queries
	SearchQueries = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "limyedb",
			Subsystem: "search",
			Name:      "queries_total",
			Help:      "Total number of search queries",
		},
		[]string{"collection", "search_type"}, // search_type: vector, hybrid, recommend
	)

	// SearchLatency tracks search latency
	SearchLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "limyedb",
			Subsystem: "search",
			Name:      "latency_seconds",
			Help:      "Search query latency in seconds",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
		},
		[]string{"collection", "search_type"},
	)

	// SearchResultCount tracks number of results returned
	SearchResultCount = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "limyedb",
			Subsystem: "search",
			Name:      "result_count",
			Help:      "Number of results returned per search",
			Buckets:   []float64{0, 1, 5, 10, 25, 50, 100, 250, 500, 1000},
		},
		[]string{"collection"},
	)

	// SearchFilteredCount tracks vectors filtered out
	SearchFilteredCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "limyedb",
			Subsystem: "search",
			Name:      "filtered_total",
			Help:      "Total number of vectors filtered out during search",
		},
		[]string{"collection"},
	)
)

// ============================================================================
// Index Metrics
// ============================================================================

var (
	// IndexBuildLatency tracks index build time
	IndexBuildLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "limyedb",
			Subsystem: "index",
			Name:      "build_latency_seconds",
			Help:      "Index build latency in seconds",
			Buckets:   []float64{0.1, 0.5, 1, 5, 10, 30, 60, 120, 300},
		},
		[]string{"collection", "index_type"},
	)

	// IndexMemoryUsage tracks memory used by indexes
	IndexMemoryUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "limyedb",
			Subsystem: "index",
			Name:      "memory_bytes",
			Help:      "Memory used by indexes in bytes",
		},
		[]string{"collection", "index_type"},
	)

	// IndexVectorCount tracks vectors in index
	IndexVectorCount = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "limyedb",
			Subsystem: "index",
			Name:      "vector_count",
			Help:      "Number of vectors in index",
		},
		[]string{"collection", "index_type"},
	)
)

// ============================================================================
// Storage Metrics
// ============================================================================

var (
	// WALWriteLatency tracks WAL write latency
	WALWriteLatency = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "limyedb",
			Subsystem: "wal",
			Name:      "write_latency_seconds",
			Help:      "WAL write latency in seconds",
			Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
		},
	)

	// WALSyncLatency tracks WAL sync latency
	WALSyncLatency = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "limyedb",
			Subsystem: "wal",
			Name:      "sync_latency_seconds",
			Help:      "WAL sync latency in seconds",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
		},
	)

	// WALSegmentCount tracks WAL segments
	WALSegmentCount = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "limyedb",
			Subsystem: "wal",
			Name:      "segment_count",
			Help:      "Number of WAL segments",
		},
	)

	// WALBytesWritten tracks bytes written to WAL
	WALBytesWritten = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "limyedb",
			Subsystem: "wal",
			Name:      "bytes_written_total",
			Help:      "Total bytes written to WAL",
		},
	)
)

// ============================================================================
// Cluster Metrics
// ============================================================================

var (
	// RaftState tracks Raft node state
	RaftState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "limyedb",
			Subsystem: "raft",
			Name:      "state",
			Help:      "Raft node state (0=follower, 1=candidate, 2=leader)",
		},
		[]string{"node_id"},
	)

	// RaftAppliedIndex tracks applied log index
	RaftAppliedIndex = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "limyedb",
			Subsystem: "raft",
			Name:      "applied_index",
			Help:      "Last applied Raft log index",
		},
		[]string{"node_id"},
	)

	// RaftCommittedIndex tracks committed log index
	RaftCommittedIndex = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "limyedb",
			Subsystem: "raft",
			Name:      "committed_index",
			Help:      "Last committed Raft log index",
		},
		[]string{"node_id"},
	)

	// RaftPeers tracks number of peers in cluster
	RaftPeers = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "limyedb",
			Subsystem: "raft",
			Name:      "peers",
			Help:      "Number of peers in Raft cluster",
		},
	)
)

// ============================================================================
// HTTP Request Metrics
// ============================================================================

var (
	// RequestDuration tracks HTTP request duration in seconds
	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "limyedb",
			Name:      "request_duration_seconds",
			Help:      "HTTP request duration in seconds",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"method", "path", "status"},
	)

	// RequestTotal tracks total HTTP requests
	RequestTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "limyedb",
			Name:      "request_total",
			Help:      "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)
)

// ============================================================================
// Helper Functions
// ============================================================================

// RecordCDCEvent records a CDC event publication
func RecordCDCEvent(collection, eventType string) {
	CDCEventsPublished.WithLabelValues(collection, eventType).Inc()
}

// RecordCDCDropped records a dropped CDC event
func RecordCDCDropped(collection string) {
	CDCEventsDropped.WithLabelValues(collection).Inc()
}

// RecordWebhookDelivery records a webhook delivery attempt
func RecordWebhookDelivery(collection, status string, latencySeconds float64) {
	CDCWebhookDeliveries.WithLabelValues(collection, status).Inc()
	CDCWebhookLatency.WithLabelValues(collection).Observe(latencySeconds)
}

// RecordWebSocketConnect records a new WebSocket connection
func RecordWebSocketConnect() {
	WebSocketConnections.Inc()
	WebSocketConnectionsTotal.Inc()
}

// RecordWebSocketDisconnect records a WebSocket disconnection
func RecordWebSocketDisconnect() {
	WebSocketConnections.Dec()
}

// RecordEmbedderRequest records an embedding API request
func RecordEmbedderRequest(provider, model, status string, latencySeconds float64, batchSize int) {
	EmbedderRequests.WithLabelValues(provider, model, status).Inc()
	EmbedderLatency.WithLabelValues(provider, model).Observe(latencySeconds)
	EmbedderBatchSize.WithLabelValues(provider).Observe(float64(batchSize))
}

// RecordSearchQuery records a search query
func RecordSearchQuery(collection, searchType string, latencySeconds float64, resultCount int) {
	SearchQueries.WithLabelValues(collection, searchType).Inc()
	SearchLatency.WithLabelValues(collection, searchType).Observe(latencySeconds)
	SearchResultCount.WithLabelValues(collection).Observe(float64(resultCount))
}
