package point

import (
	"bytes"
	"math"
	"testing"
)

func TestNewPoint(t *testing.T) {
	vector := Vector{1.0, 2.0, 3.0}
	payload := map[string]interface{}{"key": "value"}

	p := NewPoint(vector, payload)

	if p.ID == "" {
		t.Error("Expected non-empty ID")
	}

	if len(p.Vector) != 3 {
		t.Errorf("Expected vector length 3, got %d", len(p.Vector))
	}

	if p.Payload["key"] != "value" {
		t.Error("Payload not set correctly")
	}

	if p.Version != 1 {
		t.Errorf("Expected version 1, got %d", p.Version)
	}
}

func TestNewPointWithID(t *testing.T) {
	p := NewPointWithID("custom-id", Vector{1.0}, nil)

	if p.ID != "custom-id" {
		t.Errorf("Expected ID 'custom-id', got '%s'", p.ID)
	}
}

func TestPointDimension(t *testing.T) {
	p := NewPoint(Vector{1.0, 2.0, 3.0, 4.0}, nil)

	if p.Dimension() != 4 {
		t.Errorf("Expected dimension 4, got %d", p.Dimension())
	}
}

func TestPointClone(t *testing.T) {
	original := NewPointWithID("test", Vector{1.0, 2.0}, map[string]interface{}{"a": 1})
	clone := original.Clone()

	// Check values are equal
	if clone.ID != original.ID {
		t.Error("Clone ID doesn't match")
	}

	if len(clone.Vector) != len(original.Vector) {
		t.Error("Clone vector length doesn't match")
	}

	// Modify clone and verify original unchanged
	clone.Vector[0] = 999.0
	if original.Vector[0] == 999.0 {
		t.Error("Clone should not affect original vector")
	}

	clone.Payload["a"] = 999
	// Note: shallow copy of payload values
}

