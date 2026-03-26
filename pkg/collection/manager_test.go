package collection

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/limyedb/limyedb/pkg/config"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Valid names
		{"simple", "myCollection", false},
		{"with_underscore", "my_collection", false},
		{"with_hyphen", "my-collection", false},
		{"with_numbers", "collection123", false},
		{"alphanumeric", "col1_test-2", false},
		{"uppercase", "MyCollection", false},
		{"single_char", "a", false},

		// Invalid names
		{"empty", "", true},
		{"starts_with_underscore", "_collection", true},
		{"starts_with_hyphen", "-collection", true},
		{"starts_with_number", "1collection", false}, // Numbers at start are allowed by current regex
		{"path_traversal", "../etc/passwd", true},
		{"path_traversal_windows", "..\\windows\\system32", true},
		{"contains_slash", "my/collection", true},
		{"contains_backslash", "my\\collection", true},
		{"contains_dot_dot", "my..collection", true},
		{"contains_space", "my collection", true},
		{"contains_special", "my@collection", true},
		{"contains_asterisk", "my*collection", true},
		{"too_long", string(make([]byte, 256)), true},
		{"max_length", string(make([]byte, 255)), true}, // 255 is valid if all chars are valid
	}

	// Fix the max_length test - it needs valid characters
	for i := range tests {
		if tests[i].name == "max_length" {
			validChars := make([]byte, 255)
			validChars[0] = 'a' // Must start with alphanumeric
			for j := 1; j < 255; j++ {
				validChars[j] = 'b'
			}
			tests[i].input = string(validChars)
			tests[i].wantErr = false
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestPathTraversalPrevention(t *testing.T) {
	// Additional path traversal attack vectors
	attacks := []string{
		"../../../etc/passwd",
		"..%2F..%2F..%2Fetc/passwd",
		"....//....//etc/passwd",
		"..\\..\\..",
		"..;/etc/passwd",
		"collection/../../../etc",
		"collection/../../..",
		"..%c0%af",
		"..%252f",
	}

	for _, attack := range attacks {
		t.Run(attack, func(t *testing.T) {
			err := validateName(attack)
			if err == nil {
				t.Errorf("validateName(%q) should have failed for path traversal", attack)
			}
		})
	}
}

func TestManagerCreate(t *testing.T) {
	dir := t.TempDir()

	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 100,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	cfg := &config.CollectionConfig{
		Name:      "test_collection",
		Dimension: 128,
		Metric:    config.MetricCosine,
	}

	coll, err := mgr.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if coll.Name() != "test_collection" {
		t.Errorf("Collection name = %s, want test_collection", coll.Name())
	}

	if coll.Dimension() != 128 {
		t.Errorf("Collection dimension = %d, want 128", coll.Dimension())
	}
}

func TestManagerCreateDuplicate(t *testing.T) {
	dir := t.TempDir()

	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 100,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	cfg := &config.CollectionConfig{
		Name:      "duplicate",
		Dimension: 128,
		Metric:    config.MetricCosine,
	}

	_, err = mgr.Create(cfg)
	if err != nil {
		t.Fatalf("First Create() error = %v", err)
	}

	_, err = mgr.Create(cfg)
	if err != ErrCollectionExists {
		t.Errorf("Second Create() error = %v, want ErrCollectionExists", err)
	}
}

func TestManagerCreateInvalidName(t *testing.T) {
	dir := t.TempDir()

	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 100,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	cfg := &config.CollectionConfig{
		Name:      "../invalid",
		Dimension: 128,
		Metric:    config.MetricCosine,
	}

	_, err = mgr.Create(cfg)
	if err == nil {
		t.Error("Create() should fail with invalid name")
	}
}

func TestManagerGet(t *testing.T) {
	dir := t.TempDir()

	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 100,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	cfg := &config.CollectionConfig{
		Name:      "gettest",
		Dimension: 64,
		Metric:    config.MetricEuclidean,
	}

	_, err = mgr.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	coll, err := mgr.Get("gettest")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if coll.Name() != "gettest" {
		t.Errorf("Collection name = %s, want gettest", coll.Name())
	}
}

func TestManagerGetNotFound(t *testing.T) {
	dir := t.TempDir()

	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 100,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	_, err = mgr.Get("nonexistent")
	if err != ErrCollectionNotFound {
		t.Errorf("Get() error = %v, want ErrCollectionNotFound", err)
	}
}

func TestManagerDelete(t *testing.T) {
	dir := t.TempDir()

	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 100,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	cfg := &config.CollectionConfig{
		Name:      "deletetest",
		Dimension: 128,
		Metric:    config.MetricCosine,
	}

	_, err = mgr.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	err = mgr.Delete("deletetest")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if mgr.Exists("deletetest") {
		t.Error("Collection should not exist after deletion")
	}

	// Verify directory is removed
	collDir := filepath.Join(dir, "deletetest")
	if _, err := os.Stat(collDir); !os.IsNotExist(err) {
		t.Error("Collection directory should be removed")
	}
}

func TestManagerDeleteNotFound(t *testing.T) {
	dir := t.TempDir()

	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 100,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	err = mgr.Delete("nonexistent")
	if err != ErrCollectionNotFound {
		t.Errorf("Delete() error = %v, want ErrCollectionNotFound", err)
	}
}

func TestManagerDeleteInvalidName(t *testing.T) {
	dir := t.TempDir()

	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 100,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	err = mgr.Delete("../invalid")
	if err == nil {
		t.Error("Delete() should fail with invalid name")
	}
}

func TestManagerList(t *testing.T) {
	dir := t.TempDir()

	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 100,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	// Create multiple collections
	names := []string{"col1", "col2", "col3"}
	for _, name := range names {
		cfg := &config.CollectionConfig{
			Name:      name,
			Dimension: 64,
			Metric:    config.MetricCosine,
		}
		if _, err := mgr.Create(cfg); err != nil {
			t.Fatalf("Create(%s) error = %v", name, err)
		}
	}

	list := mgr.List()
	if len(list) != 3 {
		t.Errorf("List() returned %d collections, want 3", len(list))
	}

	// Verify all names are present
	nameMap := make(map[string]bool)
	for _, name := range list {
		nameMap[name] = true
	}
	for _, name := range names {
		if !nameMap[name] {
			t.Errorf("Collection %s not found in list", name)
		}
	}
}

func TestManagerExists(t *testing.T) {
	dir := t.TempDir()

	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 100,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	cfg := &config.CollectionConfig{
		Name:      "existstest",
		Dimension: 128,
		Metric:    config.MetricCosine,
	}

	if mgr.Exists("existstest") {
		t.Error("Collection should not exist before creation")
	}

	if _, err := mgr.Create(cfg); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if !mgr.Exists("existstest") {
		t.Error("Collection should exist after creation")
	}
}

func TestManagerCount(t *testing.T) {
	dir := t.TempDir()

	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 100,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	if mgr.Count() != 0 {
		t.Errorf("Initial count = %d, want 0", mgr.Count())
	}

	for i := 0; i < 5; i++ {
		cfg := &config.CollectionConfig{
			Name:      "count" + string(rune('0'+i)),
			Dimension: 64,
			Metric:    config.MetricCosine,
		}
		if _, err := mgr.Create(cfg); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	if mgr.Count() != 5 {
		t.Errorf("Count after 5 creates = %d, want 5", mgr.Count())
	}
}

func TestManagerMaxCollections(t *testing.T) {
	dir := t.TempDir()

	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 3,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	// Create up to the limit
	for i := 0; i < 3; i++ {
		cfg := &config.CollectionConfig{
			Name:      "max" + string(rune('0'+i)),
			Dimension: 64,
			Metric:    config.MetricCosine,
		}
		if _, err := mgr.Create(cfg); err != nil {
			t.Fatalf("Create(%d) error = %v", i, err)
		}
	}

	// Try to exceed the limit
	cfg := &config.CollectionConfig{
		Name:      "max3",
		Dimension: 64,
		Metric:    config.MetricCosine,
	}
	_, err = mgr.Create(cfg)
	if err != ErrMaxCollections {
		t.Errorf("Create() error = %v, want ErrMaxCollections", err)
	}
}

func TestManagerConcurrentAccess(t *testing.T) {
	dir := t.TempDir()

	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 1000,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	const numGoroutines = 10
	const operationsPerGoroutine = 10

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*operationsPerGoroutine)

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for i := 0; i < operationsPerGoroutine; i++ {
				name := "concurrent" + string(rune('A'+goroutineID)) + string(rune('0'+i))

				// Create
				cfg := &config.CollectionConfig{
					Name:      name,
					Dimension: 64,
					Metric:    config.MetricCosine,
				}
				_, err := mgr.Create(cfg)
				if err != nil && err != ErrCollectionExists {
					errors <- err
					continue
				}

				// Get
				_, err = mgr.Get(name)
				if err != nil && err != ErrCollectionNotFound {
					errors <- err
				}

				// List
				_ = mgr.List()

				// Exists
				_ = mgr.Exists(name)

				// Count
				_ = mgr.Count()
			}
		}(g)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent operation error: %v", err)
	}
}

