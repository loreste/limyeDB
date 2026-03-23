package collection

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/limyedb/limyedb/pkg/index/payload"
	"github.com/limyedb/limyedb/pkg/point"
)

// PayloadIndexConfig holds configuration for a payload field index
type PayloadIndexConfig struct {
	FieldName  string            `json:"field_name"`
	FieldType  PayloadFieldType  `json:"field_type"`
	IndexType  PayloadIndexType  `json:"index_type"`
	Options    map[string]interface{} `json:"options,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
}

// PayloadFieldType represents the data type of a payload field
type PayloadFieldType string

const (
	FieldTypeKeyword  PayloadFieldType = "keyword"   // Exact match strings
	FieldTypeText     PayloadFieldType = "text"      // Full-text search
	FieldTypeInteger  PayloadFieldType = "integer"   // Integer values
	FieldTypeFloat    PayloadFieldType = "float"     // Floating point values
	FieldTypeBool     PayloadFieldType = "bool"      // Boolean values
	FieldTypeGeo      PayloadFieldType = "geo"       // Geo coordinates
	FieldTypeDatetime PayloadFieldType = "datetime"  // Date/time values
)

// PayloadIndexType represents the type of index to create
type PayloadIndexType string

const (
	IndexTypeHash      PayloadIndexType = "hash"      // Hash index for exact match
	IndexTypeNumeric   PayloadIndexType = "numeric"   // B-tree for range queries
	IndexTypeFullText  PayloadIndexType = "fulltext"  // Inverted index for text search
	IndexTypeGeoPoint  PayloadIndexType = "geo"       // Spatial index
)

// PayloadIndexInfo provides information about an existing index
type PayloadIndexInfo struct {
	Config      *PayloadIndexConfig `json:"config"`
	PointsIndexed int64            `json:"points_indexed"`
	SizeBytes   int64              `json:"size_bytes"`
	Status      string             `json:"status"` // "building", "ready", "error"
}

// PayloadSchema describes the full payload schema for a collection
type PayloadSchema struct {
	Fields map[string]*PayloadIndexConfig `json:"fields"`
}

// CreatePayloadIndex creates a new payload index on a field
func (c *Collection) CreatePayloadIndex(cfg *PayloadIndexConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cfg.FieldName == "" {
		return CollectionError("field name required")
	}

	// Validate field type and index type compatibility
	if err := validateIndexConfig(cfg); err != nil {
		return err
	}

	// Create the index in the payload index
	var indexType payload.IndexType
	switch cfg.IndexType {
	case IndexTypeHash:
		indexType = payload.IndexTypeHash
	case IndexTypeNumeric:
		indexType = payload.IndexTypeNumeric
	case IndexTypeFullText:
		indexType = payload.IndexTypeFullText
	case IndexTypeGeoPoint:
		indexType = payload.IndexTypeGeo
	default:
		indexType = payload.IndexTypeHash
	}

	c.payloadIndex.CreateIndex(cfg.FieldName, indexType)

	cfg.CreatedAt = time.Now()

	// Re-index existing points
	if err := c.reindexPayloadField(cfg.FieldName); err != nil {
		return fmt.Errorf("failed to reindex field: %w", err)
	}

	c.updatedAt.Store(time.Now())
	return nil
}

// DeletePayloadIndex removes a payload index
func (c *Collection) DeletePayloadIndex(fieldName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.payloadIndex.DeleteIndex(fieldName)
	c.updatedAt.Store(time.Now())
	return nil
}

// GetPayloadIndexes returns information about all payload indexes
func (c *Collection) GetPayloadIndexes() map[string]*PayloadIndexInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]*PayloadIndexInfo)

	for _, fieldName := range c.payloadIndex.IndexedFields() {
		info := &PayloadIndexInfo{
			Config: &PayloadIndexConfig{
				FieldName: fieldName,
				FieldType: FieldTypeKeyword, // Default, would need to track actual type
				IndexType: IndexTypeHash,
			},
			Status: "ready",
		}
		// Get stats if available
		if stats := c.payloadIndex.GetIndexStats(fieldName); stats != nil {
			info.PointsIndexed = stats.PointCount
			info.SizeBytes = stats.SizeBytes
		}
		result[fieldName] = info
	}

	return result
}

// SetPayloadSchema sets the full payload schema for the collection
func (c *Collection) SetPayloadSchema(schema *PayloadSchema) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for fieldName, cfg := range schema.Fields {
		cfg.FieldName = fieldName

		var indexType payload.IndexType
		switch cfg.IndexType {
		case IndexTypeHash:
			indexType = payload.IndexTypeHash
		case IndexTypeNumeric:
			indexType = payload.IndexTypeNumeric
		case IndexTypeFullText:
			indexType = payload.IndexTypeFullText
		case IndexTypeGeoPoint:
			indexType = payload.IndexTypeGeo
		default:
			indexType = payload.IndexTypeHash
		}

		c.payloadIndex.CreateIndex(fieldName, indexType)
	}

	// Re-index all points
	var idx interface{ GetAllPoints() []*point.Point }
	if c.config.HasNamedVectors() {
		for _, i := range c.indices {
			idx = i
			break
		}
	} else {
		idx = c.index
	}

	if idx != nil {
		for _, p := range idx.GetAllPoints() {
			nodeID, ok := c.getNodeID(p.ID)
			if ok {
				c.payloadIndex.IndexPoint(nodeID, p.Payload)
			}
		}
	}

	c.updatedAt.Store(time.Now())
	return nil
}

// GetPayloadSchema returns the current payload schema
func (c *Collection) GetPayloadSchema() *PayloadSchema {
	indexes := c.GetPayloadIndexes()

	schema := &PayloadSchema{
		Fields: make(map[string]*PayloadIndexConfig),
	}

	for fieldName, info := range indexes {
		schema.Fields[fieldName] = info.Config
	}

	return schema
}

// reindexPayloadField re-indexes all points for a specific field
func (c *Collection) reindexPayloadField(fieldName string) error {
	var idx interface{ GetAllPoints() []*point.Point }
	if c.config.HasNamedVectors() {
		for _, i := range c.indices {
			idx = i
			break
		}
	} else {
		idx = c.index
	}

	if idx == nil {
		return nil
	}

	for _, p := range idx.GetAllPoints() {
		nodeID, ok := c.getNodeID(p.ID)
		if ok && p.Payload != nil {
			if val, exists := p.Payload[fieldName]; exists {
				c.payloadIndex.IndexField(nodeID, fieldName, val)
			}
		}
	}

	return nil
}

// validateIndexConfig validates the index configuration
func validateIndexConfig(cfg *PayloadIndexConfig) error {
	// Validate field type and index type compatibility
	switch cfg.FieldType {
	case FieldTypeKeyword:
		if cfg.IndexType != IndexTypeHash && cfg.IndexType != "" {
			return CollectionError("keyword fields only support hash index")
		}
		if cfg.IndexType == "" {
			cfg.IndexType = IndexTypeHash
		}
	case FieldTypeText:
		if cfg.IndexType != IndexTypeFullText && cfg.IndexType != "" {
			return CollectionError("text fields only support fulltext index")
		}
		if cfg.IndexType == "" {
			cfg.IndexType = IndexTypeFullText
		}
	case FieldTypeInteger, FieldTypeFloat:
		if cfg.IndexType != IndexTypeNumeric && cfg.IndexType != "" {
			return CollectionError("numeric fields only support numeric index")
		}
		if cfg.IndexType == "" {
			cfg.IndexType = IndexTypeNumeric
		}
	case FieldTypeGeo:
		if cfg.IndexType != IndexTypeGeoPoint && cfg.IndexType != "" {
			return CollectionError("geo fields only support geo index")
		}
		if cfg.IndexType == "" {
			cfg.IndexType = IndexTypeGeoPoint
		}
	case FieldTypeBool:
		if cfg.IndexType == "" {
			cfg.IndexType = IndexTypeHash
		}
	case FieldTypeDatetime:
		if cfg.IndexType == "" {
			cfg.IndexType = IndexTypeNumeric
		}
	}

	return nil
}

// PayloadIndexManager manages payload indexes across collections
type PayloadIndexManager struct {
	dataDir string
}

// NewPayloadIndexManager creates a new payload index manager
func NewPayloadIndexManager(dataDir string) *PayloadIndexManager {
	return &PayloadIndexManager{dataDir: dataDir}
}

// SaveConfig saves payload index configuration to disk
func (m *PayloadIndexManager) SaveConfig(collectionName string, schema *PayloadSchema) error {
	if err := os.MkdirAll(m.dataDir, 0750); err != nil {
		return err
	}

	path := filepath.Join(m.dataDir, collectionName+"_payload_indexes.json")
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// LoadConfig loads payload index configuration from disk
func (m *PayloadIndexManager) LoadConfig(collectionName string) (*PayloadSchema, error) {
	path := filepath.Join(m.dataDir, collectionName+"_payload_indexes.json")

	// #nosec G304 - path is constructed internally
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &PayloadSchema{Fields: make(map[string]*PayloadIndexConfig)}, nil
		}
		return nil, err
	}

	var schema PayloadSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, err
	}

	return &schema, nil
}
