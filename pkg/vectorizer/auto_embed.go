package vectorizer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/limyedb/limyedb/pkg/point"
)

// AutoEmbedConfig configures automatic embedding for a collection
type AutoEmbedConfig struct {
	// Vectorizer name to use
	Vectorizer string `json:"vectorizer"`

	// SourceFields specifies which payload fields to extract text from
	// If empty, uses all string fields
	SourceFields []string `json:"source_fields,omitempty"`

	// TargetVector specifies which named vector to populate
	// If empty, uses the default vector
	TargetVector string `json:"target_vector,omitempty"`

	// Template for combining multiple fields
	// Use {field_name} placeholders, e.g., "{title} - {description}"
	// If empty, fields are concatenated with spaces
	Template string `json:"template,omitempty"`

	// OnConflict specifies behavior when vector already exists
	// Options: "skip", "overwrite"
	OnConflict string `json:"on_conflict,omitempty"`

	// Enabled controls whether auto-embedding is active
	Enabled bool `json:"enabled"`
}

// AutoEmbedder handles automatic text-to-vector conversion
type AutoEmbedder struct {
	manager *VectorizerManager
	configs map[string]*AutoEmbedConfig // collection -> config
	mu      sync.RWMutex
}

// NewAutoEmbedder creates a new auto-embedder
func NewAutoEmbedder(manager *VectorizerManager) *AutoEmbedder {
	return &AutoEmbedder{
		manager: manager,
		configs: make(map[string]*AutoEmbedConfig),
	}
}

