package point

import (
	"encoding/json"
	"errors"
)

// MultiVector represents a point with multiple named vectors
type MultiVector struct {
	ID      string                 `json:"id"`
	Vectors map[string]Vector      `json:"vectors"` // Named vectors
	Payload map[string]interface{} `json:"payload,omitempty"`
}

// NewMultiVector creates a new multi-vector point
func NewMultiVector(id string) *MultiVector {
	return &MultiVector{
		ID:      id,
		Vectors: make(map[string]Vector),
		Payload: make(map[string]interface{}),
	}
}

// NewMultiVectorWithVectors creates a multi-vector point with initial vectors
func NewMultiVectorWithVectors(id string, vectors map[string]Vector, payload map[string]interface{}) *MultiVector {
	mv := &MultiVector{
		ID:      id,
		Vectors: vectors,
	}
	if payload != nil {
		mv.Payload = payload
	} else {
		mv.Payload = make(map[string]interface{})
	}
	return mv
}

// SetVector sets a named vector
func (mv *MultiVector) SetVector(name string, vector Vector) {
	mv.Vectors[name] = vector
}

// GetVector gets a named vector
func (mv *MultiVector) GetVector(name string) (Vector, bool) {
	v, ok := mv.Vectors[name]
	return v, ok
}

// HasVector checks if a named vector exists
func (mv *MultiVector) HasVector(name string) bool {
	_, ok := mv.Vectors[name]
	return ok
}

// VectorNames returns all vector names
func (mv *MultiVector) VectorNames() []string {
	names := make([]string, 0, len(mv.Vectors))
	for name := range mv.Vectors {
		names = append(names, name)
	}
	return names
}

// ToPoint converts to a single-vector Point using the default vector
func (mv *MultiVector) ToPoint(defaultVectorName string) (*Point, error) {
	vec, ok := mv.Vectors[defaultVectorName]
	if !ok {
		return nil, errors.New("default vector not found")
	}
	return &Point{
		ID:      mv.ID,
		Vector:  vec,
		Payload: mv.Payload,
	}, nil
}

// FromPoint creates a MultiVector from a Point with a single named vector
func FromPoint(p *Point, vectorName string) *MultiVector {
	return &MultiVector{
		ID:      p.ID,
		Vectors: map[string]Vector{vectorName: p.Vector},
		Payload: p.Payload,
	}
}

// Validate validates the multi-vector point
func (mv *MultiVector) Validate() error {
	if mv.ID == "" {
		return errors.New("point ID is required")
	}
	if len(mv.Vectors) == 0 {
		return errors.New("at least one vector is required")
	}
	for name, vec := range mv.Vectors {
		if len(vec) == 0 {
			return errors.New("vector '" + name + "' is empty")
		}
	}
	return nil
}

// Encode serializes the multi-vector point
func (mv *MultiVector) Encode() ([]byte, error) {
	return json.Marshal(mv)
}

// DecodeMultiVector deserializes a multi-vector point
func DecodeMultiVector(data []byte) (*MultiVector, error) {
	var mv MultiVector
	if err := json.Unmarshal(data, &mv); err != nil {
		return nil, err
	}
	return &mv, nil
}

// =============================================================================
// Multi-Vector Configuration
// =============================================================================

// VectorConfig holds configuration for a named vector
type VectorConfig struct {
	Name      string `json:"name"`
	Dimension int    `json:"dimension"`
	Metric    string `json:"metric"`
	OnDisk    bool   `json:"on_disk"`

	// Quantization settings per vector
	Quantization string `json:"quantization,omitempty"` // "none", "scalar", "binary", "pq"
}

// MultiVectorConfig holds configuration for multiple vectors
type MultiVectorConfig struct {
	Vectors       []VectorConfig `json:"vectors"`
	DefaultVector string         `json:"default_vector"` // Default vector for single-vector queries
}

// NewMultiVectorConfig creates a default multi-vector config
func NewMultiVectorConfig() *MultiVectorConfig {
	return &MultiVectorConfig{
		Vectors:       make([]VectorConfig, 0),
		DefaultVector: "default",
	}
}

// AddVector adds a vector configuration
func (c *MultiVectorConfig) AddVector(cfg VectorConfig) {
	c.Vectors = append(c.Vectors, cfg)
}