func TestManagerRename(t *testing.T) {
	dir := t.TempDir()

	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 100,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	cfg := &config.CollectionConfig{
		Name:      "original",
		Dimension: 128,
		Metric:    config.MetricCosine,
	}

	_, err = mgr.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	err = mgr.Rename("original", "renamed")
	if err != nil {
		t.Fatalf("Rename() error = %v", err)
	}

	if mgr.Exists("original") {
		t.Error("Original name should not exist after rename")
	}

	if !mgr.Exists("renamed") {
		t.Error("New name should exist after rename")
	}

	coll, err := mgr.Get("renamed")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if coll.Name() != "renamed" {
		t.Errorf("Collection name = %s, want renamed", coll.Name())
	}
}

func TestManagerRenameInvalidName(t *testing.T) {
	dir := t.TempDir()

	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 100,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	cfg := &config.CollectionConfig{
		Name:      "validname",
		Dimension: 128,
		Metric:    config.MetricCosine,
	}

	_, err = mgr.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	err = mgr.Rename("validname", "../invalid")
	if err == nil {
		t.Error("Rename() should fail with invalid new name")
	}
}

func TestManagerRenameNotFound(t *testing.T) {
	dir := t.TempDir()

	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 100,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	err = mgr.Rename("nonexistent", "newname")
	if err != ErrCollectionNotFound {
		t.Errorf("Rename() error = %v, want ErrCollectionNotFound", err)
	}
}

