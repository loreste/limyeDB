package multimodel

import (
	"testing"
)

func TestNewMultiModelIndex(t *testing.T) {
	configs := []VectorConfig{
		{Name: "dense", Type: VectorTypeDense, Dimension: 128, Metric: "cosine"},
		{Name: "sparse", Type: VectorTypeSparse, Metric: "dot_product"},
	}

	idx := NewMultiModelIndex(configs)

	if idx == nil {
		t.Fatal("expected non-nil index")
	}
	if len(idx.configs) != 2 {
		t.Errorf("expected 2 configs, got %d", len(idx.configs))
	}
	if idx.denseIdx["dense"] == nil {
		t.Error("expected dense index to be created")
	}
	if idx.sparseIdx["sparse"] == nil {
		t.Error("expected sparse index to be created")
	}
}

func TestMultiModelIndex_Insert(t *testing.T) {
	configs := []VectorConfig{
		{Name: "dense", Type: VectorTypeDense, Dimension: 4, Metric: "cosine"},
	}

	idx := NewMultiModelIndex(configs)

	point := &MultiVectorPoint{
		ID: "point1",
		Vectors: map[string]interface{}{
			"dense": []float32{0.1, 0.2, 0.3, 0.4},
		},
		Payload: map[string]interface{}{
			"category": "test",
		},
	}

	err := idx.Insert(point)
	if err != nil {
		t.Errorf("failed to insert: %v", err)
	}

	if len(idx.points) != 1 {
		t.Errorf("expected 1 point, got %d", len(idx.points))
	}
}

func TestMultiModelIndex_InsertDimensionMismatch(t *testing.T) {
	configs := []VectorConfig{
		{Name: "dense", Type: VectorTypeDense, Dimension: 4, Metric: "cosine"},
	}

	idx := NewMultiModelIndex(configs)

	point := &MultiVectorPoint{
		ID: "point1",
		Vectors: map[string]interface{}{
			"dense": []float32{0.1, 0.2, 0.3}, // Wrong dimension
		},
	}

	err := idx.Insert(point)
	if err == nil {
		t.Error("expected dimension mismatch error")
	}
}

func TestMultiModelIndex_InsertUnknownVector(t *testing.T) {
	configs := []VectorConfig{
		{Name: "dense", Type: VectorTypeDense, Dimension: 4, Metric: "cosine"},
	}

	idx := NewMultiModelIndex(configs)

	point := &MultiVectorPoint{
		ID: "point1",
		Vectors: map[string]interface{}{
			"unknown": []float32{0.1, 0.2, 0.3, 0.4},
		},
	}

	err := idx.Insert(point)
	if err == nil {
		t.Error("expected unknown vector type error")
	}
}

