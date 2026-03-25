package collection

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"context"

	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/point"
	"github.com/limyedb/limyedb/pkg/storage/s3"
	"github.com/limyedb/limyedb/pkg/storage/snapshot"
)

// validNamePattern defines allowed characters for collection names
// Only alphanumeric, underscore, and hyphen are allowed
var validNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// validateName checks if a collection name is safe (no path traversal)
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("collection name cannot be empty")
	}
	if len(name) > 255 {
		return fmt.Errorf("collection name too long (max 255 characters)")
	}
	if !validNamePattern.MatchString(name) {
		return fmt.Errorf("collection name contains invalid characters (only alphanumeric, underscore, hyphen allowed, must start with alphanumeric)")
	}
	// Extra safety: check for path traversal attempts
	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("collection name contains invalid path characters")
	}
	return nil
}

// Manager manages collections
type Manager struct {
	collections map[string]*Collection
	dataDir     string
	maxCollections int

	mu sync.RWMutex
}

// ManagerConfig holds manager configuration
type ManagerConfig struct {
	DataDir        string
	MaxCollections int
}

// DefaultManagerConfig returns default manager configuration
func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		DataDir:        "./data/collections",
		MaxCollections: 1000,
	}
}

// NewManager creates a new collection manager
func NewManager(cfg *ManagerConfig) (*Manager, error) {
	if err := os.MkdirAll(cfg.DataDir, 0750); err != nil {
		return nil, err
	}

	m := &Manager{
		collections:    make(map[string]*Collection),
		dataDir:        cfg.DataDir,
		maxCollections: cfg.MaxCollections,
	}

	// Load existing collections
	if err := m.loadCollections(); err != nil {
		return nil, err
	}

	return m, nil
}

// loadCollections loads collection metadata from disk
func (m *Manager) loadCollections() error {
	entries, err := os.ReadDir(m.dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		metaPath := filepath.Join(m.dataDir, entry.Name(), "meta.json")
		// #nosec G304 - metaPath is constructed from internal dataDir, not user input
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}

		var cfg config.CollectionConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			continue
		}

		// Create collection (lazy load data)
		coll, err := New(&cfg)
		if err != nil {
			continue
		}

		m.collections[cfg.Name] = coll
	}

	return nil
}

// Create creates a new collection
func (m *Manager) Create(cfg *config.CollectionConfig) (*Collection, error) {
	// Validate collection name to prevent path traversal
	if err := validateName(cfg.Name); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if exists
	if _, exists := m.collections[cfg.Name]; exists {
		return nil, ErrCollectionExists
	}

	// Check limit
	if len(m.collections) >= m.maxCollections {
		return nil, ErrMaxCollections
	}

	// Apply defaults
	if cfg.HNSW.M == 0 {
		cfg.HNSW.M = 16
	}
	if cfg.HNSW.EfConstruction == 0 {
		cfg.HNSW.EfConstruction = 200
	}
	if cfg.HNSW.EfSearch == 0 {
		cfg.HNSW.EfSearch = 100
	}
	if cfg.HNSW.MaxElements == 0 {
		cfg.HNSW.MaxElements = 100000
	}

	// Create collection
	coll, err := New(cfg)
	if err != nil {
		return nil, err
	}

	// Create directory with restricted permissions
	collDir := filepath.Join(m.dataDir, cfg.Name)
	if err := os.MkdirAll(collDir, 0750); err != nil {
		return nil, err
	}

	// Save metadata
	if err := m.saveMetadata(cfg); err != nil {
		_ = os.RemoveAll(collDir) // Best effort cleanup
		return nil, err
	}

	m.collections[cfg.Name] = coll
	return coll, nil
}

// Get retrieves a collection by name
func (m *Manager) Get(name string) (*Collection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	coll, exists := m.collections[name]
	if !exists {
		return nil, ErrCollectionNotFound
	}

	return coll, nil
}

// List returns all collection names
func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.collections))
	for name := range m.collections {
		names = append(names, name)
	}
	return names
}

// ListInfo returns info for all collections
func (m *Manager) ListInfo() []*Info {
	m.mu.RLock()
	defer m.mu.RUnlock()

	infos := make([]*Info, 0, len(m.collections))
	for _, coll := range m.collections {
		infos = append(infos, coll.Info())
	}
	return infos
}

// Delete removes a collection
func (m *Manager) Delete(name string) error {
	// Validate name to prevent path traversal attacks
	if err := validateName(name); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.collections[name]; !exists {
		return ErrCollectionNotFound
	}

	// Remove from map
	delete(m.collections, name)

	// Remove data directory - safe because name is validated
	collDir := filepath.Join(m.dataDir, name)
	// Double-check the path is within dataDir
	absCollDir, err := filepath.Abs(collDir)
	if err != nil {
		return err
	}
	absDataDir, err := filepath.Abs(m.dataDir)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(absCollDir, absDataDir) {
		return fmt.Errorf("invalid collection path")
	}
	return os.RemoveAll(collDir)
}

// Exists checks if a collection exists
func (m *Manager) Exists(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.collections[name]
	return exists
}

// Count returns the number of collections
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.collections)
}

