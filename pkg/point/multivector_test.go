package point

import (
	"testing"
)

func TestMultiVector(t *testing.T) {
	mv := NewMultiVector("doc1")

	mv.SetVector("dense", Vector{1.0, 2.0, 3.0})
	mv.SetVector("sparse", Vector{0.1, 0.2})

	// Test GetVector
	dense, ok := mv.GetVector("dense")
	if !ok {
		t.Error("Expected to find 'dense' vector")
	}
	if len(dense) != 3 {
		t.Errorf("Expected dimension 3, got %d", len(dense))
	}

	// Test HasVector
	if !mv.HasVector("dense") {
		t.Error("Expected HasVector('dense') to be true")
	}
	if mv.HasVector("nonexistent") {
		t.Error("Expected HasVector('nonexistent') to be false")
	}

	// Test VectorNames
	names := mv.VectorNames()
	if len(names) != 2 {
		t.Errorf("Expected 2 vector names, got %d", len(names))
	}
}

func TestMultiVectorValidation(t *testing.T) {
	// Empty ID
	mv1 := &MultiVector{ID: "", Vectors: map[string]Vector{"v": {1.0}}}
	if err := mv1.Validate(); err == nil {
		t.Error("Expected error for empty ID")
	}

	// No vectors
	mv2 := &MultiVector{ID: "doc1", Vectors: map[string]Vector{}}
	if err := mv2.Validate(); err == nil {
		t.Error("Expected error for no vectors")
	}

	// Empty vector
	mv3 := &MultiVector{ID: "doc1", Vectors: map[string]Vector{"empty": {}}}
	if err := mv3.Validate(); err == nil {
		t.Error("Expected error for empty vector")
	}

	// Valid
	mv4 := &MultiVector{ID: "doc1", Vectors: map[string]Vector{"v": {1.0}}}
	if err := mv4.Validate(); err != nil {
		t.Errorf("Expected valid, got error: %v", err)
	}
}

func TestMultiVectorToPoint(t *testing.T) {
	mv := NewMultiVectorWithVectors("doc1", map[string]Vector{
		"default": {1.0, 2.0, 3.0},
		"other":   {4.0, 5.0},
	}, map[string]interface{}{"key": "value"})

	// Convert to Point using default vector
	p, err := mv.ToPoint("default")
	if err != nil {
		t.Fatalf("ToPoint failed: %v", err)
	}

	if p.ID != "doc1" {
		t.Errorf("Expected ID 'doc1', got '%s'", p.ID)
	}
	if len(p.Vector) != 3 {
		t.Errorf("Expected vector length 3, got %d", len(p.Vector))
	}
	if p.Payload["key"] != "value" {
		t.Error("Payload not preserved")
	}

	// Non-existent vector
	_, err = mv.ToPoint("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent vector")
	}
}

func TestFromPoint(t *testing.T) {
	p := &Point{
		ID:      "doc1",
		Vector:  Vector{1.0, 2.0, 3.0},
		Payload: map[string]interface{}{"key": "value"},
	}

	mv := FromPoint(p, "default")

	if mv.ID != "doc1" {
		t.Errorf("Expected ID 'doc1', got '%s'", mv.ID)
	}
	if !mv.HasVector("default") {
		t.Error("Expected to have 'default' vector")
	}
	if mv.Payload["key"] != "value" {
		t.Error("Payload not preserved")
	}
}