func TestManagerPersistence(t *testing.T) {
	dir := t.TempDir()

	// Create manager and add collections
	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 100,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	cfg := &config.CollectionConfig{
		Name:      "persistent",
		Dimension: 256,
		Metric:    config.MetricDotProduct,
	}

	_, err = mgr.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	err = mgr.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Reopen manager and verify persistence
	mgr2, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 100,
	})
	if err != nil {
		t.Fatalf("NewManager() reopen error = %v", err)
	}
	defer mgr2.Close()

	if !mgr2.Exists("persistent") {
		t.Error("Collection should persist across restarts")
	}

	coll, err := mgr2.Get("persistent")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if coll.Dimension() != 256 {
		t.Errorf("Persisted dimension = %d, want 256", coll.Dimension())
	}

	if coll.Metric() != config.MetricDotProduct {
		t.Errorf("Persisted metric = %s, want dot_product", coll.Metric())
	}
}

func TestManagerFlush(t *testing.T) {
	dir := t.TempDir()

	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 100,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	cfg := &config.CollectionConfig{
		Name:      "flushtest",
		Dimension: 128,
		Metric:    config.MetricCosine,
	}

	_, err = mgr.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	err = mgr.Flush()
	if err != nil {
		t.Errorf("Flush() error = %v", err)
	}

	// Verify metadata file exists
	metaPath := filepath.Join(dir, "flushtest", "meta.json")
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		t.Error("Metadata file should exist after flush")
	}
}

