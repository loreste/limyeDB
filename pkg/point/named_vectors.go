package point

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/google/uuid"
)

// NamedVectors represents multiple named vectors for a single point
type NamedVectors map[string]Vector

// PointV2 represents a point with support for multiple named vectors
// This is the next-generation point structure supporting multi-vector storage
type PointV2 struct {
	ID            string                 `json:"id"`
	Vector        Vector                 `json:"vector,omitempty"`         // Default/unnamed vector (backwards compat)
	Vectors       NamedVectors           `json:"vectors,omitempty"`        // Named vectors
	Payload       map[string]interface{} `json:"payload,omitempty"`
	Sparse        *SparseVector          `json:"sparse,omitempty"`         // Hybrid search mapping
	MultiVectors  map[string][][]float32 `json:"multi_vectors,omitempty"`  // ColBERT matrices
	Version       uint64                 `json:"-"`
}

// NewPointV2 creates a new point with named vectors
func NewPointV2(vectors NamedVectors, payload map[string]interface{}) *PointV2 {
	return &PointV2{
		ID:      uuid.New().String(),
		Vectors: vectors,
		Payload: payload,
		Version: 1,
	}
}

// NewPointV2WithID creates a point with a specific ID and named vectors
func NewPointV2WithID(id string, vectors NamedVectors, payload map[string]interface{}) *PointV2 {
	return &PointV2{
		ID:      id,
		Vectors: vectors,
		Payload: payload,
		Version: 1,
	}
}

// NewPointV2FromSingle creates a PointV2 from a single vector (backwards compatibility)
func NewPointV2FromSingle(id string, vector Vector, payload map[string]interface{}) *PointV2 {
	return &PointV2{
		ID:      id,
		Vector:  vector,
		Payload: payload,
		Version: 1,
	}
}

// GetVector returns a vector by name, or the default vector if name is empty
func (p *PointV2) GetVector(name string) (Vector, bool) {
	if name == "" || name == "default" {
		if len(p.Vector) > 0 {
			return p.Vector, true
		}
		// Try to get from named vectors with "default" key
		if v, ok := p.Vectors["default"]; ok {
			return v, true
		}
		// Return first vector if only one exists
		if len(p.Vectors) == 1 {
			for _, v := range p.Vectors {
				return v, true
			}
		}
		return nil, false
	}
	v, ok := p.Vectors[name]
	return v, ok
}

// SetVector sets a vector by name
func (p *PointV2) SetVector(name string, vector Vector) {
	if name == "" || name == "default" {
		p.Vector = vector
		return
	}
	if p.Vectors == nil {
		p.Vectors = make(NamedVectors)
	}
	p.Vectors[name] = vector
}

// VectorNames returns all vector names in this point
func (p *PointV2) VectorNames() []string {
	names := make([]string, 0, len(p.Vectors)+1)
	if len(p.Vector) > 0 {
		names = append(names, "default")
	}
	for name := range p.Vectors {
		names = append(names, name)
	}
	return names
}

// HasVector checks if a vector with the given name exists
func (p *PointV2) HasVector(name string) bool {
	_, ok := p.GetVector(name)
	return ok
}

// Dimension returns the dimension of the specified vector
func (p *PointV2) Dimension(vectorName string) int {
	v, ok := p.GetVector(vectorName)
	if !ok {
		return 0
	}
	return len(v)
}

// Clone creates a deep copy of the point
func (p *PointV2) Clone() *PointV2 {
	clone := &PointV2{
		ID:      p.ID,
		Version: p.Version,
	}

	if len(p.Vector) > 0 {
		clone.Vector = make(Vector, len(p.Vector))
		copy(clone.Vector, p.Vector)
	}

	if len(p.Vectors) > 0 {
		clone.Vectors = make(NamedVectors)
		for name, vec := range p.Vectors {
			cloneVec := make(Vector, len(vec))
			copy(cloneVec, vec)
			clone.Vectors[name] = cloneVec
		}
	}

	if p.Payload != nil {
		clone.Payload = make(map[string]interface{})
		for k, v := range p.Payload {
			clone.Payload[k] = v
		}
	}

	if p.Sparse != nil {
		clone.Sparse = &SparseVector{
			Indices: make([]uint32, len(p.Sparse.Indices)),
			Values:  make([]float32, len(p.Sparse.Values)),
		}
		copy(clone.Sparse.Indices, p.Sparse.Indices)
		copy(clone.Sparse.Values, p.Sparse.Values)
	}

	return clone
}

