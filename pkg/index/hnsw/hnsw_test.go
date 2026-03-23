package hnsw

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/point"
)

func generateRandomVector(dim int) point.Vector {
	v := make(point.Vector, dim)
	for i := range v {
		v[i] = rand.Float32()*2 - 1
	}
	return v
}

func TestHNSWNew(t *testing.T) {
	cfg := &Config{
		M:              16,
		EfConstruction: 200,
		EfSearch:       100,
		MaxElements:    1000,
		Metric:         config.MetricCosine,
		Dimension:      128,
	}

	index, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if index.M != 16 {
		t.Errorf("M = %d, want 16", index.M)
	}

	if index.dimension != 128 {
		t.Errorf("dimension = %d, want 128", index.dimension)
	}
}

func TestHNSWNewInvalidM(t *testing.T) {
	cfg := &Config{
		M:         1,
		Dimension: 128,
	}

	_, err := New(cfg)
	if err == nil {
		t.Error("Expected error for M < 2")
	}
}

func TestHNSWNewInvalidDimension(t *testing.T) {
	cfg := &Config{
		M:         16,
		Dimension: 0,
	}

	_, err := New(cfg)
	if err == nil {
		t.Error("Expected error for dimension <= 0")
	}
}

func TestHNSWInsert(t *testing.T) {
	index := createTestIndex(128, 100)

	p := point.NewPointWithID("test-1", generateRandomVector(128), nil)
	err := index.Insert(p)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	if index.Size() != 1 {
		t.Errorf("Size() = %d, want 1", index.Size())
	}
}

func TestHNSWInsertDuplicate(t *testing.T) {
	index := createTestIndex(128, 100)

	p := point.NewPointWithID("test-1", generateRandomVector(128), nil)
	index.Insert(p)

	err := index.Insert(p)
	if err != ErrPointExists {
		t.Errorf("Expected ErrPointExists, got %v", err)
	}
}

func TestHNSWInsertDimensionMismatch(t *testing.T) {
	index := createTestIndex(128, 100)

	p := point.NewPointWithID("test-1", generateRandomVector(64), nil)
	err := index.Insert(p)

	if err != ErrDimensionMismatch {
		t.Errorf("Expected ErrDimensionMismatch, got %v", err)
	}
}

func TestHNSWGet(t *testing.T) {
	index := createTestIndex(4, 100)

	original := point.NewPointWithID("test-1", point.Vector{1, 2, 3, 4}, map[string]interface{}{"key": "value"})
	index.Insert(original)

	retrieved, err := index.Get("test-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if retrieved.ID != original.ID {
		t.Errorf("ID = %s, want %s", retrieved.ID, original.ID)
	}

	if len(retrieved.Vector) != len(original.Vector) {
		t.Error("Vector length mismatch")
	}
}

func TestHNSWGetNotFound(t *testing.T) {
	index := createTestIndex(128, 100)

	_, err := index.Get("nonexistent")
	if err != ErrPointNotFound {
		t.Errorf("Expected ErrPointNotFound, got %v", err)
	}
}

func TestHNSWDelete(t *testing.T) {
	index := createTestIndex(128, 100)

	p := point.NewPointWithID("test-1", generateRandomVector(128), nil)
	index.Insert(p)

	err := index.Delete("test-1")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = index.Get("test-1")
	if err != ErrPointNotFound {
		t.Error("Point should not be found after deletion")
	}
}

