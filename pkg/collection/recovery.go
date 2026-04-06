package collection

import (
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/limyedb/limyedb/pkg/index/hnsw"
	"github.com/limyedb/limyedb/pkg/storage/wal"
)

// Recover performs startup recovery:
// 1. Loads collection metadata (existing loadCollections behavior)
// 2. Loads HNSW index state from disk
// 3. Replays WAL to restore any in-flight operations
func (m *Manager) Recover() error {
	slog.Info("Starting recovery process")

	// Step 1: Collections should already be loaded by NewManager
	// But we need to load index metadata for each collection
	m.mu.RLock()
	collections := make([]*Collection, 0, len(m.collections))
	for _, coll := range m.collections {
		collections = append(collections, coll)
	}
	m.mu.RUnlock()

	// Step 2: Load index state for each collection
	for _, coll := range collections {
		if err := m.loadIndexState(coll); err != nil {
			slog.Warn("Failed to load index state, will rebuild from WAL",
				"collection", coll.Name(),
				"error", err)
		}
	}

	// Step 3: Replay WAL if available
	if m.wal != nil {
		slog.Info("Replaying WAL")
		replayedCount, err := m.replayWAL()
		if err != nil {
			return fmt.Errorf("WAL replay failed: %w", err)
		}
		slog.Info("WAL replay complete", "records", replayedCount)

		// Set WAL for all collections after replay
		m.SetWALForCollections()
	}

	slog.Info("Recovery complete", "collections", len(collections))
	return nil
}

// loadIndexState loads the HNSW index metadata for a collection
func (m *Manager) loadIndexState(coll *Collection) error {
	if coll.config.HasNamedVectors() {
		// Load metadata for each named vector index
		for name := range coll.indices {
			metaPath := filepath.Join(m.dataDir, coll.Name(), name, "index.meta")
			if hnsw.HasMetadata(metaPath) {
				slog.Debug("Loading index metadata", "collection", coll.Name(), "vector", name)
				if err := coll.indices[name].LoadMetadata(metaPath); err != nil {
					return fmt.Errorf("failed to load index metadata for %s/%s: %w", coll.Name(), name, err)
				}
			}
		}
	} else if coll.index != nil {
		// Load metadata for default index
		metaPath := filepath.Join(m.dataDir, coll.Name(), "index.meta")
		if hnsw.HasMetadata(metaPath) {
			slog.Debug("Loading index metadata", "collection", coll.Name())
			if err := coll.index.LoadMetadata(metaPath); err != nil {
				return fmt.Errorf("failed to load index metadata for %s: %w", coll.Name(), err)
			}
		}
	}

	return nil
}

// replayWAL replays all records from the WAL to restore state
func (m *Manager) replayWAL() (int, error) {
	var replayedCount int

	err := m.wal.Replay(func(record *wal.Record) error {
		// Get collection (skip if collection was deleted)
		coll, err := m.Get(record.Collection)
		if err != nil {
			slog.Debug("Skipping WAL record for missing collection",
				"collection", record.Collection,
				"type", record.Type)
			return nil
		}

		switch record.Type {
		case wal.RecordTypeInsert:
			p, err := deserializePoint(record.Data)
			if err != nil {
				slog.Warn("Failed to deserialize point from WAL",
					"collection", record.Collection,
					"error", err)
				return nil // Continue with other records
			}
			// Use internal method to skip WAL write during replay
			if err := coll.insertInternal(p); err != nil {
				// Point might already exist from index metadata, that's OK
				if err.Error() == "point with this ID already exists" {
					return nil
				}
				slog.Warn("Failed to replay insert",
					"collection", record.Collection,
					"pointID", p.ID,
					"error", err)
			}

		case wal.RecordTypeUpdate:
			p, err := deserializePoint(record.Data)
			if err != nil {
				slog.Warn("Failed to deserialize point from WAL",
					"collection", record.Collection,
					"error", err)
				return nil
			}
			// Use internal method to skip WAL write during replay
			if err := coll.upsertInternal(p); err != nil {
				slog.Warn("Failed to replay upsert",
					"collection", record.Collection,
					"pointID", p.ID,
					"error", err)
			}

		case wal.RecordTypeDelete:
			id := string(record.Data)
			// Use internal method to skip WAL write during replay
			// Ignore errors - point might already be deleted from metadata or not exist
			_ = coll.deleteInternal(id)

		case wal.RecordTypeCheckpoint:
			// Checkpoint records are informational, no action needed
			slog.Debug("Encountered checkpoint record", "seqNum", record.SeqNum)

		default:
			slog.Warn("Unknown WAL record type", "type", record.Type)
		}

		replayedCount++
		return nil
	})

	return replayedCount, err
}

// RecoverWithVectors performs recovery and ensures vectors are restored
// This is used when vectors are stored on disk (mmap) and need to be associated with nodes
func (m *Manager) RecoverWithVectors() error {
	// Standard recovery first
	if err := m.Recover(); err != nil {
		return err
	}

	// Vectors stored in mmap are automatically available through the storage layer
	// No additional work needed here as the mmap storage handles persistence

	return nil
}
