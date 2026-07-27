package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/limyedb/limyedb/pkg/quantization"
)

// Config holds the main configuration for LimyeDB
type Config struct {
	Server   ServerConfig   `json:"server"`
	Storage  StorageConfig  `json:"storage"`
	HNSW     HNSWConfig     `json:"hnsw"`
	WAL      WALConfig      `json:"wal"`
	Snapshot SnapshotConfig `json:"snapshot"`
}

// Duration wraps time.Duration so config files can express timeouts as
// human-readable strings ("30s") as well as raw nanosecond counts. A plain
// time.Duration field accepts neither from JSON: json.Unmarshal rejects the
// string form, so duration settings never actually loaded from any config
// file, in either JSON or YAML.
type Duration time.Duration

// UnmarshalJSON accepts either a duration string ("30s", "1m") or a numeric
// nanosecond count.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case string:
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", value, err)
		}
		*d = Duration(parsed)
	case float64:
		*d = Duration(time.Duration(value))
	default:
		return fmt.Errorf("duration must be a string or number, got %T", v)
	}
	return nil
}

// MarshalJSON writes the duration in its human-readable string form so a
// round-trip through Save produces a file that Load can read back.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// Std returns the underlying time.Duration.
func (d Duration) Std() time.Duration { return time.Duration(d) }

// ServerConfig holds server-related configuration
type ServerConfig struct {
	RESTAddress    string   `json:"rest_address"`
	GRPCAddress    string   `json:"grpc_address"`
	ReadTimeout    Duration `json:"read_timeout"`
	WriteTimeout   Duration `json:"write_timeout"`
	MaxRequestSize int64    `json:"max_request_size"`
}

// StorageConfig holds storage-related configuration
type StorageConfig struct {
	DataDir         string `json:"data_dir"`
	MaxCollections  int    `json:"max_collections"`
	MmapEnabled     bool   `json:"mmap_enabled"`
	FlushIntervalMs int    `json:"flush_interval_ms"`
}

// HNSWConfig holds HNSW index configuration defaults
type HNSWConfig struct {
	M              int `json:"m"`               // Max connections per node
	EfConstruction int `json:"ef_construction"` // Build quality
	EfSearch       int `json:"ef_search"`       // Search quality
	MaxElements    int `json:"max_elements"`    // Initial capacity
}

// WALConfig holds write-ahead log configuration
type WALConfig struct {
	Enabled       bool   `json:"enabled"`
	Dir           string `json:"dir"`
	SegmentSizeMB int    `json:"segment_size_mb"`
	SyncOnWrite   bool   `json:"sync_on_write"`

	// Async write options for improved throughput (higher risk of data loss)
	AsyncEnabled    bool `json:"async_enabled"`     // Enable async write queue
	AsyncBatchSize  int  `json:"async_batch_size"`  // Max records to batch (default: 100)
	AsyncIntervalMs int  `json:"async_interval_ms"` // Max ms between flushes (default: 10)
}

