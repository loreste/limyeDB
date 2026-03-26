package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	// Server defaults
	if cfg.Server.RESTAddress != ":8080" {
		t.Errorf("Server.RESTAddress = %s, want :8080", cfg.Server.RESTAddress)
	}
	if cfg.Server.GRPCAddress != ":50051" {
		t.Errorf("Server.GRPCAddress = %s, want :50051", cfg.Server.GRPCAddress)
	}
	if cfg.Server.ReadTimeout != 30*time.Second {
		t.Errorf("Server.ReadTimeout = %v, want 30s", cfg.Server.ReadTimeout)
	}
	if cfg.Server.WriteTimeout != 30*time.Second {
		t.Errorf("Server.WriteTimeout = %v, want 30s", cfg.Server.WriteTimeout)
	}
	if cfg.Server.MaxRequestSize != 64*1024*1024 {
		t.Errorf("Server.MaxRequestSize = %d, want 64MB", cfg.Server.MaxRequestSize)
	}

	// Storage defaults
	if cfg.Storage.DataDir != "./data" {
		t.Errorf("Storage.DataDir = %s, want ./data", cfg.Storage.DataDir)
	}
	if cfg.Storage.MaxCollections != 1000 {
		t.Errorf("Storage.MaxCollections = %d, want 1000", cfg.Storage.MaxCollections)
	}
	if !cfg.Storage.MmapEnabled {
		t.Error("Storage.MmapEnabled should be true")
	}

	// HNSW defaults
	if cfg.HNSW.M != 16 {
		t.Errorf("HNSW.M = %d, want 16", cfg.HNSW.M)
	}
	if cfg.HNSW.EfConstruction != 200 {
		t.Errorf("HNSW.EfConstruction = %d, want 200", cfg.HNSW.EfConstruction)
	}
	if cfg.HNSW.EfSearch != 100 {
		t.Errorf("HNSW.EfSearch = %d, want 100", cfg.HNSW.EfSearch)
	}

	// WAL defaults
	if !cfg.WAL.Enabled {
		t.Error("WAL.Enabled should be true")
	}
	if cfg.WAL.SegmentSizeMB != 64 {
		t.Errorf("WAL.SegmentSizeMB = %d, want 64", cfg.WAL.SegmentSizeMB)
	}

	// Snapshot defaults
	if cfg.Snapshot.RetainCount != 5 {
		t.Errorf("Snapshot.RetainCount = %d, want 5", cfg.Snapshot.RetainCount)
	}
}

func TestLoadConfigFileNotExists(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Should return default config when file doesn't exist
	defaultCfg := DefaultConfig()
	if cfg.Server.RESTAddress != defaultCfg.Server.RESTAddress {
		t.Error("Should return default config when file doesn't exist")
	}
}

func TestLoadConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	customConfig := &Config{
		Server: ServerConfig{
			RESTAddress:    ":9090",
			GRPCAddress:    ":50052",
			ReadTimeout:    60 * time.Second,
			WriteTimeout:   60 * time.Second,
			MaxRequestSize: 128 * 1024 * 1024,
		},
		Storage: StorageConfig{
			DataDir:        "/custom/data",
			MaxCollections: 500,
			MmapEnabled:    false,
		},
		HNSW: HNSWConfig{
			M:              32,
			EfConstruction: 400,
			EfSearch:       200,
			MaxElements:    500000,
		},
		WAL: WALConfig{
			Enabled:       false,
			Dir:           "/custom/wal",
			SegmentSizeMB: 128,
			SyncOnWrite:   false,
		},
		Snapshot: SnapshotConfig{
			Dir:            "/custom/snapshots",
			IntervalSec:    7200,
			RetainCount:    10,
			CompressionLvl: 9,
		},
	}

	data, err := json.MarshalIndent(customConfig, "", "  ")
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.Server.RESTAddress != ":9090" {
		t.Errorf("Server.RESTAddress = %s, want :9090", loaded.Server.RESTAddress)
	}
	if loaded.Storage.MaxCollections != 500 {
		t.Errorf("Storage.MaxCollections = %d, want 500", loaded.Storage.MaxCollections)
	}
	if loaded.HNSW.M != 32 {
		t.Errorf("HNSW.M = %d, want 32", loaded.HNSW.M)
	}
}

func TestLoadConfigInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "invalid.json")

	if err := os.WriteFile(configPath, []byte("not valid json"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("Load() should fail for invalid JSON")
	}
}

func TestSaveConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "save_test.json")

	cfg := DefaultConfig()
	cfg.Server.RESTAddress = ":7777"

	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file should exist after save")
	}

	// Load and verify
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.Server.RESTAddress != ":7777" {
		t.Errorf("Loaded RESTAddress = %s, want :7777", loaded.Server.RESTAddress)
	}
}

func TestCollectionConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *CollectionConfig
		wantErr bool
	}{
		{
			name: "valid_single_vector",
			config: &CollectionConfig{
				Name:      "valid_collection",
				Dimension: 128,
				Metric:    MetricCosine,
			},
			wantErr: false,
		},
		{
			name: "valid_euclidean",
			config: &CollectionConfig{
				Name:      "euclidean_collection",
				Dimension: 256,
				Metric:    MetricEuclidean,
			},
			wantErr: false,
		},
		{
			name: "valid_dot_product",
			config: &CollectionConfig{
				Name:      "dot_product_collection",
				Dimension: 512,
				Metric:    MetricDotProduct,
			},
			wantErr: false,
		},
		{
			name: "empty_name",
			config: &CollectionConfig{
				Name:      "",
				Dimension: 128,
				Metric:    MetricCosine,
			},
			wantErr: true,
		},
		{
			name: "zero_dimension",
			config: &CollectionConfig{
				Name:      "zero_dim",
				Dimension: 0,
				Metric:    MetricCosine,
			},
			wantErr: true,
		},
		{
			name: "negative_dimension",
			config: &CollectionConfig{
				Name:      "negative_dim",
				Dimension: -1,
				Metric:    MetricCosine,
			},
			wantErr: true,
		},
		{
			name: "dimension_too_large",
			config: &CollectionConfig{
				Name:      "too_large",
				Dimension: 65537,
				Metric:    MetricCosine,
			},
			wantErr: true,
		},
		{
			name: "max_valid_dimension",
			config: &CollectionConfig{
				Name:      "max_dim",
				Dimension: 65536,
				Metric:    MetricCosine,
			},
			wantErr: false,
		},
		{
			name: "invalid_metric",
			config: &CollectionConfig{
				Name:      "invalid_metric",
				Dimension: 128,
				Metric:    "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCollectionConfigValidateNamedVectors(t *testing.T) {
	tests := []struct {
		name    string
		config  *CollectionConfig
		wantErr bool
	}{
		{
			name: "valid_named_vectors",
			config: &CollectionConfig{
				Name: "multi_vector",
				Vectors: map[string]VectorConfig{
					"text":  {Dimension: 768, Metric: MetricCosine},
					"image": {Dimension: 512, Metric: MetricEuclidean},
				},
			},
			wantErr: false,
		},
		{
			name: "named_vector_empty_metric",
			config: &CollectionConfig{
				Name: "default_metric",
				Vectors: map[string]VectorConfig{
					"default": {Dimension: 128, Metric: ""},
				},
			},
			wantErr: false, // Empty metric defaults to cosine
		},
		{
			name: "named_vector_invalid_dimension",
			config: &CollectionConfig{
				Name: "invalid_named",
				Vectors: map[string]VectorConfig{
					"bad": {Dimension: 0, Metric: MetricCosine},
				},
			},
			wantErr: true,
		},
		{
			name: "named_vector_invalid_metric",
			config: &CollectionConfig{
				Name: "invalid_metric_named",
				Vectors: map[string]VectorConfig{
					"bad": {Dimension: 128, Metric: "invalid"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHasNamedVectors(t *testing.T) {
	tests := []struct {
		name   string
		config *CollectionConfig
		want   bool
	}{
		{
			name: "with_named_vectors",
			config: &CollectionConfig{
				Name: "named",
				Vectors: map[string]VectorConfig{
					"text": {Dimension: 768},
				},
			},
			want: true,
		},
		{
			name: "without_named_vectors",
			config: &CollectionConfig{
				Name:      "single",
				Dimension: 128,
			},
			want: false,
		},
		{
			name: "empty_vectors_map",
			config: &CollectionConfig{
				Name:    "empty",
				Vectors: map[string]VectorConfig{},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.HasNamedVectors()
			if got != tt.want {
				t.Errorf("HasNamedVectors() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetVectorConfig(t *testing.T) {
	// Named vectors collection
	namedConfig := &CollectionConfig{
		Name: "named",
		Vectors: map[string]VectorConfig{
			"text":  {Dimension: 768, Metric: MetricCosine},
			"image": {Dimension: 512, Metric: MetricEuclidean},
		},
	}

	// Single vector collection
	singleConfig := &CollectionConfig{
		Name:      "single",
		Dimension: 128,
		Metric:    MetricDotProduct,
	}

	t.Run("get_existing_named_vector", func(t *testing.T) {
		vc := namedConfig.GetVectorConfig("text")
		if vc == nil {
			t.Fatal("GetVectorConfig(text) returned nil")
		}
		if vc.Dimension != 768 {
			t.Errorf("Dimension = %d, want 768", vc.Dimension)
		}
		if vc.Metric != MetricCosine {
			t.Errorf("Metric = %s, want cosine", vc.Metric)
		}
	})

	t.Run("get_nonexistent_named_vector", func(t *testing.T) {
		vc := namedConfig.GetVectorConfig("nonexistent")
		if vc != nil {
			t.Error("GetVectorConfig(nonexistent) should return nil")
		}
	})

	t.Run("get_default_from_single_vector", func(t *testing.T) {
		vc := singleConfig.GetVectorConfig("")
		if vc == nil {
			t.Fatal("GetVectorConfig('') returned nil for single vector config")
		}
		if vc.Dimension != 128 {
			t.Errorf("Dimension = %d, want 128", vc.Dimension)
		}
	})

	t.Run("get_default_explicit_name", func(t *testing.T) {
		vc := singleConfig.GetVectorConfig("default")
		if vc == nil {
			t.Fatal("GetVectorConfig(default) returned nil")
		}
		if vc.Dimension != 128 {
			t.Errorf("Dimension = %d, want 128", vc.Dimension)
		}
	})

	t.Run("get_nondefault_from_single_vector", func(t *testing.T) {
		vc := singleConfig.GetVectorConfig("nonexistent")
		if vc != nil {
			t.Error("GetVectorConfig(nonexistent) should return nil for single vector config")
		}
	})
}

func TestVectorNames(t *testing.T) {
	t.Run("named_vectors", func(t *testing.T) {
		config := &CollectionConfig{
			Name: "named",
			Vectors: map[string]VectorConfig{
				"text":  {Dimension: 768},
				"image": {Dimension: 512},
				"audio": {Dimension: 256},
			},
		}

		names := config.VectorNames()
		if len(names) != 3 {
			t.Errorf("VectorNames() returned %d names, want 3", len(names))
		}

		nameMap := make(map[string]bool)
		for _, n := range names {
			nameMap[n] = true
		}

		for _, expected := range []string{"text", "image", "audio"} {
			if !nameMap[expected] {
				t.Errorf("VectorNames() missing %s", expected)
			}
		}
	})

	t.Run("single_vector", func(t *testing.T) {
		config := &CollectionConfig{
			Name:      "single",
			Dimension: 128,
		}

		names := config.VectorNames()
		if len(names) != 1 {
			t.Errorf("VectorNames() returned %d names, want 1", len(names))
		}
		if names[0] != "default" {
			t.Errorf("VectorNames()[0] = %s, want default", names[0])
		}
	})
}

func TestConfigErrors(t *testing.T) {
	t.Run("error_string", func(t *testing.T) {
		err := ErrInvalidCollectionName
		if err.Error() != "invalid collection name" {
			t.Errorf("Error() = %s", err.Error())
		}
	})

	t.Run("dimension_error", func(t *testing.T) {
		err := ErrInvalidDimension
		if err.Error() != "dimension must be between 1 and 65536" {
			t.Errorf("Error() = %s", err.Error())
		}
	})

	t.Run("metric_error", func(t *testing.T) {
		err := ErrInvalidMetric
		if err.Error() != "invalid metric type" {
			t.Errorf("Error() = %s", err.Error())
		}
	})
}

func TestMetricTypes(t *testing.T) {
	tests := []struct {
		metric MetricType
		want   string
	}{
		{MetricCosine, "cosine"},
		{MetricEuclidean, "euclidean"},
		{MetricDotProduct, "dot_product"},
	}

	for _, tt := range tests {
		t.Run(string(tt.metric), func(t *testing.T) {
			if string(tt.metric) != tt.want {
				t.Errorf("MetricType = %s, want %s", tt.metric, tt.want)
			}
		})
	}
}

func TestConfigJSONRoundTrip(t *testing.T) {
	original := &CollectionConfig{
		Name:      "test_collection",
		Dimension: 256,
		Metric:    MetricCosine,
		HNSW: HNSWConfig{
			M:              32,
			EfConstruction: 400,
			EfSearch:       200,
			MaxElements:    1000000,
		},
		OnDisk:            true,
		ShardCount:        4,
		ReplicationFactor: 3,
		Aliases:           []string{"alias1", "alias2"},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var loaded CollectionConfig
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if loaded.Name != original.Name {
		t.Errorf("Name mismatch: got %s, want %s", loaded.Name, original.Name)
	}
	if loaded.Dimension != original.Dimension {
		t.Errorf("Dimension mismatch: got %d, want %d", loaded.Dimension, original.Dimension)
	}
	if loaded.HNSW.M != original.HNSW.M {
		t.Errorf("HNSW.M mismatch: got %d, want %d", loaded.HNSW.M, original.HNSW.M)
	}
	if len(loaded.Aliases) != len(original.Aliases) {
		t.Errorf("Aliases count mismatch: got %d, want %d", len(loaded.Aliases), len(original.Aliases))
	}
}

func BenchmarkValidate(b *testing.B) {
	config := &CollectionConfig{
		Name:      "benchmark",
		Dimension: 768,
		Metric:    MetricCosine,
		HNSW: HNSWConfig{
			M:              16,
			EfConstruction: 200,
			EfSearch:       100,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.Validate()
	}
}

func BenchmarkGetVectorConfig(b *testing.B) {
	config := &CollectionConfig{
		Name: "benchmark",
		Vectors: map[string]VectorConfig{
			"text":  {Dimension: 768, Metric: MetricCosine},
			"image": {Dimension: 512, Metric: MetricEuclidean},
			"audio": {Dimension: 256, Metric: MetricDotProduct},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.GetVectorConfig("image")
	}
}