// Validate checks if the point is valid
func (p *PointV2) Validate() error {
	if p.ID == "" {
		return ErrEmptyID
	}

	// Must have at least one vector
	if len(p.Vector) == 0 && len(p.Vectors) == 0 && p.Sparse == nil {
		return ErrEmptyVector
	}

	// Validate default vector
	if len(p.Vector) > 0 {
		if err := validateVector(p.Vector); err != nil {
			return fmt.Errorf("default vector: %w", err)
		}
	}

	// Validate named vectors
	for name, vec := range p.Vectors {
		if err := validateVector(vec); err != nil {
			return fmt.Errorf("vector %q: %w", name, err)
		}
	}

	return nil
}

func validateVector(v Vector) error {
	if len(v) == 0 {
		return ErrEmptyVector
	}
	for i, val := range v {
		if math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) {
			return &InvalidVectorError{Index: i, Value: val}
		}
	}
	return nil
}

// Normalize normalizes the specified vector to unit length
func (p *PointV2) Normalize(vectorName string) {
	if vectorName == "" || vectorName == "default" {
		p.Vector.Normalize()
		return
	}
	if vec, ok := p.Vectors[vectorName]; ok {
		vec.Normalize()
		p.Vectors[vectorName] = vec
	}
}

// NormalizeAll normalizes all vectors to unit length
func (p *PointV2) NormalizeAll() {
	if len(p.Vector) > 0 {
		p.Vector.Normalize()
	}
	for name, vec := range p.Vectors {
		vec.Normalize()
		p.Vectors[name] = vec
	}
}

// ToPoint converts PointV2 to legacy Point (uses default vector)
func (p *PointV2) ToPoint() *Point {
	vec, _ := p.GetVector("")
	return &Point{
		ID:      p.ID,
		Vector:  vec,
		Payload: p.Payload,
		Sparse:  p.Sparse,
		Version: p.Version,
	}
}

// PointToV2 converts a legacy Point to PointV2
func PointToV2(p *Point) *PointV2 {
	return &PointV2{
		ID:      p.ID,
		Vector:  p.Vector,
		Payload: p.Payload,
		Sparse:  p.Sparse,
		Version: p.Version,
	}
}

// Encode serializes the point to binary format
func (p *PointV2) Encode(w io.Writer) error {
	// Write format version (2 for PointV2)
	if err := binary.Write(w, binary.LittleEndian, uint8(2)); err != nil {
		return err
	}

	// Write ID
	idBytes := []byte(p.ID)
	idLen, err := safeIntToUint16(len(idBytes))
	if err != nil {
		return errors.New("point ID too long")
	}
	if err := binary.Write(w, binary.LittleEndian, idLen); err != nil {
		return err
	}
	if _, err := w.Write(idBytes); err != nil {
		return err
	}

	// Write default vector
	if err := writeVector(w, p.Vector); err != nil {
		return err
	}

	// Write named vectors count and data
	numVectors, err := safeIntToUint16(len(p.Vectors))
	if err != nil {
		return errors.New("too many named vectors")
	}
	if err := binary.Write(w, binary.LittleEndian, numVectors); err != nil {
		return err
	}
	for name, vec := range p.Vectors {
		// Write name
		nameBytes := []byte(name)
		if len(nameBytes) > math.MaxUint8 {
			return errors.New("vector name too long")
		}
		nameBytesLen := uint8(len(nameBytes)) // safe: bounds checked above
		if err := binary.Write(w, binary.LittleEndian, nameBytesLen); err != nil {
			return err
		}
		if _, err := w.Write(nameBytes); err != nil {
			return err
		}
		// Write vector
		if err := writeVector(w, vec); err != nil {
			return err
		}
	}

	// Write payload as JSON
	payloadBytes, err := json.Marshal(p.Payload)
	if err != nil {
		return err
	}
	payloadLen, err := safeIntToUint32(len(payloadBytes))
	if err != nil {
		return errors.New("payload too large")
	}
	if err := binary.Write(w, binary.LittleEndian, payloadLen); err != nil {
		return err
	}
	if _, err := w.Write(payloadBytes); err != nil {
		return err
	}

	// Write version
	return binary.Write(w, binary.LittleEndian, p.Version)
}

