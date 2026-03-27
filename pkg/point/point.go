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

// safeIntToUint32 safely converts int to uint32 with bounds checking
func safeIntToUint32(v int) (uint32, error) {
	if v < 0 || v > math.MaxUint32 {
		return 0, errors.New("integer overflow: value out of uint32 range")
	}
	return uint32(v), nil
}

// safeIntToUint16 safely converts int to uint16 with bounds checking
func safeIntToUint16(v int) (uint16, error) {
	if v < 0 || v > math.MaxUint16 {
		return 0, errors.New("integer overflow: value out of uint16 range")
	}
	return uint16(v), nil
}

type Point struct {
	ID           string                 `json:"id"`
	Vector       Vector                 `json:"vector"`
	Payload      map[string]interface{} `json:"payload,omitempty"`
	NamedVectors map[string]Vector      `json:"named_vectors,omitempty"`
	Sparse       *SparseVector          `json:"sparse,omitempty"`
	MultiVectors map[string][][]float32 `json:"multi_vectors,omitempty"`
	Version      uint64                 `json:"-"` // Internal version for optimistic locking
}

// SparseVector is a sparse mapping of token integers to semantic weights
type SparseVector struct {
	Indices []uint32  `json:"indices"`
	Values  []float32 `json:"values"`
}

// Vector is a dense floating-point vector
type Vector []float32

// NewPoint creates a new point with a generated UUID
func NewPoint(vector Vector, payload map[string]interface{}) *Point {
	return &Point{
		ID:      uuid.New().String(),
		Vector:  vector,
		Payload: payload,
		Version: 1,
	}
}

// NewPointWithID creates a point with a specific ID
func NewPointWithID(id string, vector Vector, payload map[string]interface{}) *Point {
	return &Point{
		ID:      id,
		Vector:  vector,
		Payload: payload,
		Version: 1,
	}
}

// Dimension returns the dimension of the vector
func (p *Point) Dimension() int {
	return len(p.Vector)
}