func TestDefaultManagerConfig(t *testing.T) {
	cfg := DefaultManagerConfig()

	if cfg.DataDir != "./data/collections" {
		t.Errorf("DefaultManagerConfig().DataDir = %s, want ./data/collections", cfg.DataDir)
	}
	if cfg.MaxCollections != 1000 {
		t.Errorf("DefaultManagerConfig().MaxCollections = %d, want 1000", cfg.MaxCollections)
	}
}

func TestManagerListInfo(t *testing.T) {
	dir := t.TempDir()

	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 100,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	// Create collections with different configurations
	configs := []*config.CollectionConfig{
		{Name: "info1", Dimension: 64, Metric: config.MetricCosine},
		{Name: "info2", Dimension: 128, Metric: config.MetricEuclidean},
		{Name: "info3", Dimension: 256, Metric: config.MetricDotProduct},
	}

	for _, cfg := range configs {
		if _, err := mgr.Create(cfg); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	infos := mgr.ListInfo()
	if len(infos) != 3 {
		t.Errorf("ListInfo() returned %d infos, want 3", len(infos))
	}

	// Verify info contents
	infoMap := make(map[string]*Info)
	for _, info := range infos {
		infoMap[info.Name] = info
	}

	if info, ok := infoMap["info1"]; ok {
		if info.Dimension != 64 {
			t.Errorf("info1 dimension = %d, want 64", info.Dimension)
		}
	} else {
		t.Error("info1 not found in ListInfo result")
	}
}

func TestManagerHNSWDefaults(t *testing.T) {
	dir := t.TempDir()

	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 100,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	// Create with minimal config (no HNSW params)
	cfg := &config.CollectionConfig{
		Name:      "defaults",
		Dimension: 128,
		Metric:    config.MetricCosine,
	}

	coll, err := mgr.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Verify HNSW defaults were applied
	collCfg := coll.Config()
	if collCfg.HNSW.M != 16 {
		t.Errorf("HNSW.M = %d, want 16", collCfg.HNSW.M)
	}
	if collCfg.HNSW.EfConstruction != 200 {
		t.Errorf("HNSW.EfConstruction = %d, want 200", collCfg.HNSW.EfConstruction)
	}
	if collCfg.HNSW.EfSearch != 100 {
		t.Errorf("HNSW.EfSearch = %d, want 100", collCfg.HNSW.EfSearch)
	}
	if collCfg.HNSW.MaxElements != 100000 {
		t.Errorf("HNSW.MaxElements = %d, want 100000", collCfg.HNSW.MaxElements)
	}
}

func BenchmarkManagerCreate(b *testing.B) {
	dir := b.TempDir()

	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: b.N + 100,
	})
	if err != nil {
		b.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cfg := &config.CollectionConfig{
			Name:      "bench" + string(rune(i%26+'a')) + string(rune(i/26+'0')),
			Dimension: 128,
			Metric:    config.MetricCosine,
		}
		if _, err := mgr.Create(cfg); err != nil && err != ErrCollectionExists {
			b.Fatalf("Create() error = %v", err)
		}
	}
}

func BenchmarkManagerGet(b *testing.B) {
	dir := b.TempDir()

	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 100,
	})
	if err != nil {
		b.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	cfg := &config.CollectionConfig{
		Name:      "benchget",
		Dimension: 128,
		Metric:    config.MetricCosine,
	}
	if _, err := mgr.Create(cfg); err != nil {
		b.Fatalf("Create() error = %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := mgr.Get("benchget"); err != nil {
			b.Fatalf("Get() error = %v", err)
		}
	}
}

func BenchmarkManagerList(b *testing.B) {
	dir := b.TempDir()

	mgr, err := NewManager(&ManagerConfig{
		DataDir:        dir,
		MaxCollections: 200,
	})
	if err != nil {
		b.Fatalf("NewManager() error = %v", err)
	}
	defer mgr.Close()

	// Create 100 collections
	for i := 0; i < 100; i++ {
		cfg := &config.CollectionConfig{
			Name:      "list" + string(rune(i%26+'a')) + string(rune(i/26+'0')),
			Dimension: 128,
			Metric:    config.MetricCosine,
		}
		if _, err := mgr.Create(cfg); err != nil {
			b.Fatalf("Create() error = %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mgr.List()
	}
}