func writeVector(w io.Writer, v Vector) error {
	vLen, err := safeIntToUint32(len(v))
	if err != nil {
		return errors.New("vector too large")
	}
	if err := binary.Write(w, binary.LittleEndian, vLen); err != nil {
		return err
	}
	for _, val := range v {
		if err := binary.Write(w, binary.LittleEndian, val); err != nil {
			return err
		}
	}
	return nil
}

// Decode deserializes the point from binary format
func (p *PointV2) Decode(r io.Reader) error {
	// Read format version
	var version uint8
	if err := binary.Read(r, binary.LittleEndian, &version); err != nil {
		return err
	}
	if version != 2 {
		return fmt.Errorf("unsupported point format version: %d", version)
	}

	// Read ID
	var idLen uint16
	if err := binary.Read(r, binary.LittleEndian, &idLen); err != nil {
		return err
	}
	idBytes := make([]byte, idLen)
	if _, err := io.ReadFull(r, idBytes); err != nil {
		return err
	}
	p.ID = string(idBytes)

	// Read default vector
	var err error
	p.Vector, err = readVector(r)
	if err != nil {
		return err
	}

	// Read named vectors
	var vecCount uint16
	if err := binary.Read(r, binary.LittleEndian, &vecCount); err != nil {
		return err
	}
	if vecCount > 0 {
		p.Vectors = make(NamedVectors)
		for i := uint16(0); i < vecCount; i++ {
			// Read name
			var nameLen uint8
			if err := binary.Read(r, binary.LittleEndian, &nameLen); err != nil {
				return err
			}
			nameBytes := make([]byte, nameLen)
			if _, err := io.ReadFull(r, nameBytes); err != nil {
				return err
			}
			name := string(nameBytes)

			// Read vector
			vec, err := readVector(r)
			if err != nil {
				return err
			}
			p.Vectors[name] = vec
		}
	}

	// Read payload
	var payloadLen uint32
	if err := binary.Read(r, binary.LittleEndian, &payloadLen); err != nil {
		return err
	}
	if payloadLen > 0 {
		payloadBytes := make([]byte, payloadLen)
		if _, err := io.ReadFull(r, payloadBytes); err != nil {
			return err
		}
		if err := json.Unmarshal(payloadBytes, &p.Payload); err != nil {
			return err
		}
	}

	// Read version
	return binary.Read(r, binary.LittleEndian, &p.Version)
}

func readVector(r io.Reader) (Vector, error) {
	var dim uint32
	if err := binary.Read(r, binary.LittleEndian, &dim); err != nil {
		return nil, err
	}
	if dim == 0 {
		return nil, nil
	}
	vec := make(Vector, dim)
	for i := range vec {
		if err := binary.Read(r, binary.LittleEndian, &vec[i]); err != nil {
			return nil, err
		}
	}
	return vec, nil
}

// ScoredPointV2 represents a point with a similarity/distance score
type ScoredPointV2 struct {
	*PointV2
	Score      float32 `json:"score"`
	VectorName string  `json:"vector_name,omitempty"` // Which vector was used for scoring
}

// SearchResultV2 represents the result of a vector search with named vector support
type SearchResultV2 struct {
	Points     []ScoredPointV2 `json:"points"`
	VectorName string          `json:"vector_name,omitempty"`
	TookMs     int64           `json:"took_ms"`
}
