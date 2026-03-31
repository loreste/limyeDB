package diskann

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/limyedb/limyedb/pkg/point"
)

func generateRandomVector(dim int) point.Vector {
	v := make(point.Vector, dim)
	for i := range v {
		v[i] = rand.Float32()*2 - 1 // #nosec G404
	}
	return v
}

func TestNewVamanaGraph(t *testing.T) {
	t.Parallel()

	g := NewVamanaGraph(128, 64, 1.2, "euclidean")
	if g == nil {
		t.Fatal("NewVamanaGraph returned nil")
	}

	if g.dimension != 128 {
		t.Errorf("Expected dimension=128, got %d", g.dimension)
	}
	if g.maxDegree != 64 {
		t.Errorf("Expected maxDegree=64, got %d", g.maxDegree)
	}
	if g.alpha != 1.2 {
		t.Errorf("Expected alpha=1.2, got %f", g.alpha)
	}
}

func TestVamanaInsert(t *testing.T) {
	t.Parallel()

	g := NewVamanaGraph(4, 8, 1.2, "euclidean")

	p := &point.Point{
		ID:     "test-1",
		Vector: point.Vector{1, 0, 0, 0},
	}

	err := g.Insert(p)
	if err != nil {
		t.Fatalf("Insert() failed: %v", err)
	}

	if g.Size() != 1 {
		t.Errorf("Expected Size()=1, got %d", g.Size())
	}
}

func TestVamanaInsertDuplicate(t *testing.T) {
	t.Parallel()

	g := NewVamanaGraph(4, 8, 1.2, "euclidean")

	p := &point.Point{
		ID:     "test-1",
		Vector: point.Vector{1, 0, 0, 0},
	}

	_ = g.Insert(p)
	err := g.Insert(p)

	if err != ErrPointExists {
		t.Errorf("Expected ErrPointExists, got %v", err)
	}
}

func TestVamanaInsertDimensionMismatch(t *testing.T) {
	t.Parallel()

	g := NewVamanaGraph(4, 8, 1.2, "euclidean")

	p := &point.Point{
		ID:     "test-1",
		Vector: point.Vector{1, 0}, // Wrong dimension
	}

	err := g.Insert(p)
	if err != ErrDimensionMismatch {
		t.Errorf("Expected ErrDimensionMismatch, got %v", err)
	}
}