// Configure sets up auto-embedding for a collection
func (a *AutoEmbedder) Configure(collectionName string, cfg *AutoEmbedConfig) error {
	if cfg.Enabled {
		// Validate vectorizer exists
		if _, ok := a.manager.Get(cfg.Vectorizer); !ok {
			return fmt.Errorf("vectorizer '%s' not found", cfg.Vectorizer)
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	a.configs[collectionName] = cfg
	return nil
}

// GetConfig returns the auto-embed config for a collection
func (a *AutoEmbedder) GetConfig(collectionName string) (*AutoEmbedConfig, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	cfg, ok := a.configs[collectionName]
	return cfg, ok
}

// RemoveConfig removes auto-embed config for a collection
func (a *AutoEmbedder) RemoveConfig(collectionName string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.configs, collectionName)
}

// ProcessPoint processes a single point, generating embeddings if needed
func (a *AutoEmbedder) ProcessPoint(ctx context.Context, collectionName string, p *point.Point) error {
	cfg, ok := a.GetConfig(collectionName)
	if !ok || !cfg.Enabled {
		return nil
	}

	// Check if vector already exists
	if len(p.Vector) > 0 && cfg.OnConflict == "skip" {
		return nil
	}

	// Extract text from payload
	text := a.extractText(p.Payload, cfg)
	if text == "" {
		return nil
	}

	// Get vectorizer
	vectorizer, ok := a.manager.Get(cfg.Vectorizer)
	if !ok {
		return fmt.Errorf("vectorizer '%s' not found", cfg.Vectorizer)
	}

	// Generate embedding
	vec, err := vectorizer.Vectorize(ctx, text)
	if err != nil {
		return fmt.Errorf("vectorization failed: %w", err)
	}

	p.Vector = vec
	return nil
}

// ProcessPoints processes multiple points in batch
func (a *AutoEmbedder) ProcessPoints(ctx context.Context, collectionName string, points []*point.Point) error {
	cfg, ok := a.GetConfig(collectionName)
	if !ok || !cfg.Enabled {
		return nil
	}

	// Get vectorizer
	vectorizer, ok := a.manager.Get(cfg.Vectorizer)
	if !ok {
		return fmt.Errorf("vectorizer '%s' not found", cfg.Vectorizer)
	}

	// Collect points that need embedding
	var textsToEmbed []string
	var pointIndices []int

	for i, p := range points {
		// Check if vector already exists
		if len(p.Vector) > 0 && cfg.OnConflict == "skip" {
			continue
		}

		text := a.extractText(p.Payload, cfg)
		if text == "" {
			continue
		}

		textsToEmbed = append(textsToEmbed, text)
		pointIndices = append(pointIndices, i)
	}

	if len(textsToEmbed) == 0 {
		return nil
	}

	// Generate embeddings in batch
	vectors, err := vectorizer.VectorizeBatch(ctx, textsToEmbed)
	if err != nil {
		return fmt.Errorf("batch vectorization failed: %w", err)
	}

	// Assign vectors to points
	for i, vec := range vectors {
		points[pointIndices[i]].Vector = vec
	}

	return nil
}

func (a *AutoEmbedder) extractText(payload map[string]interface{}, cfg *AutoEmbedConfig) string {
	if payload == nil {
		return ""
	}

	// Use template if provided
	if cfg.Template != "" {
		return a.applyTemplate(payload, cfg.Template)
	}

	// Extract specific fields or all string fields
	var parts []string

	if len(cfg.SourceFields) > 0 {
		for _, field := range cfg.SourceFields {
			if val, ok := payload[field]; ok {
				if str, ok := val.(string); ok && str != "" {
					parts = append(parts, str)
				}
			}
		}
	} else {
		// Extract all string fields
		for _, val := range payload {
			if str, ok := val.(string); ok && str != "" {
				parts = append(parts, str)
			}
		}
	}

	return strings.Join(parts, " ")
}

func (a *AutoEmbedder) applyTemplate(payload map[string]interface{}, template string) string {
	result := template

	for key, val := range payload {
		placeholder := "{" + key + "}"
		if str, ok := val.(string); ok {
			result = strings.ReplaceAll(result, placeholder, str)
		}
	}

	// Remove any remaining placeholders
	for strings.Contains(result, "{") && strings.Contains(result, "}") {
		start := strings.Index(result, "{")
		end := strings.Index(result, "}")
		if start < end {
			result = result[:start] + result[end+1:]
		} else {
			break
		}
	}

	return strings.TrimSpace(result)
}

// EmbedText generates an embedding for arbitrary text
func (a *AutoEmbedder) EmbedText(ctx context.Context, vectorizerName string, text string) (point.Vector, error) {
	vectorizer, ok := a.manager.Get(vectorizerName)
	if !ok {
		return nil, fmt.Errorf("vectorizer '%s' not found", vectorizerName)
	}

	return vectorizer.Vectorize(ctx, text)
}

// EmbedTexts generates embeddings for multiple texts
func (a *AutoEmbedder) EmbedTexts(ctx context.Context, vectorizerName string, texts []string) ([]point.Vector, error) {
	vectorizer, ok := a.manager.Get(vectorizerName)
	if !ok {
		return nil, fmt.Errorf("vectorizer '%s' not found", vectorizerName)
	}

	return vectorizer.VectorizeBatch(ctx, texts)
}

// SearchByText converts text to vector and returns the vector for searching
func (a *AutoEmbedder) SearchByText(ctx context.Context, collectionName string, text string) (point.Vector, error) {
	cfg, ok := a.GetConfig(collectionName)
	if !ok {
		return nil, errors.New("collection not configured for auto-embedding")
	}

	vectorizer, ok := a.manager.Get(cfg.Vectorizer)
	if !ok {
		return nil, fmt.Errorf("vectorizer '%s' not found", cfg.Vectorizer)
	}

	return vectorizer.Vectorize(ctx, text)
}

// ChunkedAutoEmbedConfig extends AutoEmbedConfig with chunking options
type ChunkedAutoEmbedConfig struct {
	AutoEmbedConfig

	// Chunking options
	ChunkSize    int    `json:"chunk_size,omitempty"`    // Max characters per chunk
	ChunkOverlap int    `json:"chunk_overlap,omitempty"` // Overlap between chunks
	ChunkField   string `json:"chunk_field,omitempty"`   // Field to chunk (default: content)
}

// ChunkText splits text into overlapping chunks
func ChunkText(text string, chunkSize, overlap int) []string {
	if chunkSize <= 0 {
		chunkSize = 500
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= chunkSize {
		overlap = chunkSize / 4
	}

	if len(text) <= chunkSize {
		return []string{text}
	}

	var chunks []string
	stride := chunkSize - overlap

	for i := 0; i < len(text); i += stride {
		end := i + chunkSize
		if end > len(text) {
			end = len(text)
		}

		chunk := text[i:end]
		chunks = append(chunks, strings.TrimSpace(chunk))

		if end == len(text) {
			break
		}
	}

	return chunks
}

// ProcessChunkedPoint creates multiple points from a single document with chunks
func (a *AutoEmbedder) ProcessChunkedPoint(ctx context.Context, collectionName string, p *point.Point, chunkCfg *ChunkedAutoEmbedConfig) ([]*point.Point, error) {
	cfg, ok := a.GetConfig(collectionName)
	if !ok || !cfg.Enabled {
		return []*point.Point{p}, nil
	}

	// Get the field to chunk
	chunkField := chunkCfg.ChunkField
	if chunkField == "" {
		chunkField = "content"
	}

	// Extract text to chunk
	text, ok := p.Payload[chunkField].(string)
	if !ok || text == "" {
		return []*point.Point{p}, nil
	}

	// Split into chunks
	chunks := ChunkText(text, chunkCfg.ChunkSize, chunkCfg.ChunkOverlap)
	if len(chunks) <= 1 {
		// No chunking needed, process normally
		if err := a.ProcessPoint(ctx, collectionName, p); err != nil {
			return nil, err
		}
		return []*point.Point{p}, nil
	}

	// Get vectorizer
	vectorizer, ok := a.manager.Get(cfg.Vectorizer)
	if !ok {
		return nil, fmt.Errorf("vectorizer '%s' not found", cfg.Vectorizer)
	}

	// Generate embeddings for all chunks
	vectors, err := vectorizer.VectorizeBatch(ctx, chunks)
	if err != nil {
		return nil, err
	}

	// Create a point for each chunk
	points := make([]*point.Point, len(chunks))
	for i, chunk := range chunks {
		// Clone the payload
		payload := make(map[string]interface{})
		for k, v := range p.Payload {
			payload[k] = v
		}

		// Update chunk-specific fields
		payload[chunkField] = chunk
		payload["_chunk_index"] = i
		payload["_chunk_total"] = len(chunks)
		payload["_parent_id"] = p.ID

		points[i] = &point.Point{
			ID:      fmt.Sprintf("%s_chunk_%d", p.ID, i),
			Vector:  vectors[i],
			Payload: payload,
		}
	}

	return points, nil
}