func TestHNSWSearch(t *testing.T) {
	index := createTestIndex(4, 100)

	// Insert some points
	points := []point.Vector{
		{1, 0, 0, 0},
		{0, 1, 0, 0},
		{0, 0, 1, 0},
		{0.9, 0.1, 0, 0}, // Similar to first
	}

	for i, v := range points {
		p := point.NewPointWithID(fmt.Sprintf("p%d", i), v, nil)
		index.Insert(p)
	}

	// Search for similar to first vector
	query := point.Vector{1, 0, 0, 0}
	results, err := index.Search(query, 2)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

func TestHNSWSearchEmpty(t *testing.T) {
	index := createTestIndex(128, 100)

	results, err := index.Search(generateRandomVector(128), 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if results != nil && len(results) != 0 {
		t.Error("Expected nil or empty results for empty index")
	}
}

func TestHNSWSearchDimensionMismatch(t *testing.T) {
	index := createTestIndex(128, 100)

	_, err := index.Search(generateRandomVector(64), 10)
	if err != ErrDimensionMismatch {
		t.Errorf("Expected ErrDimensionMismatch, got %v", err)
	}
}

func TestHNSWSearchWithFilter(t *testing.T) {
	index := createTestIndex(4, 100)

	// Insert points with payloads
	for i := 0; i < 10; i++ {
		p := point.NewPointWithID(
			fmt.Sprintf("p%d", i),
			generateRandomVector(4),
			map[string]interface{}{"category": i % 2},
		)
		index.Insert(p)
	}

	// Search with filter
	params := &SearchParams{
		K:  5,
		Ef: 50,
		Filter: func(id string, payload map[string]interface{}) bool {
			cat, ok := payload["category"]
			if !ok {
				return false
			}
			return cat == 0 // Only category 0
		},
	}

	results, err := index.SearchWithFilter(generateRandomVector(4), params)
	if err != nil {
		t.Fatalf("SearchWithFilter() error = %v", err)
	}

	// All results should have category 0
	for _, r := range results {
		p, _ := index.Get(index.nodes[r.ID].ID)
		if p.Payload["category"] != 0 {
			t.Error("Filter not applied correctly")
		}
	}
}

func TestHNSWBatchSearch(t *testing.T) {
	index := createTestIndex(4, 100)

	// Insert points
	for i := 0; i < 20; i++ {
		p := point.NewPointWithID(fmt.Sprintf("p%d", i), generateRandomVector(4), nil)
		index.Insert(p)
	}

	// Batch search
	queries := make([]point.Vector, 5)
	for i := range queries {
		queries[i] = generateRandomVector(4)
	}

	results, err := index.BatchSearch(queries, 3)
	if err != nil {
		t.Fatalf("BatchSearch() error = %v", err)
	}

	if len(results) != 5 {
		t.Errorf("Expected 5 result sets, got %d", len(results))
	}
}

func TestHNSWRecommend(t *testing.T) {
	index := createTestIndex(4, 100)

	// Insert points
	points := []point.Vector{
		{1, 0, 0, 0},
		{0.9, 0.1, 0, 0},
		{0.8, 0.2, 0, 0},
		{0, 1, 0, 0},
		{0, 0, 1, 0},
	}

	for i, v := range points {
		p := point.NewPointWithID(fmt.Sprintf("p%d", i), v, nil)
		index.Insert(p)
	}

	results, err := index.Recommend("p0", 2)
	if err != nil {
		t.Fatalf("Recommend() error = %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 recommendations, got %d", len(results))
	}

	// Results should not include the query point
	for _, r := range results {
		if index.nodes[r.ID].ID == "p0" {
			t.Error("Recommend should not include query point")
		}
	}
}

func TestHNSWRangeSearch(t *testing.T) {
	index := createTestIndex(4, 100)

	// Insert points at known distances
	points := []point.Vector{
		{1, 0, 0, 0},     // distance 0 from query
		{0.9, 0.1, 0, 0}, // small distance
		{0, 1, 0, 0},     // distance ~1 (orthogonal)
	}

	for i, v := range points {
		p := point.NewPointWithID(fmt.Sprintf("p%d", i), v, nil)
		index.Insert(p)
	}

	// Range search
	query := point.Vector{1, 0, 0, 0}
	results, err := index.RangeSearch(query, 0.5)
	if err != nil {
		t.Fatalf("RangeSearch() error = %v", err)
	}

	// Should find points within radius
	if len(results) == 0 {
		t.Error("Expected at least one result within radius")
	}

	for _, r := range results {
		if r.Distance > 0.5 {
			t.Errorf("Result distance %v exceeds radius 0.5", r.Distance)
		}
	}
}

func TestHNSWSetEfSearch(t *testing.T) {
	index := createTestIndex(128, 100)
	index.SetEfSearch(200)

	if index.efSearch != 200 {
		t.Errorf("efSearch = %d, want 200", index.efSearch)
	}
}

func createTestIndex(dimension, maxElements int) *HNSW {
	cfg := &Config{
		M:              16,
		EfConstruction: 100,
		EfSearch:       50,
		MaxElements:    maxElements,
		Metric:         config.MetricCosine,
		Dimension:      dimension,
	}

	index, _ := New(cfg)
	return index
}

func BenchmarkHNSWInsert(b *testing.B) {
	index := createTestIndex(128, b.N+1000)

	points := make([]*point.Point, b.N)
	for i := range points {
		points[i] = point.NewPointWithID(fmt.Sprintf("p%d", i), generateRandomVector(128), nil)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		index.Insert(points[i])
	}
}

func BenchmarkHNSWSearch(b *testing.B) {
	index := createTestIndex(128, 10000)

	// Pre-populate
	for i := 0; i < 10000; i++ {
		p := point.NewPointWithID(fmt.Sprintf("p%d", i), generateRandomVector(128), nil)
		index.Insert(p)
	}

	query := generateRandomVector(128)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		index.Search(query, 10)
	}
}
