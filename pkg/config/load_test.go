package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadYAML covers the YAML path added so the shipped config.example.yaml
// and the tutorials, which are all YAML, actually load. Before this, Load only
// did json.Unmarshal, so a YAML file either errored or silently produced an
// empty config.
func TestLoadYAML(t *testing.T) {
	yaml := `
server:
  rest_address: ":9090"
  grpc_address: ":60000"
storage:
  data_dir: "/var/lib/limyedb"
  mmap_enabled: true
hnsw:
  m: 32
  ef_construction: 250
`
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load yaml: %v", err)
	}

	// The struct is tagged json only, so this asserts the YAML->JSON bridge
	// honored snake_case field names rather than dropping them.
	if cfg.Server.RESTAddress != ":9090" {
		t.Errorf("RESTAddress = %q, want :9090", cfg.Server.RESTAddress)
	}
	if cfg.Server.GRPCAddress != ":60000" {
		t.Errorf("GRPCAddress = %q, want :60000", cfg.Server.GRPCAddress)
	}
	if cfg.Storage.DataDir != "/var/lib/limyedb" {
		t.Errorf("DataDir = %q, want /var/lib/limyedb", cfg.Storage.DataDir)
	}
	if cfg.HNSW.M != 32 {
		t.Errorf("HNSW.M = %d, want 32", cfg.HNSW.M)
	}
	if cfg.HNSW.EfConstruction != 250 {
		t.Errorf("HNSW.EfConstruction = %d, want 250", cfg.HNSW.EfConstruction)
	}
}

func TestLoadJSON(t *testing.T) {
	js := `{"server":{"rest_address":":7000"},"hnsw":{"m":8}}`
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(js), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load json: %v", err)
	}
	if cfg.Server.RESTAddress != ":7000" {
		t.Errorf("RESTAddress = %q, want :7000", cfg.Server.RESTAddress)
	}
	if cfg.HNSW.M != 8 {
		t.Errorf("HNSW.M = %d, want 8", cfg.HNSW.M)
	}
}

// TestLoadShippedExampleYAML is the regression guard: the config.example.yaml
// checked into the repo must actually load. It previously could not.
func TestLoadShippedExampleYAML(t *testing.T) {
	path := filepath.Join("..", "..", "config.example.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("example config not found at %s", path)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("shipped config.example.yaml failed to load: %v", err)
	}
	if cfg.Server.RESTAddress == "" {
		t.Error("RESTAddress empty after loading the shipped example; YAML fields not mapped")
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("missing file should return defaults, got error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected default config, got nil")
	}
}