// SnapshotConfig holds snapshot configuration
type SnapshotConfig struct {
	Dir            string `json:"dir"`
	IntervalSec    int    `json:"interval_sec"`
	RetainCount    int    `json:"retain_count"`
	CompressionLvl int    `json:"compression_level"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			RESTAddress:    ":8080",
			GRPCAddress:    ":50051",
			ReadTimeout:    Duration(30 * time.Second),
			WriteTimeout:   Duration(30 * time.Second),
			MaxRequestSize: 64 * 1024 * 1024, // 64MB
		},
		Storage: StorageConfig{
			DataDir:         "./data",
			MaxCollections:  1000,
			MmapEnabled:     true,
			FlushIntervalMs: 1000,
		},
		HNSW: HNSWConfig{
			M:              16,
			EfConstruction: 200,
			EfSearch:       100,
			MaxElements:    100000,
		},
		WAL: WALConfig{
			Enabled:       true,
			Dir:           "./data/wal",
			SegmentSizeMB: 64,
			SyncOnWrite:   true,
		},
		Snapshot: SnapshotConfig{
			Dir:            "./data/snapshots",
			IntervalSec:    3600,
			RetainCount:    5,
			CompressionLvl: 6,
		},
	}
}

// Load reads configuration from a JSON file
func Load(path string) (*Config, error) {
	// #nosec G304 - path is provided by system administrator at startup
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, err
	}

	cfg := DefaultConfig()

	// Accept both JSON and YAML. The config struct is tagged json only, so
	// YAML is bridged through JSON to honor those tags rather than falling
	// back to yaml's lowercased-field matching, which would silently drop
	// fields like rest_address. The shipped config.example.yaml and every
	// tutorial use YAML, so this path must work.
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		var intermediate map[string]interface{}
		if err := yaml.Unmarshal(data, &intermediate); err != nil {
			return nil, fmt.Errorf("parse yaml config: %w", err)
		}
		jsonData, err := json.Marshal(intermediate)
		if err != nil {
			return nil, fmt.Errorf("convert yaml config: %w", err)
		}
		if err := json.Unmarshal(jsonData, cfg); err != nil {
			return nil, fmt.Errorf("load yaml config: %w", err)
		}
	default:
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("load json config: %w", err)
		}
	}

	return cfg, nil
}

// Save writes configuration to a JSON file
func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// VectorConfig holds configuration for a single vector type
type VectorConfig struct {
	Dimension    int                  `json:"dimension"`
	Metric       MetricType           `json:"metric"`
	HNSW         HNSWConfig           `json:"hnsw"`
	OnDisk       bool                 `json:"on_disk"`
	Quantization *quantization.Config `json:"quantization,omitempty"`
}

// CollectionConfig holds per-collection configuration
type CollectionConfig struct {
	Name          string               `json:"name"`
	Dimension     int                  `json:"dimension"` // Default vector dimension (backwards compat)
	Metric        MetricType           `json:"metric"`    // Default vector metric (backwards compat)
	HNSW          HNSWConfig           `json:"hnsw"`      // Default HNSW config (backwards compat)
	OnDisk        bool                 `json:"on_disk"`
	PayloadSchema interface{}          `json:"payload_schema,omitempty"`
	Quantization  *quantization.Config `json:"quantization,omitempty"`

	// Named vectors support - each key is a vector name with its own config
	Vectors map[string]VectorConfig `json:"vectors,omitempty"`

	// Aliases for this collection
	Aliases []string `json:"aliases,omitempty"`

	// Sharding configuration
	ShardCount        int `json:"shard_count,omitempty"`
	ReplicationFactor int `json:"replication_factor,omitempty"`
}

// MetricType represents the distance metric to use
type MetricType string

const (
	MetricCosine     MetricType = "cosine"
	MetricEuclidean  MetricType = "euclidean"
	MetricDotProduct MetricType = "dot_product"
)

// Validate checks if the configuration is valid
func (c *CollectionConfig) Validate() error {
	if c.Name == "" {
		return ErrInvalidCollectionName
	}

	// If using named vectors, validate each one
	if len(c.Vectors) > 0 {
		for name, vc := range c.Vectors {
			if vc.Dimension <= 0 || vc.Dimension > 65536 {
				return ConfigError("vector '" + name + "': " + string(ErrInvalidDimension))
			}
			switch vc.Metric {
			case MetricCosine, MetricEuclidean, MetricDotProduct:
				// valid
			case "":
				// Default to cosine if not specified
			default:
				return ConfigError("vector '" + name + "': " + string(ErrInvalidMetric))
			}
		}
		return nil
	}

	// Legacy single-vector validation
	if c.Dimension <= 0 || c.Dimension > 65536 {
		return ErrInvalidDimension
	}
	switch c.Metric {
	case MetricCosine, MetricEuclidean, MetricDotProduct:
		// valid
	default:
		return ErrInvalidMetric
	}
	return nil
}

// HasNamedVectors returns true if the collection uses named vectors
func (c *CollectionConfig) HasNamedVectors() bool {
	return len(c.Vectors) > 0
}

// GetVectorConfig returns the vector configuration for a given name
// For collections without named vectors, returns the default config
func (c *CollectionConfig) GetVectorConfig(name string) *VectorConfig {
	if len(c.Vectors) > 0 {
		if vc, ok := c.Vectors[name]; ok {
			return &vc
		}
		return nil
	}
	// Return default config for legacy single-vector collections
	if name == "" || name == "default" {
		return &VectorConfig{
			Dimension: c.Dimension,
			Metric:    c.Metric,
			HNSW:      c.HNSW,
			OnDisk:    c.OnDisk,
		}
	}
	return nil
}

// VectorNames returns all vector names in the collection
func (c *CollectionConfig) VectorNames() []string {
	if len(c.Vectors) > 0 {
		names := make([]string, 0, len(c.Vectors))
		for name := range c.Vectors {
			names = append(names, name)
		}
		return names
	}
	return []string{"default"}
}

// Error types
type ConfigError string

func (e ConfigError) Error() string { return string(e) }

const (
	ErrInvalidCollectionName ConfigError = "invalid collection name"
	ErrInvalidDimension      ConfigError = "dimension must be between 1 and 65536"
	ErrInvalidMetric         ConfigError = "invalid metric type"
)
