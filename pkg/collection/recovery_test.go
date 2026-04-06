package collection

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/point"
	"github.com/limyedb/limyedb/pkg/storage/wal"
)

func TestRecoveryWithWAL(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "limyedb-recovery-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	collDir := filepath.Join(tmpDir, "collections")
	walDir := filepath.Join(tmpDir, "wal")

	// Phase 1: Create manager with WAL, insert data, shutdown gracefully
	t.Run("InsertAndShutdown", func(t *testing.T) {
		// Initialize WAL
		walInstance, err := wal.Open(&wal.Config{
			Dir:         walDir,
			SegmentSize: 64 * 1024 * 1024,
			SyncOnWrite: true,
		})
		if err != nil {
			t.Fatalf("Failed to create WAL: %v", err)
		}

		// Create manager
		mgr, err := NewManager(&ManagerConfig{
			DataDir:        collDir,
			MaxCollections: 100,
			WAL:            walInstance,
		})
		if err != nil {
			t.Fatalf("Failed to create manager: %v", err)
		}

		// Create collection
		cfg := &config.CollectionConfig{
			Name:      "test_collection",
			Dimension: 128,
			Metric:    config.MetricCosine,
		}
		coll, err := mgr.Create(cfg)
		if err != nil {
			t.Fatalf("Failed to create collection: %v", err)
		}

		// Insert points
		for i := 0; i < 100; i++ {
			vec := make(point.Vector, 128)
			for j := range vec {
				vec[j] = float32(i*128 + j)
			}
			p := point.NewPoint(vec, map[string]interface{}{"index": i})
			if err := coll.Insert(p); err != nil {
				t.Fatalf("Failed to insert point %d: %v", i, err)
			}
		}

		// Verify data is there
		if coll.Size() != 100 {
			t.Errorf("Expected 100 points, got %d", coll.Size())
		}

		// Graceful shutdown
		if err := walInstance.Sync(); err != nil {
			t.Errorf("Failed to sync WAL: %v", err)
		}
		if err := mgr.SaveAllIndexMetadata(); err != nil {
			t.Errorf("Failed to save index metadata: %v", err)
		}
		if err := mgr.Close(); err != nil {
			t.Errorf("Failed to close manager: %v", err)
		}
		if err := walInstance.Close(); err != nil {
			t.Errorf("Failed to close WAL: %v", err)
		}
	})

	// Phase 2: Restart and verify data is recovered
	t.Run("RecoverAndVerify", func(t *testing.T) {
		// Reinitialize WAL
		walInstance, err := wal.Open(&wal.Config{
			Dir:         walDir,
			SegmentSize: 64 * 1024 * 1024,
			SyncOnWrite: true,
		})
		if err != nil {
			t.Fatalf("Failed to reopen WAL: %v", err)
		}
		defer walInstance.Close()

		// Create manager (this loads collections)
		mgr, err := NewManager(&ManagerConfig{
			DataDir:        collDir,
			MaxCollections: 100,
			WAL:            walInstance,
		})
		if err != nil {
			t.Fatalf("Failed to create manager: %v", err)
		}
		defer mgr.Close()

		// Run recovery
		if err := mgr.Recover(); err != nil {
			t.Fatalf("Recovery failed: %v", err)
		}

		// Verify collection exists
		coll, err := mgr.Get("test_collection")
		if err != nil {
			t.Fatalf("Failed to get collection: %v", err)
		}

		// Verify data was recovered
		if coll.Size() != 100 {
			t.Errorf("Expected 100 points after recovery, got %d", coll.Size())
		}
	})
}

func TestWALReplayWithDeleteOperations(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "limyedb-wal-delete-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	collDir := filepath.Join(tmpDir, "collections")
	walDir := filepath.Join(tmpDir, "wal")

	var deletedID string

	// Phase 1: Insert, then delete some points
	t.Run("InsertDeleteAndShutdown", func(t *testing.T) {
		walInstance, err := wal.Open(&wal.Config{
			Dir:         walDir,
			SegmentSize: 64 * 1024 * 1024,
			SyncOnWrite: true,
		})
		if err != nil {
			t.Fatalf("Failed to create WAL: %v", err)
		}

		mgr, err := NewManager(&ManagerConfig{
			DataDir:        collDir,
			MaxCollections: 100,
			WAL:            walInstance,
		})
		if err != nil {
			t.Fatalf("Failed to create manager: %v", err)
		}

		cfg := &config.CollectionConfig{
			Name:      "delete_test",
			Dimension: 64,
			Metric:    config.MetricEuclidean,
		}
		coll, err := mgr.Create(cfg)
		if err != nil {
			t.Fatalf("Failed to create collection: %v", err)
		}

		// Insert points
		var pointIDs []string
		for i := 0; i < 10; i++ {
			vec := make(point.Vector, 64)
			for j := range vec {
				vec[j] = float32(i + j)
			}
			p := point.NewPoint(vec, nil)
			pointIDs = append(pointIDs, p.ID)
			if err := coll.Insert(p); err != nil {
				t.Fatalf("Failed to insert point: %v", err)
			}
		}

		// Delete one point
		deletedID = pointIDs[5]
		if err := coll.Delete(deletedID); err != nil {
			t.Fatalf("Failed to delete point: %v", err)
		}

		// Verify deletion
		if coll.Size() != 9 {
			t.Errorf("Expected 9 points after delete, got %d", coll.Size())
		}

		// Shutdown
		_ = walInstance.Sync()
		_ = mgr.SaveAllIndexMetadata()
		_ = mgr.Close()
		_ = walInstance.Close()
	})

	// Phase 2: Recover and verify deletion persisted
	t.Run("RecoverAndVerifyDeletion", func(t *testing.T) {
		walInstance, err := wal.Open(&wal.Config{
			Dir:         walDir,
			SegmentSize: 64 * 1024 * 1024,
			SyncOnWrite: true,
		})
		if err != nil {
			t.Fatalf("Failed to reopen WAL: %v", err)
		}
		defer walInstance.Close()

		mgr, err := NewManager(&ManagerConfig{
			DataDir:        collDir,
			MaxCollections: 100,
			WAL:            walInstance,
		})
		if err != nil {
			t.Fatalf("Failed to create manager: %v", err)
		}
		defer mgr.Close()

		if err := mgr.Recover(); err != nil {
			t.Fatalf("Recovery failed: %v", err)
		}

		coll, err := mgr.Get("delete_test")
		if err != nil {
			t.Fatalf("Failed to get collection: %v", err)
		}

		// Verify size after recovery
		if coll.Size() != 9 {
			t.Errorf("Expected 9 points after recovery, got %d", coll.Size())
		}

		// Verify deleted point is not found
		_, err = coll.Get(deletedID)
		if err == nil {
			t.Error("Expected deleted point to not be found")
		}
	})
}

