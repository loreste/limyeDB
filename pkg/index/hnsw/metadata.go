package hnsw

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// IndexMetadata contains all the persistent state needed to restore an HNSW index
type IndexMetadata struct {
	Version      int               `json:"version"`
	EntryPoint   uint32            `json:"entry_point"`
	MaxLevel     int               `json:"max_level"`
	NodeCount    int64             `json:"node_count"`
	DeletedCount int64             `json:"deleted_count"`
	M            int               `json:"m"`
	Mmax         int               `json:"mmax"`
	EfSearch     int               `json:"ef_search"`
	Dimension    int               `json:"dimension"`
	IDToIndex    map[string]uint32 `json:"id_to_index"`
	Nodes        []NodeMetadata    `json:"nodes"`
}

// NodeMetadata contains the persistent state for a single node
type NodeMetadata struct {
	ID          string     `json:"id"`
	Level       int        `json:"level"`
	Deleted     bool       `json:"deleted"`
	Connections [][]uint32 `json:"connections"`
}

const metadataVersion = 1

// SaveMetadata persists the HNSW index metadata to disk
func (h *HNSW) SaveMetadata(path string) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Build metadata structure
	meta := &IndexMetadata{
		Version:      metadataVersion,
		EntryPoint:   h.entryPoint,
		MaxLevel:     h.maxLevel,
		NodeCount:    h.nodeCount.Load(),
		DeletedCount: h.deletedCount.Load(),
		M:            h.M,
		Mmax:         h.Mmax,
		EfSearch:     h.efSearch,
		Dimension:    h.dimension,
		IDToIndex:    make(map[string]uint32, len(h.idToIndex)),
		Nodes:        make([]NodeMetadata, len(h.nodes)),
	}

	// Copy ID to index mapping
	for id, idx := range h.idToIndex {
		meta.IDToIndex[id] = idx
	}

	// Copy node metadata
	for i, node := range h.nodes {
		if node == nil {
			continue
		}

		nodeMeta := NodeMetadata{
			ID:      node.ID,
			Level:   node.Level,
			Deleted: node.IsDeleted(),
		}

		// Copy connections if not using mmap
		if h.graphMmap == nil {
			nodeMeta.Connections = make([][]uint32, len(node.Connections))
			for layer := range node.Connections {
				nodeMeta.Connections[layer] = make([]uint32, len(node.Connections[layer]))
				copy(nodeMeta.Connections[layer], node.Connections[layer])
			}
		} else {
			// For mmap, we need to get connections from mmap
			nodeMeta.Connections = make([][]uint32, node.Level+1)
			for layer := 0; layer <= node.Level; layer++ {
				conns := h.graphMmap.GetConnections(uint32(i), layer)
				nodeMeta.Connections[layer] = make([]uint32, len(conns))
				copy(nodeMeta.Connections[layer], conns)
			}
		}

		meta.Nodes[i] = nodeMeta
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed to create metadata directory: %w", err)
	}

	// Marshal to JSON
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Write atomically using temp file
	// #nosec G304 - path is constructed from validated collection names in internal code
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	// Rename for atomic write
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to rename metadata file: %w", err)
	}

	return nil
}

// LoadMetadata restores the HNSW index metadata from disk
func (h *HNSW) LoadMetadata(path string) error {
	// Read metadata file
	// #nosec G304 - path is constructed from validated collection names in internal code
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No metadata file, start fresh
		}
		return fmt.Errorf("failed to read metadata: %w", err)
	}

	var meta IndexMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	// Validate version
	if meta.Version != metadataVersion {
		return fmt.Errorf("unsupported metadata version: %d", meta.Version)
	}

	// Validate dimension matches
	if meta.Dimension != h.dimension {
		return fmt.Errorf("dimension mismatch: metadata has %d, index configured for %d", meta.Dimension, h.dimension)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Restore index state
	h.entryPoint = meta.EntryPoint
	h.maxLevel = meta.MaxLevel
	h.nodeCount.Store(meta.NodeCount)
	h.deletedCount.Store(meta.DeletedCount)

	// Restore ID to index mapping
	h.idToIndex = make(map[string]uint32, len(meta.IDToIndex))
	for id, idx := range meta.IDToIndex {
		h.idToIndex[id] = idx
	}

	// Pre-allocate nodes slice
	h.nodes = make([]*Node, len(meta.Nodes))

	// Restore nodes (without vectors - those come from WAL replay)
	for i, nodeMeta := range meta.Nodes {
		if nodeMeta.ID == "" {
			continue
		}

		useMmap := h.graphMmap != nil
		node := NewNode(nodeMeta.ID, nil, nodeMeta.Level, h.M, useMmap)

		if nodeMeta.Deleted {
			node.MarkDeleted()
		}

		// Restore connections
		if !useMmap && len(nodeMeta.Connections) > 0 {
			node.Connections = make([][]uint32, len(nodeMeta.Connections))
			for layer := range nodeMeta.Connections {
				node.Connections[layer] = make([]uint32, len(nodeMeta.Connections[layer]))
				copy(node.Connections[layer], nodeMeta.Connections[layer])
			}
		} else if useMmap {
			// Add node to mmap
			if err := h.graphMmap.AddNode(uint32(i), nodeMeta.Level); err != nil {
				return fmt.Errorf("failed to add node to graph mmap: %w", err)
			}
			// Restore connections to mmap
			for layer := range nodeMeta.Connections {
				if err := h.graphMmap.SetConnections(uint32(i), layer, nodeMeta.Connections[layer]); err != nil {
					return fmt.Errorf("failed to restore connections: %w", err)
				}
			}
		}

		h.nodes[i] = node
	}

	return nil
}

// HasMetadata checks if metadata file exists
func HasMetadata(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