// Clone creates a deep copy of the point
func (p *Point) Clone() *Point {
	clone := &Point{
		ID:      p.ID,
		Vector:  make(Vector, len(p.Vector)),
		Version: p.Version,
	}
	copy(clone.Vector, p.Vector)

	if p.Payload != nil {
		clone.Payload = make(map[string]interface{})
		for k, v := range p.Payload {
			clone.Payload[k] = v
		}
	}
	if p.NamedVectors != nil {
		clone.NamedVectors = make(map[string]Vector)
		for k, v := range p.NamedVectors {
			newVec := make(Vector, len(v))
			copy(newVec, v)
			clone.NamedVectors[k] = newVec
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
func (p *Point) Validate() error {
	if p.ID == "" {
		return ErrEmptyID
	}
	if len(p.Vector) == 0 && len(p.NamedVectors) == 0 && p.Sparse == nil {
		return ErrEmptyVector
	}
	for i, v := range p.Vector {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return &InvalidVectorError{Index: i, Value: v}
		}
	}
	for _, vec := range p.NamedVectors {
		for i, v := range vec {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return &InvalidVectorError{Index: i, Value: v}
			}
		}
	}
	return nil
}

// Normalize normalizes the vector to unit length (L2 normalization)
func (p *Point) Normalize() {
	p.Vector.Normalize()
}

// Normalize normalizes the vector to unit length
func (v Vector) Normalize() {
	var norm float32
	for _, val := range v {
		norm += val * val
	}
	if norm == 0 {
		return
	}
	norm = float32(math.Sqrt(float64(norm)))
	for i := range v {
		v[i] /= norm
	}
}

// Magnitude returns the L2 norm of the vector
func (v Vector) Magnitude() float32 {
	var sum float32
	for _, val := range v {
		sum += val * val
	}
	return float32(math.Sqrt(float64(sum)))
}

// Encode serializes the point to binary format
func (p *Point) Encode(w io.Writer) error {
	// Write ID length and ID with bounds checking
	idBytes := []byte(p.ID)
	idLen, err := safeIntToUint16(len(idBytes))
	if err != nil {
		return fmt.Errorf("point ID too long: %w", err)
	}
	if err := binary.Write(w, binary.LittleEndian, idLen); err != nil {
		return err
	}
	if _, err := w.Write(idBytes); err != nil {
		return err
	}

	// Write vector dimension and values with bounds checking
	vecLen, err := safeIntToUint32(len(p.Vector))
	if err != nil {
		return fmt.Errorf("vector too large: %w", err)
	}
	if err := binary.Write(w, binary.LittleEndian, vecLen); err != nil {
		return err
	}
	for _, v := range p.Vector {
		if err := binary.Write(w, binary.LittleEndian, v); err != nil {
			return err
		}
	}

	// Write payload as JSON with bounds checking
	payloadBytes, err := json.Marshal(p.Payload)
	if err != nil {
		return err
	}
	payloadLen, err := safeIntToUint32(len(payloadBytes))
	if err != nil {
		return fmt.Errorf("payload too large: %w", err)
	}
	if err := binary.Write(w, binary.LittleEndian, payloadLen); err != nil {
		return err
	}
	if _, err := w.Write(payloadBytes); err != nil {
		return err
	}

	// Write NamedVectors
	numNamed, err := safeIntToUint16(len(p.NamedVectors))
	if err != nil {
		return fmt.Errorf("too many named vectors: %w", err)
	}
	if err := binary.Write(w, binary.LittleEndian, numNamed); err != nil {
		return err
	}
	for name, vec := range p.NamedVectors {
		nameBytes := []byte(name)
		nameLen, err := safeIntToUint16(len(nameBytes))
		if err != nil {
			return fmt.Errorf("named vector key too long: %w", err)
		}
		if err := binary.Write(w, binary.LittleEndian, nameLen); err != nil {
			return err
		}
		if _, err := w.Write(nameBytes); err != nil {
			return err
		}

		vLen, err := safeIntToUint32(len(vec))
		if err != nil {
			return fmt.Errorf("named vector too large: %w", err)
		}
		if err := binary.Write(w, binary.LittleEndian, vLen); err != nil {
			return err
		}
		for _, v := range vec {
			if err := binary.Write(w, binary.LittleEndian, v); err != nil {
				return err
			}
		}
	}

	// Write version
	if err := binary.Write(w, binary.LittleEndian, p.Version); err != nil {
		return err
	}

	// Write SparseVector (appended safely after Version for protocol leniency)
	hasSparse := p.Sparse != nil
	if err := binary.Write(w, binary.LittleEndian, hasSparse); err != nil {
		return err
	}
	if hasSparse {
		sparseLen, err := safeIntToUint32(len(p.Sparse.Indices))
		if err != nil {
			return fmt.Errorf("sparse indices too large: %w", err)
		}
		if err := binary.Write(w, binary.LittleEndian, sparseLen); err != nil {
			return err
		}
		for _, idx := range p.Sparse.Indices {
			if err := binary.Write(w, binary.LittleEndian, idx); err != nil {
				return err
			}
		}
		for _, val := range p.Sparse.Values {
			if err := binary.Write(w, binary.LittleEndian, val); err != nil {
				return err
			}
		}
	}
	return nil
}

// Decode deserializes the point from binary format
func (p *Point) Decode(r io.Reader) error {
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

	// Read vector
	var dim uint32
	if err := binary.Read(r, binary.LittleEndian, &dim); err != nil {
		return err
	}
	p.Vector = make(Vector, dim)
	for i := range p.Vector {
		if err := binary.Read(r, binary.LittleEndian, &p.Vector[i]); err != nil {
			return err
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

	// Read NamedVectors
	var numNamed uint16
	if err := binary.Read(r, binary.LittleEndian, &numNamed); err != nil {
		if errors.Is(err, io.EOF) {
			// Backwards compatibility for points without NamedVectors (legacy)
			p.Version = 1
			return nil
		}
		return err
	}
	if numNamed > 0 {
		p.NamedVectors = make(map[string]Vector)
		for i := uint16(0); i < numNamed; i++ {
			var nLen uint16
			if err := binary.Read(r, binary.LittleEndian, &nLen); err != nil {
				return err
			}
			nBytes := make([]byte, nLen)
			if _, err := io.ReadFull(r, nBytes); err != nil {
				return err
			}
			name := string(nBytes)

			var vLen uint32
			if err := binary.Read(r, binary.LittleEndian, &vLen); err != nil {
				return err
			}
			vec := make(Vector, vLen)
			for j := range vec {
				if err := binary.Read(r, binary.LittleEndian, &vec[j]); err != nil {
					return err
				}
			}
			p.NamedVectors[name] = vec
		}
	}

	// Read version
	err := binary.Read(r, binary.LittleEndian, &p.Version)
	if errors.Is(err, io.EOF) {
		p.Version = 1
		return nil
	}
	if err != nil {
		return err
	}

	// Read SparseVector sequentially backwards
	var hasSparse bool
	err = binary.Read(r, binary.LittleEndian, &hasSparse)
	if errors.Is(err, io.EOF) {
		return nil // Legacy points lack sparse blocks
	}
	if err != nil {
		return err
	}

	if hasSparse {
		var sparseLen uint32
		if err := binary.Read(r, binary.LittleEndian, &sparseLen); err != nil {
			return err
		}

		p.Sparse = &SparseVector{
			Indices: make([]uint32, sparseLen),
			Values:  make([]float32, sparseLen),
		}
		for i := range p.Sparse.Indices {
			if err := binary.Read(r, binary.LittleEndian, &p.Sparse.Indices[i]); err != nil {
				return err
			}
		}
		for i := range p.Sparse.Values {
			if err := binary.Read(r, binary.LittleEndian, &p.Sparse.Values[i]); err != nil {
				return err
			}
		}
	}

	return nil
}

// ScoredPoint represents a point with a similarity/distance score
type ScoredPoint struct {
	*Point
	Score float32 `json:"score"`
}

// SearchResult represents the result of a vector search
type SearchResult struct {
	Points []ScoredPoint `json:"points"`
	Took   int64         `json:"took_ms"`
}

// Error types
var (
	ErrEmptyID     = errors.New("point ID cannot be empty")
	ErrEmptyVector = errors.New("vector cannot be empty")
)

// InvalidVectorError indicates an invalid value in the vector
type InvalidVectorError struct {
	Index int
	Value float32
}

func (e *InvalidVectorError) Error() string {
	return "invalid vector value at index"
}