// GetVectorConfig gets configuration for a named vector
func (c *MultiVectorConfig) GetVectorConfig(name string) (*VectorConfig, bool) {
	for i := range c.Vectors {
		if c.Vectors[i].Name == name {
			return &c.Vectors[i], true
		}
	}
	return nil, false
}

// Validate validates the configuration
func (c *MultiVectorConfig) Validate() error {
	if len(c.Vectors) == 0 {
		return errors.New("at least one vector configuration is required")
	}

	names := make(map[string]bool)
	for _, v := range c.Vectors {
		if v.Name == "" {
			return errors.New("vector name is required")
		}
		if v.Dimension <= 0 {
			return errors.New("vector dimension must be positive")
		}
		if names[v.Name] {
			return errors.New("duplicate vector name: " + v.Name)
		}
		names[v.Name] = true
	}

	if c.DefaultVector != "" && !names[c.DefaultVector] {
		return errors.New("default vector not found in configuration")
	}

	return nil
}

// =============================================================================
// ColBERT-style Multi-Vector Support
// =============================================================================

// ColBERTVector represents a ColBERT-style document with multiple token vectors
type ColBERTVector struct {
	ID           string                 `json:"id"`
	TokenVectors []Vector               `json:"token_vectors"` // One vector per token
	Payload      map[string]interface{} `json:"payload,omitempty"`
}

// NewColBERTVector creates a new ColBERT vector
func NewColBERTVector(id string, tokenVectors []Vector) *ColBERTVector {
	return &ColBERTVector{
		ID:           id,
		TokenVectors: tokenVectors,
		Payload:      make(map[string]interface{}),
	}
}

// NumTokens returns the number of token vectors
func (cv *ColBERTVector) NumTokens() int {
	return len(cv.TokenVectors)
}

// Dimension returns the dimension of token vectors
func (cv *ColBERTVector) Dimension() int {
	if len(cv.TokenVectors) == 0 {
		return 0
	}
	return len(cv.TokenVectors[0])
}

// MaxSim computes the ColBERT MaxSim score with a query
// For each query token, find max similarity with any doc token, then sum
func (cv *ColBERTVector) MaxSim(queryTokens []Vector) float32 {
	if len(queryTokens) == 0 || len(cv.TokenVectors) == 0 {
		return 0
	}

	var totalScore float32

	for _, queryToken := range queryTokens {
		var maxSim float32 = -1e10

		for _, docToken := range cv.TokenVectors {
			sim := dotProduct(queryToken, docToken)
			if sim > maxSim {
				maxSim = sim
			}
		}

		totalScore += maxSim
	}

	return totalScore
}

// dotProduct computes dot product between two vectors
func dotProduct(a, b Vector) float32 {
	var sum float32
	for i := range a {
		if i < len(b) {
			sum += a[i] * b[i]
		}
	}
	return sum
}

// =============================================================================
// Matryoshka Embeddings Support
// =============================================================================

// MatryoshkaVector supports variable-length embeddings (Matryoshka representation)
// The full vector can be truncated to smaller dimensions while maintaining quality
type MatryoshkaVector struct {
	ID         string                 `json:"id"`
	FullVector Vector                 `json:"full_vector"`
	Dimensions []int                  `json:"dimensions"` // Supported truncation dimensions
	Payload    map[string]interface{} `json:"payload,omitempty"`
}

// NewMatryoshkaVector creates a new Matryoshka vector
func NewMatryoshkaVector(id string, vector Vector, dimensions []int) *MatryoshkaVector {
	return &MatryoshkaVector{
		ID:         id,
		FullVector: vector,
		Dimensions: dimensions,
		Payload:    make(map[string]interface{}),
	}
}

// GetTruncated returns the vector truncated to specified dimension
func (mv *MatryoshkaVector) GetTruncated(dim int) Vector {
	if dim >= len(mv.FullVector) {
		return mv.FullVector
	}
	return mv.FullVector[:dim]
}

// SupportsD dimension checks if this dimension is supported
func (mv *MatryoshkaVector) SupportsDimension(dim int) bool {
	for _, d := range mv.Dimensions {
		if d == dim {
			return true
		}
	}
	return false
}