func TestMultiModelIndex_Search(t *testing.T) {
	configs := []VectorConfig{
		{Name: "dense", Type: VectorTypeDense, Dimension: 4, Metric: "cosine"},
	}

	idx := NewMultiModelIndex(configs)

	// Insert some points
	points := []*MultiVectorPoint{
		{ID: "p1", Vectors: map[string]interface{}{"dense": []float32{1.0, 0.0, 0.0, 0.0}}},
		{ID: "p2", Vectors: map[string]interface{}{"dense": []float32{0.9, 0.1, 0.0, 0.0}}},
		{ID: "p3", Vectors: map[string]interface{}{"dense": []float32{0.0, 1.0, 0.0, 0.0}}},
	}

	for _, p := range points {
		idx.Insert(p)
	}

	// Search for similar to p1
	query := []float32{1.0, 0.0, 0.0, 0.0}
	results, err := idx.Search("dense", query, 2)
	if err != nil {
		t.Errorf("search failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	// First result should be p1 (exact match)
	if results[0].ID != "p1" {
		t.Errorf("expected first result to be p1, got %s", results[0].ID)
	}
}

func TestMultiModelIndex_SearchUnknownVector(t *testing.T) {
	configs := []VectorConfig{
		{Name: "dense", Type: VectorTypeDense, Dimension: 4, Metric: "cosine"},
	}

	idx := NewMultiModelIndex(configs)

	_, err := idx.Search("unknown", []float32{1.0, 0.0, 0.0, 0.0}, 10)
	if err == nil {
		t.Error("expected error for unknown vector type")
	}
}

func TestMultiModelIndex_SparseVector(t *testing.T) {
	configs := []VectorConfig{
		{Name: "sparse", Type: VectorTypeSparse, Metric: "dot_product"},
	}

	idx := NewMultiModelIndex(configs)

	point := &MultiVectorPoint{
		ID: "point1",
		Vectors: map[string]interface{}{
			"sparse": &SparseVector{
				Indices: []uint32{0, 5, 10},
				Values:  []float32{1.0, 0.5, 0.3},
			},
		},
	}

	err := idx.Insert(point)
	if err != nil {
		t.Errorf("failed to insert sparse vector: %v", err)
	}

	query := &SparseVector{
		Indices: []uint32{0, 5, 10},
		Values:  []float32{1.0, 0.5, 0.3},
	}

	results, err := idx.Search("sparse", query, 1)
	if err != nil {
		t.Errorf("search failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestMultiModelIndex_HybridSearch(t *testing.T) {
	configs := []VectorConfig{
		{Name: "dense", Type: VectorTypeDense, Dimension: 4, Metric: "cosine"},
		{Name: "sparse", Type: VectorTypeSparse, Metric: "dot_product"},
	}

	idx := NewMultiModelIndex(configs)

	// Insert points with both vector types
	points := []*MultiVectorPoint{
		{
			ID: "p1",
			Vectors: map[string]interface{}{
				"dense":  []float32{1.0, 0.0, 0.0, 0.0},
				"sparse": &SparseVector{Indices: []uint32{0}, Values: []float32{1.0}},
			},
		},
		{
			ID: "p2",
			Vectors: map[string]interface{}{
				"dense":  []float32{0.0, 1.0, 0.0, 0.0},
				"sparse": &SparseVector{Indices: []uint32{1}, Values: []float32{1.0}},
			},
		},
		{
			ID: "p3",
			Vectors: map[string]interface{}{
				"dense":  []float32{0.5, 0.5, 0.0, 0.0},
				"sparse": &SparseVector{Indices: []uint32{0, 1}, Values: []float32{0.5, 0.5}},
			},
		},
	}

	for _, p := range points {
		idx.Insert(p)
	}

	queries := map[string]interface{}{
		"dense":  []float32{1.0, 0.0, 0.0, 0.0},
		"sparse": &SparseVector{Indices: []uint32{0}, Values: []float32{1.0}},
	}

	weights := map[string]float32{
		"dense":  0.5,
		"sparse": 0.5,
	}

	results, err := idx.HybridSearch(queries, 2, weights)
	if err != nil {
		t.Errorf("hybrid search failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestDenseIndex(t *testing.T) {
	config := VectorConfig{Name: "test", Type: VectorTypeDense, Dimension: 4}
	idx := NewDenseIndex(config)

	idx.Insert("p1", []float32{1.0, 0.0, 0.0, 0.0})
	idx.Insert("p2", []float32{0.0, 1.0, 0.0, 0.0})
	idx.Insert("p3", []float32{0.5, 0.5, 0.0, 0.0})

	results := idx.Search([]float32{1.0, 0.0, 0.0, 0.0}, 2)

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	if results[0].ID != "p1" {
		t.Errorf("expected p1 as first result, got %s", results[0].ID)
	}
}

func TestSparseIndex(t *testing.T) {
	config := VectorConfig{Name: "test", Type: VectorTypeSparse}
	idx := NewSparseIndex(config)

	idx.Insert("p1", &SparseVector{Indices: []uint32{0, 1}, Values: []float32{1.0, 0.5}})
	idx.Insert("p2", &SparseVector{Indices: []uint32{2, 3}, Values: []float32{1.0, 0.5}})
	idx.Insert("p3", &SparseVector{Indices: []uint32{0, 2}, Values: []float32{0.5, 0.5}})

	query := &SparseVector{Indices: []uint32{0, 1}, Values: []float32{1.0, 0.5}}
	results := idx.Search(query, 2)

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	if results[0].ID != "p1" {
		t.Errorf("expected p1 as first result, got %s", results[0].ID)
	}
}

func TestCosineSimilarity(t *testing.T) {
	a := []float32{1.0, 0.0, 0.0, 0.0}
	b := []float32{1.0, 0.0, 0.0, 0.0}

	sim := cosineSimilarity(a, b)
	if sim < 0.99 || sim > 1.01 {
		t.Errorf("expected similarity ~1.0, got %f", sim)
	}

	c := []float32{0.0, 1.0, 0.0, 0.0}
	sim = cosineSimilarity(a, c)
	if sim < -0.01 || sim > 0.01 {
		t.Errorf("expected similarity ~0.0, got %f", sim)
	}
}

func TestSparseDotProduct(t *testing.T) {
	a := &SparseVector{Indices: []uint32{0, 2, 4}, Values: []float32{1.0, 2.0, 3.0}}
	b := &SparseVector{Indices: []uint32{0, 2, 4}, Values: []float32{1.0, 2.0, 3.0}}

	dot := sparseDotProduct(a, b)
	expected := float32(1.0*1.0 + 2.0*2.0 + 3.0*3.0) // 14.0

	if dot != expected {
		t.Errorf("expected %f, got %f", expected, dot)
	}

	// Test with non-overlapping indices
	c := &SparseVector{Indices: []uint32{1, 3, 5}, Values: []float32{1.0, 2.0, 3.0}}
	dot = sparseDotProduct(a, c)
	if dot != 0 {
		t.Errorf("expected 0 for non-overlapping, got %f", dot)
	}
}

func TestFuseResults(t *testing.T) {
	configs := []VectorConfig{
		{Name: "dense", Type: VectorTypeDense, Dimension: 4},
	}
	idx := NewMultiModelIndex(configs)

	results := map[string][]SearchResult{
		"vec1": {
			{ID: "p1", Score: 1.0},
			{ID: "p2", Score: 0.9},
		},
		"vec2": {
			{ID: "p2", Score: 1.0},
			{ID: "p3", Score: 0.9},
		},
	}

	weights := map[string]float32{"vec1": 1.0, "vec2": 1.0}
	fused := idx.fuseResults(results, weights, 2)

	if len(fused) != 2 {
		t.Errorf("expected 2 results, got %d", len(fused))
	}

	// p2 should be first as it appears in both result sets
	if fused[0].ID != "p2" {
		t.Errorf("expected p2 first (appears in both), got %s", fused[0].ID)
	}
}

func BenchmarkDenseIndex_Insert(b *testing.B) {
	config := VectorConfig{Name: "test", Type: VectorTypeDense, Dimension: 128}
	idx := NewDenseIndex(config)
	vector := make([]float32, 128)
	for i := range vector {
		vector[i] = float32(i) / 128.0
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Insert("point", vector)
	}
}

func BenchmarkDenseIndex_Search(b *testing.B) {
	config := VectorConfig{Name: "test", Type: VectorTypeDense, Dimension: 128}
	idx := NewDenseIndex(config)

	// Insert 1000 vectors
	for i := 0; i < 1000; i++ {
		vector := make([]float32, 128)
		for j := range vector {
			vector[j] = float32(i+j) / 1000.0
		}
		idx.Insert(string(rune(i)), vector)
	}

	query := make([]float32, 128)
	for i := range query {
		query[i] = 0.5
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Search(query, 10)
	}
}