// saveMetadata saves collection metadata to disk
func (m *Manager) saveMetadata(cfg *config.CollectionConfig) error {
	collDir := filepath.Join(m.dataDir, cfg.Name)
	metaPath := filepath.Join(collDir, "meta.json")

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(metaPath, data, 0600)
}

// CreateSnapshot creates a snapshot of all collections
func (m *Manager) CreateSnapshot(snapMgr *snapshot.Manager) (*snapshot.Snapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.collections))
	for name := range m.collections {
		names = append(names, name)
	}

	writer, err := snapMgr.CreateSnapshot(names)
	if err != nil {
		return nil, err
	}

	// Write header
	if err := writer.WriteHeader(1, len(m.collections)); err != nil {
		_ = writer.Cancel() // Best effort cleanup
		return nil, err
	}

	// Write each collection
	for name, coll := range m.collections {
		// Export all points from the collection
		points := make([]snapshot.PointData, 0)
		_ = coll.Iterate(func(p *point.Point) error {
			points = append(points, snapshot.PointData{
				ID:      p.ID,
				Vector:  p.Vector,
				Payload: p.Payload,
			})
			return nil
		})

		data := snapshot.CollectionData{
			Config: coll.Config(),
			Points: points,
		}

		if err := writer.WriteCollection(name, data); err != nil {
			_ = writer.Cancel() // Best effort cleanup
			return nil, err
		}
	}

	return writer.Finish()
}

// RestoreSnapshot restores collections from a snapshot
func (m *Manager) RestoreSnapshot(snapMgr *snapshot.Manager, snapID string) error {
	reader, err := snapMgr.OpenSnapshot(snapID)
	if err != nil {
		return err
	}
	defer reader.Close()

	m.mu.Lock()
	defer m.mu.Unlock()

	for i := uint32(0); i < reader.NumCollections; i++ {
		name, data, err := reader.ReadCollection()
		if err != nil {
			return err
		}

		// Convert config
		cfgBytes, err := json.Marshal(data.Config)
		if err != nil {
			return err
		}
		var cfg config.CollectionConfig
		if err := json.Unmarshal(cfgBytes, &cfg); err != nil {
			return err
		}

		// Create collection
		coll, err := New(&cfg)
		if err != nil {
			return err
		}

		// Insert points
		for _, pd := range data.Points {
			p := point.NewPointWithID(pd.ID, pd.Vector, pd.Payload)
			if err := coll.Insert(p); err != nil {
				// Log error but continue
			}
		}

		m.collections[name] = coll

		// Save metadata
		if err := m.saveMetadata(&cfg); err != nil {
			return err
		}
	}

	return nil
}

// Flush flushes all collections to disk
func (m *Manager) Flush() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var lastErr error
	for _, coll := range m.collections {
		// Save collection metadata
		if err := m.saveMetadata(coll.config); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// Close closes all collections
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var lastErr error

	// Save metadata for all collections before clearing
	for _, coll := range m.collections {
		if err := m.saveMetadata(coll.config); err != nil {
			lastErr = err
		}
	}

	// Clear the collections map
	m.collections = make(map[string]*Collection)

	return lastErr
}

// Rename renames a collection
func (m *Manager) Rename(oldName, newName string) error {
	// Validate new collection name to prevent path traversal
	if err := validateName(newName); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	coll, exists := m.collections[oldName]
	if !exists {
		return ErrCollectionNotFound
	}

	if _, exists := m.collections[newName]; exists {
		return ErrCollectionExists
	}

	// Update config
	coll.config.Name = newName

	// Rename directory
	oldDir := filepath.Join(m.dataDir, oldName)
	newDir := filepath.Join(m.dataDir, newName)
	if err := os.Rename(oldDir, newDir); err != nil {
		return err
	}

	// Update map
	delete(m.collections, oldName)
	m.collections[newName] = coll

	// Update metadata
	return m.saveMetadata(coll.config)
}

// UpdateConfig updates collection configuration
func (m *Manager) UpdateConfig(name string, updates map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	coll, exists := m.collections[name]
	if !exists {
		return ErrCollectionNotFound
	}

	// Apply updates (limited to safe fields)
	if ef, ok := updates["ef_search"].(int); ok {
		coll.SetEfSearch(ef)
		coll.config.HNSW.EfSearch = ef
	}

	return m.saveMetadata(coll.config)
}

// Error constants
const (
	ErrMaxCollections CollectionError = "maximum number of collections reached"
)

// ArchiveCollection safely flushes local NVMe layers unbinding nodes and natively streams directly to S3
func (m *Manager) ArchiveCollection(ctx context.Context, name string, s3Store *s3.Storage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, exists := m.collections[name]
	if !exists {
		return ErrCollectionNotFound
	}

	collDir := filepath.Join(m.dataDir, name)

	err := filepath.Walk(collDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			relPath, _ := filepath.Rel(m.dataDir, path)
			if err := s3Store.UploadFile(ctx, path, fmt.Sprintf("limyedb/archive/%s", relPath)); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("serverless stream bounds failed cleanly handling AWS: %w", err)
	}

	delete(m.collections, name)
	return os.RemoveAll(collDir)
}