func TestVamanaSearch(t *testing.T) {
	t.Parallel()

	g := NewVamanaGraph(4, 8, 1.2, "euclidean")

	// Insert points
	points := []point.Vector{
		{1, 0, 0, 0},
		{0, 1, 0, 0},
		{0, 0, 1, 0},
		{0.9, 0.1, 0, 0}, // Similar to first
	}

	for i, v := range points {
		p := &point.Point{
			ID:     fmt.Sprintf("p%d", i),
			Vector: v,
		}
		_ = g.Insert(p)
	}

	// Search
	results, err := g.Search(point.Vector{1, 0, 0, 0}, 2)
	if err != nil {
		t.Fatalf("Search() failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

func TestVamanaSearchEmpty(t *testing.T) {
	t.Parallel()

	g := NewVamanaGraph(4, 8, 1.2, "euclidean")

	results, err := g.Search(point.Vector{1, 0, 0, 0}, 10)
	if err != nil {
		t.Fatalf("Search() failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results for empty index, got %d", len(results))
	}
}

func TestVamanaSearchDimensionMismatch(t *testing.T) {
	t.Parallel()

	g := NewVamanaGraph(4, 8, 1.2, "euclidean")

	_, err := g.Search(point.Vector{1, 0}, 10)
	if err != ErrDimensionMismatch {
		t.Errorf("Expected ErrDimensionMismatch, got %v", err)
	}
}

func TestVamanaGet(t *testing.T) {
	t.Parallel()

	g := NewVamanaGraph(4, 8, 1.2, "euclidean")

	original := &point.Point{
		ID:      "test-1",
		Vector:  point.Vector{1, 2, 3, 4},
		Payload: map[string]interface{}{"key": "value"},
	}
	_ = g.Insert(original)

	retrieved, err := g.Get("test-1")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	if retrieved.ID != original.ID {
		t.Errorf("Expected ID=%s, got %s", original.ID, retrieved.ID)
	}
}

func TestVamanaGetNotFound(t *testing.T) {
	t.Parallel()

	g := NewVamanaGraph(4, 8, 1.2, "euclidean")

	_, err := g.Get("nonexistent")
	if err != ErrPointNotFound {
		t.Errorf("Expected ErrPointNotFound, got %v", err)
	}
}

func TestVamanaDelete(t *testing.T) {
	t.Parallel()

	g := NewVamanaGraph(4, 8, 1.2, "euclidean")

	p := &point.Point{
		ID:     "test-1",
		Vector: point.Vector{1, 0, 0, 0},
	}
	_ = g.Insert(p)

	err := g.Delete("test-1")
	if err != nil {
		t.Fatalf("Delete() failed: %v", err)
	}

	// Should not be findable after deletion
	_, err = g.Get("test-1")
	if err != ErrPointNotFound {
		t.Error("Expected point to not be found after deletion")
	}

	// Size should decrease
	if g.Size() != 0 {
		t.Errorf("Expected Size()=0 after deletion, got %d", g.Size())
	}
}

func TestVamanaDeleteNotFound(t *testing.T) {
	t.Parallel()

	g := NewVamanaGraph(4, 8, 1.2, "euclidean")

	err := g.Delete("nonexistent")
	if err != ErrPointNotFound {
		t.Errorf("Expected ErrPointNotFound, got %v", err)
	}
}

func TestVamanaIterate(t *testing.T) {
	t.Parallel()

	g := NewVamanaGraph(4, 8, 1.2, "euclidean")

	for i := 0; i < 5; i++ {
		p := &point.Point{
			ID:     fmt.Sprintf("p%d", i),
			Vector: generateRandomVector(4),
		}
		_ = g.Insert(p)
	}

	count := 0
	err := g.Iterate(func(id string) error {
		count++
		return nil
	})

	if err != nil {
		t.Fatalf("Iterate() failed: %v", err)
	}

	if count != 5 {
		t.Errorf("Expected 5 iterations, got %d", count)
	}
}

func TestVamanaGetAllPoints(t *testing.T) {
	t.Parallel()

	g := NewVamanaGraph(4, 8, 1.2, "euclidean")

	for i := 0; i < 5; i++ {
		p := &point.Point{
			ID:     fmt.Sprintf("p%d", i),
			Vector: generateRandomVector(4),
		}
		_ = g.Insert(p)
	}

	points := g.GetAllPoints()
	if len(points) != 5 {
		t.Errorf("Expected 5 points, got %d", len(points))
	}
}

func TestVamanaNodeID(t *testing.T) {
	t.Parallel()

	g := NewVamanaGraph(4, 8, 1.2, "euclidean")

	p := &point.Point{
		ID:     "test-1",
		Vector: point.Vector{1, 0, 0, 0},
	}
	_ = g.Insert(p)

	nodeID, exists := g.GetNodeID("test-1")
	if !exists {
		t.Error("Expected node ID to exist")
	}

	pointID := g.GetPointID(nodeID)
	if pointID != "test-1" {
		t.Errorf("Expected pointID=test-1, got %s", pointID)
	}
}

func TestVamanaRecommend(t *testing.T) {
	t.Parallel()

	g := NewVamanaGraph(4, 8, 1.2, "euclidean")

	points := []point.Vector{
		{1, 0, 0, 0},
		{0.9, 0.1, 0, 0},
		{0.8, 0.2, 0, 0},
		{0, 1, 0, 0},
		{0, 0, 1, 0},
	}

	for i, v := range points {
		p := &point.Point{
			ID:     fmt.Sprintf("p%d", i),
			Vector: v,
		}
		_ = g.Insert(p)
	}

	results, err := g.Recommend("p0", 2)
	if err != nil {
		t.Fatalf("Recommend() failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected recommendations")
	}

	// Should not include the query point
	for _, r := range results {
		if g.GetPointID(r.ID) == "p0" {
			t.Error("Recommend should not include query point")
		}
	}
}

func TestVamanaSearchWithEf(t *testing.T) {
	t.Parallel()

	g := NewVamanaGraph(4, 8, 1.2, "euclidean")

	for i := 0; i < 20; i++ {
		p := &point.Point{
			ID:     fmt.Sprintf("p%d", i),
			Vector: generateRandomVector(4),
		}
		_ = g.Insert(p)
	}

	results, err := g.SearchWithEf(generateRandomVector(4), 5, 50)
	if err != nil {
		t.Fatalf("SearchWithEf() failed: %v", err)
	}

	if len(results) > 5 {
		t.Errorf("Expected max 5 results, got %d", len(results))
	}
}

func TestVamanaGetStats(t *testing.T) {
	t.Parallel()

	g := NewVamanaGraph(4, 8, 1.2, "euclidean")

	for i := 0; i < 10; i++ {
		p := &point.Point{
			ID:     fmt.Sprintf("p%d", i),
			Vector: generateRandomVector(4),
		}
		_ = g.Insert(p)
	}

	stats := g.GetStats()

	if stats.TotalNodes != 10 {
		t.Errorf("Expected TotalNodes=10, got %d", stats.TotalNodes)
	}
	if stats.Dimension != 4 {
		t.Errorf("Expected Dimension=4, got %d", stats.Dimension)
	}
	if stats.MaxDegree != 8 {
		t.Errorf("Expected MaxDegree=8, got %d", stats.MaxDegree)
	}
}

func TestEuclidean(t *testing.T) {
	t.Parallel()

	a := point.Vector{1, 0, 0, 0}
	b := point.Vector{0, 1, 0, 0}

	dist := Euclidean(a, b)

	// L2 squared distance should be 2 (1^2 + 1^2)
	expected := float32(2.0)
	if dist != expected {
		t.Errorf("Expected dist=%f, got %f", expected, dist)
	}
}

func TestCosine(t *testing.T) {
	t.Parallel()

	a := point.Vector{1, 0, 0, 0}
	b := point.Vector{1, 0, 0, 0}

	dist := Cosine(a, b)

	// Same vectors should have distance 0
	if dist > 0.001 {
		t.Errorf("Expected dist~0 for same vectors, got %f", dist)
	}

	c := point.Vector{0, 1, 0, 0}
	dist = Cosine(a, c)

	// Orthogonal vectors should have distance 1
	if dist < 0.999 {
		t.Errorf("Expected dist~1 for orthogonal vectors, got %f", dist)
	}
}

func TestVamanaMetrics(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		metric    string
		dimension int
	}{
		{"euclidean", 4},
		{"cosine", 4},
		{"l2", 4},
		{"dot", 4},
		{"ip", 4},
	}

	for _, tc := range testCases {
		t.Run(tc.metric, func(t *testing.T) {
			g := NewVamanaGraph(tc.dimension, 8, 1.2, tc.metric)

			p := &point.Point{
				ID:     "test",
				Vector: generateRandomVector(tc.dimension),
			}
			err := g.Insert(p)
			if err != nil {
				t.Errorf("Insert failed with metric %s: %v", tc.metric, err)
			}
		})
	}
}

func BenchmarkVamanaInsert(b *testing.B) {
	g := NewVamanaGraph(128, 32, 1.2, "euclidean")

	points := make([]*point.Point, b.N)
	for i := range points {
		points[i] = &point.Point{
			ID:     fmt.Sprintf("p%d", i),
			Vector: generateRandomVector(128),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Insert(points[i])
	}
}

func BenchmarkVamanaSearch(b *testing.B) {
	g := NewVamanaGraph(128, 32, 1.2, "euclidean")

	// Pre-populate
	for i := 0; i < 1000; i++ {
		p := &point.Point{
			ID:     fmt.Sprintf("p%d", i),
			Vector: generateRandomVector(128),
		}
		g.Insert(p)
	}

	query := generateRandomVector(128)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Search(query, 10)
	}
}