func TestMultiVectorEncodeDecode(t *testing.T) {
	mv := NewMultiVectorWithVectors("doc1", map[string]Vector{
		"dense":  {1.0, 2.0, 3.0},
		"sparse": {0.1, 0.2},
	}, map[string]interface{}{"category": "test"})

	encoded, err := mv.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decoded, err := DecodeMultiVector(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decoded.ID != mv.ID {
		t.Errorf("ID mismatch: expected '%s', got '%s'", mv.ID, decoded.ID)
	}
	if len(decoded.Vectors) != len(mv.Vectors) {
		t.Errorf("Vector count mismatch: expected %d, got %d", len(mv.Vectors), len(decoded.Vectors))
	}
}

func TestMultiVectorConfig(t *testing.T) {
	cfg := NewMultiVectorConfig()

	cfg.AddVector(VectorConfig{
		Name:      "dense",
		Dimension: 128,
		Metric:    "cosine",
	})
	cfg.AddVector(VectorConfig{
		Name:      "sparse",
		Dimension: 64,
		Metric:    "dot",
	})
	cfg.DefaultVector = "dense"

	if err := cfg.Validate(); err != nil {
		t.Errorf("Expected valid config, got error: %v", err)
	}

	// Get vector config
	denseCfg, ok := cfg.GetVectorConfig("dense")
	if !ok {
		t.Error("Expected to find 'dense' config")
	}
	if denseCfg.Dimension != 128 {
		t.Errorf("Expected dimension 128, got %d", denseCfg.Dimension)
	}

	// Invalid: duplicate names
	cfg2 := NewMultiVectorConfig()
	cfg2.AddVector(VectorConfig{Name: "v1", Dimension: 64})
	cfg2.AddVector(VectorConfig{Name: "v1", Dimension: 128})
	if err := cfg2.Validate(); err == nil {
		t.Error("Expected error for duplicate names")
	}

	// Invalid: default vector not found
	cfg3 := NewMultiVectorConfig()
	cfg3.AddVector(VectorConfig{Name: "v1", Dimension: 64})
	cfg3.DefaultVector = "nonexistent"
	if err := cfg3.Validate(); err == nil {
		t.Error("Expected error for non-existent default vector")
	}
}

func TestColBERTVector(t *testing.T) {
	// Document with 3 token vectors
	docTokens := []Vector{
		{1.0, 0.0, 0.0, 0.0},
		{0.0, 1.0, 0.0, 0.0},
		{0.0, 0.0, 1.0, 0.0},
	}
	cv := NewColBERTVector("doc1", docTokens)

	if cv.NumTokens() != 3 {
		t.Errorf("Expected 3 tokens, got %d", cv.NumTokens())
	}
	if cv.Dimension() != 4 {
		t.Errorf("Expected dimension 4, got %d", cv.Dimension())
	}

	// Query with 2 token vectors
	queryTokens := []Vector{
		{0.9, 0.1, 0.0, 0.0}, // Similar to first doc token
		{0.0, 0.0, 0.8, 0.2}, // Similar to third doc token
	}

	score := cv.MaxSim(queryTokens)
	// Expected: max(0.9*1+0.1*0, 0.9*0+0.1*1, 0.9*0+0.1*0) = 0.9 for query token 1
	// Expected: max(0.8*0, 0.8*0, 0.8*1+0.2*0) = 0.8 for query token 2
	// Total: 0.9 + 0.8 = 1.7
	if score < 1.6 || score > 1.8 {
		t.Errorf("Expected MaxSim score ~1.7, got %f", score)
	}
}

func TestMatryoshkaVector(t *testing.T) {
	fullVec := Vector{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0}
	mv := NewMatryoshkaVector("doc1", fullVec, []int{2, 4, 8})

	// Get truncated vectors
	trunc2 := mv.GetTruncated(2)
	if len(trunc2) != 2 {
		t.Errorf("Expected length 2, got %d", len(trunc2))
	}
	if trunc2[0] != 1.0 || trunc2[1] != 2.0 {
		t.Error("Truncated vector values incorrect")
	}

	trunc4 := mv.GetTruncated(4)
	if len(trunc4) != 4 {
		t.Errorf("Expected length 4, got %d", len(trunc4))
	}

	// Full vector when dimension exceeds
	truncFull := mv.GetTruncated(100)
	if len(truncFull) != 8 {
		t.Errorf("Expected full length 8, got %d", len(truncFull))
	}

	// SupportsDimension
	if !mv.SupportsDimension(4) {
		t.Error("Should support dimension 4")
	}
	if mv.SupportsDimension(3) {
		t.Error("Should not support dimension 3")
	}
}

func BenchmarkColBERTMaxSim(b *testing.B) {
	// Document with 100 token vectors of dimension 128
	docTokens := make([]Vector, 100)
	for i := range docTokens {
		docTokens[i] = make(Vector, 128)
		for j := range docTokens[i] {
			docTokens[i][j] = float32(i+j) / 1000
		}
	}
	cv := NewColBERTVector("doc1", docTokens)

	// Query with 10 token vectors
	queryTokens := make([]Vector, 10)
	for i := range queryTokens {
		queryTokens[i] = make(Vector, 128)
		for j := range queryTokens[i] {
			queryTokens[i][j] = float32(i*j) / 1000
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cv.MaxSim(queryTokens)
	}
}