func TestPointValidate(t *testing.T) {
	tests := []struct {
		name    string
		point   *Point
		wantErr bool
	}{
		{
			name:    "valid point",
			point:   NewPointWithID("id", Vector{1.0, 2.0}, nil),
			wantErr: false,
		},
		{
			name:    "empty ID",
			point:   &Point{ID: "", Vector: Vector{1.0}},
			wantErr: true,
		},
		{
			name:    "empty vector",
			point:   &Point{ID: "id", Vector: Vector{}},
			wantErr: true,
		},
		{
			name:    "NaN in vector",
			point:   &Point{ID: "id", Vector: Vector{float32(math.NaN())}},
			wantErr: true,
		},
		{
			name:    "Inf in vector",
			point:   &Point{ID: "id", Vector: Vector{float32(math.Inf(1))}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.point.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPointNormalize(t *testing.T) {
	p := NewPointWithID("test", Vector{3.0, 4.0}, nil)
	p.Normalize()

	// Check unit length
	var sum float32
	for _, v := range p.Vector {
		sum += v * v
	}
	magnitude := float32(math.Sqrt(float64(sum)))

	if math.Abs(float64(magnitude-1.0)) > 0.001 {
		t.Errorf("Expected unit magnitude, got %v", magnitude)
	}
}

func TestVectorMagnitude(t *testing.T) {
	v := Vector{3.0, 4.0}
	mag := v.Magnitude()

	if math.Abs(float64(mag-5.0)) > 0.001 {
		t.Errorf("Expected magnitude 5.0, got %v", mag)
	}
}

func TestPointEncodeDecode(t *testing.T) {
	original := NewPointWithID("test-id", Vector{1.0, 2.0, 3.0}, map[string]interface{}{
		"category": "test",
		"score":    0.95,
	})
	original.Version = 42

	// Encode
	var buf bytes.Buffer
	if err := original.Encode(&buf); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	// Decode
	decoded := &Point{}
	if err := decoded.Decode(&buf); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	// Verify
	if decoded.ID != original.ID {
		t.Errorf("ID mismatch: got %s, want %s", decoded.ID, original.ID)
	}

	if len(decoded.Vector) != len(original.Vector) {
		t.Errorf("Vector length mismatch: got %d, want %d", len(decoded.Vector), len(original.Vector))
	}

	for i := range original.Vector {
		if decoded.Vector[i] != original.Vector[i] {
			t.Errorf("Vector[%d] mismatch: got %v, want %v", i, decoded.Vector[i], original.Vector[i])
		}
	}

	if decoded.Version != original.Version {
		t.Errorf("Version mismatch: got %d, want %d", decoded.Version, original.Version)
	}

	if decoded.Payload["category"] != "test" {
		t.Error("Payload category mismatch")
	}
}

func TestScoredPoint(t *testing.T) {
	p := NewPointWithID("test", Vector{1.0}, nil)
	sp := ScoredPoint{
		Point: p,
		Score: 0.95,
	}

	if sp.ID != "test" {
		t.Error("ScoredPoint should inherit Point fields")
	}

	if sp.Score != 0.95 {
		t.Error("Score not set correctly")
	}
}

func TestPointEncodeDecodeSparseVector(t *testing.T) {
	original := NewPointWithID("sparse-pt", Vector{1.0, 2.0}, nil)
	original.Version = 7
	original.Sparse = &SparseVector{
		Indices: []uint32{10, 42, 99},
		Values:  []float32{0.5, 1.2, -0.3},
	}

	var buf bytes.Buffer
	if err := original.Encode(&buf); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	decoded := &Point{}
	if err := decoded.Decode(&buf); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if decoded.Sparse == nil {
		t.Fatal("Decoded sparse vector is nil")
	}
	if len(decoded.Sparse.Indices) != 3 {
		t.Fatalf("Expected 3 sparse indices, got %d", len(decoded.Sparse.Indices))
	}
	for i := range original.Sparse.Indices {
		if decoded.Sparse.Indices[i] != original.Sparse.Indices[i] {
			t.Errorf("Sparse index %d: got %d, want %d", i, decoded.Sparse.Indices[i], original.Sparse.Indices[i])
		}
		if decoded.Sparse.Values[i] != original.Sparse.Values[i] {
			t.Errorf("Sparse value %d: got %f, want %f", i, decoded.Sparse.Values[i], original.Sparse.Values[i])
		}
	}
}

func TestPointEncodeDecodeNamedVectors(t *testing.T) {
	original := NewPointWithID("named-pt", Vector{1.0}, nil)
	original.Version = 3
	original.NamedVectors = map[string]Vector{
		"text":  {0.1, 0.2, 0.3},
		"image": {0.4, 0.5},
	}

	var buf bytes.Buffer
	if err := original.Encode(&buf); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	decoded := &Point{}
	if err := decoded.Decode(&buf); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if len(decoded.NamedVectors) != 2 {
		t.Fatalf("Expected 2 named vectors, got %d", len(decoded.NamedVectors))
	}
	for name, vec := range original.NamedVectors {
		dv, ok := decoded.NamedVectors[name]
		if !ok {
			t.Fatalf("Missing named vector %q", name)
		}
		if len(dv) != len(vec) {
			t.Errorf("Named vector %q length mismatch: got %d, want %d", name, len(dv), len(vec))
		}
		for i := range vec {
			if dv[i] != vec[i] {
				t.Errorf("Named vector %q[%d]: got %f, want %f", name, i, dv[i], vec[i])
			}
		}
	}
}

func TestPointEncodeDecodeNoPayload(t *testing.T) {
	original := NewPointWithID("no-payload", Vector{9.0, 8.0, 7.0}, nil)
	original.Version = 1

	var buf bytes.Buffer
	if err := original.Encode(&buf); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	decoded := &Point{}
	if err := decoded.Decode(&buf); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID mismatch: got %s, want %s", decoded.ID, original.ID)
	}
	if len(decoded.Vector) != 3 {
		t.Errorf("Vector length mismatch: got %d, want 3", len(decoded.Vector))
	}
}

func TestVectorNormalizeZero(t *testing.T) {
	v := Vector{0.0, 0.0, 0.0}
	v.Normalize()
	// Should be no-op, not NaN
	for i, val := range v {
		if val != 0.0 {
			t.Errorf("Expected 0 at index %d after normalizing zero vector, got %f", i, val)
		}
	}
}

func TestCloneWithSparseVector(t *testing.T) {
	original := NewPointWithID("sp", Vector{1.0}, nil)
	original.Sparse = &SparseVector{
		Indices: []uint32{1, 2},
		Values:  []float32{0.5, 0.6},
	}

	clone := original.Clone()
	clone.Sparse.Indices[0] = 999
	if original.Sparse.Indices[0] == 999 {
		t.Error("Modifying clone sparse indices should not affect original")
	}
}

func TestCloneWithNamedVectors(t *testing.T) {
	original := NewPointWithID("nv", Vector{1.0}, nil)
	original.NamedVectors = map[string]Vector{
		"a": {1.0, 2.0},
	}

	clone := original.Clone()
	clone.NamedVectors["a"][0] = 999.0
	if original.NamedVectors["a"][0] == 999.0 {
		t.Error("Modifying clone named vector should not affect original")
	}
}

func TestValidateWithNamedVectors(t *testing.T) {
	// Valid: point with only named vectors (no primary vector)
	p := &Point{
		ID: "nv-only",
		NamedVectors: map[string]Vector{
			"text": {1.0, 2.0},
		},
	}
	if err := p.Validate(); err != nil {
		t.Errorf("Expected valid, got error: %v", err)
	}

	// Invalid: NaN in named vector
	p2 := &Point{
		ID: "nv-nan",
		NamedVectors: map[string]Vector{
			"bad": {float32(math.NaN())},
		},
	}
	if err := p2.Validate(); err == nil {
		t.Error("Expected error for NaN in named vector")
	}
}

func TestValidateWithSparseOnly(t *testing.T) {
	p := &Point{
		ID: "sparse-only",
		Sparse: &SparseVector{
			Indices: []uint32{1},
			Values:  []float32{0.5},
		},
	}
	if err := p.Validate(); err != nil {
		t.Errorf("Expected valid sparse-only point, got error: %v", err)
	}
}

func TestInvalidVectorError(t *testing.T) {
	e := &InvalidVectorError{Index: 5, Value: float32(math.NaN())}
	msg := e.Error()
	if msg == "" {
		t.Error("Expected non-empty error message")
	}
}

func BenchmarkPointEncode(b *testing.B) {
	p := NewPointWithID("test", make(Vector, 128), nil)
	for i := range p.Vector {
		p.Vector[i] = float32(i)
	}

	var buf bytes.Buffer
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		buf.Reset()
		p.Encode(&buf)
	}
}

func BenchmarkPointDecode(b *testing.B) {
	p := NewPointWithID("test", make(Vector, 128), nil)
	for i := range p.Vector {
		p.Vector[i] = float32(i)
	}

	var buf bytes.Buffer
	p.Encode(&buf)
	data := buf.Bytes()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		decoded := &Point{}
		reader := bytes.NewReader(data)
		decoded.Decode(reader)
	}
}