func TestHNSWMetadataPersistence(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "limyedb-hnsw-meta-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	collDir := filepath.Join(tmpDir, "collections")

	// Phase 1: Create collection, insert data, save metadata
	t.Run("CreateAndSaveMetadata", func(t *testing.T) {
		mgr, err := NewManager(&ManagerConfig{
			DataDir:        collDir,
			MaxCollections: 100,
		})
		if err != nil {
			t.Fatalf("Failed to create manager: %v", err)
		}

		cfg := &config.CollectionConfig{
			Name:      "meta_test",
			Dimension: 32,
			Metric:    config.MetricCosine,
		}
		coll, err := mgr.Create(cfg)
		if err != nil {
			t.Fatalf("Failed to create collection: %v", err)
		}

		// Insert points
		for i := 0; i < 50; i++ {
			vec := make(point.Vector, 32)
			for j := range vec {
				vec[j] = float32(i*32 + j)
			}
			p := point.NewPoint(vec, map[string]interface{}{"idx": i})
			if err := coll.Insert(p); err != nil {
				t.Fatalf("Failed to insert point: %v", err)
			}
		}

		// Save metadata
		if err := mgr.SaveAllIndexMetadata(); err != nil {
			t.Fatalf("Failed to save metadata: %v", err)
		}

		// Verify metadata file exists
		metaPath := filepath.Join(collDir, "meta_test", "index.meta")
		if _, err := os.Stat(metaPath); os.IsNotExist(err) {
			t.Error("Index metadata file was not created")
		}

		_ = mgr.Close()
	})

	// Phase 2: Load metadata and verify index state
	t.Run("LoadAndVerifyMetadata", func(t *testing.T) {
		mgr, err := NewManager(&ManagerConfig{
			DataDir:        collDir,
			MaxCollections: 100,
		})
		if err != nil {
			t.Fatalf("Failed to create manager: %v", err)
		}
		defer mgr.Close()

		// Run recovery (loads index metadata)
		if err := mgr.Recover(); err != nil {
			t.Fatalf("Recovery failed: %v", err)
		}

		coll, err := mgr.Get("meta_test")
		if err != nil {
			t.Fatalf("Failed to get collection: %v", err)
		}

		// Note: Without WAL, vectors won't be restored, only the graph structure
		// This test verifies metadata loading works
		info := coll.Info()
		if info.Name != "meta_test" {
			t.Errorf("Expected collection name 'meta_test', got '%s'", info.Name)
		}
	})
}

func TestSerializeDeserializePoint(t *testing.T) {
	// Test basic point
	p := &point.Point{
		ID:      "test-point-123",
		Vector:  point.Vector{1.0, 2.0, 3.0, 4.0},
		Payload: map[string]interface{}{"key": "value", "num": float64(42)},
	}

	data, err := serializePoint(p)
	if err != nil {
		t.Fatalf("Failed to serialize point: %v", err)
	}

	restored, err := deserializePoint(data)
	if err != nil {
		t.Fatalf("Failed to deserialize point: %v", err)
	}

	if restored.ID != p.ID {
		t.Errorf("ID mismatch: expected %s, got %s", p.ID, restored.ID)
	}

	if len(restored.Vector) != len(p.Vector) {
		t.Errorf("Vector length mismatch: expected %d, got %d", len(p.Vector), len(restored.Vector))
	}

	for i := range p.Vector {
		if restored.Vector[i] != p.Vector[i] {
			t.Errorf("Vector[%d] mismatch: expected %f, got %f", i, p.Vector[i], restored.Vector[i])
		}
	}

	if restored.Payload["key"] != p.Payload["key"] {
		t.Errorf("Payload key mismatch")
	}
}
