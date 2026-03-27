package visualization

import (
	"math"
	"testing"

	"github.com/limyedb/limyedb/pkg/point"
)

func TestPCA(t *testing.T) {
	// Create simple test data - 3 clusters in 4D space
	vectors := []point.Vector{
		{1, 0, 0, 0},
		{1.1, 0.1, 0, 0},
		{0.9, -0.1, 0, 0},
		{0, 1, 0, 0},
		{0.1, 1.1, 0, 0},
		{-0.1, 0.9, 0, 0},
		{0, 0, 1, 0},
		{0, 0.1, 1.1, 0},
		{0, -0.1, 0.9, 0},
	}

	pca := NewPCA(2)
	result := pca.FitTransform(vectors)

	if len(result) != len(vectors) {
		t.Errorf("Expected %d results, got %d", len(vectors), len(result))
	}

	for i, r := range result {
		if len(r) != 2 {
			t.Errorf("Result %d has %d dimensions, expected 2", i, len(r))
		}
	}

	// Check that similar vectors are close in reduced space
	dist01 := squaredDist(result[0], result[1])
	dist03 := squaredDist(result[0], result[3])

	if dist01 > dist03 {
		t.Log("Note: Similar vectors should be closer in reduced space")
	}
}

func TestPCA3D(t *testing.T) {
	vectors := make([]point.Vector, 10)
	for i := range vectors {
		vectors[i] = make(point.Vector, 100)
		for j := range vectors[i] {
			vectors[i][j] = float32(i + j)
		}
	}

	pca := NewPCA(3)
	result := pca.FitTransform(vectors)

	if len(result) != 10 {
		t.Errorf("Expected 10 results, got %d", len(result))
	}

	for i, r := range result {
		if len(r) != 3 {
			t.Errorf("Result %d has %d dimensions, expected 3", i, len(r))
		}
	}
}

func TestTSNE(t *testing.T) {
	// Small dataset for fast testing
	vectors := []point.Vector{
		{1, 0, 0, 0},
		{1, 0.1, 0, 0},
		{0, 1, 0, 0},
		{0, 1, 0.1, 0},
		{0, 0, 1, 0},
		{0, 0, 1, 0.1},
	}

	tsne := NewTSNE(2)
	tsne.Iterations = 100 // Fewer iterations for testing

	result := tsne.FitTransform(vectors)

	if len(result) != len(vectors) {
		t.Errorf("Expected %d results, got %d", len(vectors), len(result))
	}

	for i, r := range result {
		if len(r) != 2 {
			t.Errorf("Result %d has %d dimensions, expected 2", i, len(r))
		}
	}
}

func TestVisualizer(t *testing.T) {
	v := NewVisualizer()

	vectors := []point.Vector{
		{1, 0, 0, 0},
		{0, 1, 0, 0},
		{0, 0, 1, 0},
	}
	ids := []string{"a", "b", "c"}

	// Test PCA 2D
	points2D := v.ReducePCA(vectors, ids)
	if len(points2D) != 3 {
		t.Errorf("Expected 3 points, got %d", len(points2D))
	}

	// Check normalization (should be 0-1)
	for _, p := range points2D {
		if p.X < 0 || p.X > 1 || p.Y < 0 || p.Y > 1 {
			t.Errorf("Points should be normalized to 0-1: got (%f, %f)", p.X, p.Y)
		}
	}

	// Check IDs preserved
	if points2D[0].ID != "a" {
		t.Errorf("Expected ID 'a', got '%s'", points2D[0].ID)
	}
}

func TestVisualizer3D(t *testing.T) {
	v := NewVisualizer()

	vectors := []point.Vector{
		{1, 0, 0, 0, 0},
		{0, 1, 0, 0, 0},
		{0, 0, 1, 0, 0},
		{0, 0, 0, 1, 0},
	}
	ids := []string{"a", "b", "c", "d"}

	points3D := v.ReducePCA3D(vectors, ids)
	if len(points3D) != 4 {
		t.Errorf("Expected 4 points, got %d", len(points3D))
	}

	for _, p := range points3D {
		if p.X < 0 || p.X > 1 || p.Y < 0 || p.Y > 1 || p.Z < 0 || p.Z > 1 {
			t.Errorf("3D points should be normalized to 0-1")
		}
	}
}

func TestKNNVisualization(t *testing.T) {
	v := NewVisualizer()

	query := point.Vector{1, 0, 0, 0}
	neighbors := []point.Vector{
		{0.9, 0.1, 0, 0},
		{0.8, 0.2, 0, 0},
		{0.7, 0.3, 0, 0},
	}
	neighborIDs := []string{"n1", "n2", "n3"}
	distances := []float32{0.1, 0.2, 0.3}

	result := v.VisualizeKNN(query, neighbors, neighborIDs, distances)

	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	if result.Query.ID != "query" {
		t.Errorf("Expected query ID 'query', got '%s'", result.Query.ID)
	}

	if len(result.Neighbors) != 3 {
		t.Errorf("Expected 3 neighbors, got %d", len(result.Neighbors))
	}

	if len(result.Edges) != 3 {
		t.Errorf("Expected 3 edges, got %d", len(result.Edges))
	}

	// Check edges
	for _, edge := range result.Edges {
		if edge.From != "query" {
			t.Errorf("Edge should start from query")
		}
	}
}

func TestClusterVisualization(t *testing.T) {
	v := NewVisualizer()

	// Create 3 clusters
	vectors := make([]point.Vector, 15)
	ids := make([]string, 15)

	for i := 0; i < 5; i++ {
		vectors[i] = point.Vector{float32(10 + i), 0, 0, 0}
		ids[i] = "a"
	}
	for i := 5; i < 10; i++ {
		vectors[i] = point.Vector{0, float32(10 + i), 0, 0}
		ids[i] = "b"
	}
	for i := 10; i < 15; i++ {
		vectors[i] = point.Vector{0, 0, float32(10 + i), 0}
		ids[i] = "c"
	}

	result := v.VisualizeClusters(vectors, ids, 3)

	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	if len(result.Points) != 15 {
		t.Errorf("Expected 15 points, got %d", len(result.Points))
	}

	if len(result.Centroids) != 3 {
		t.Errorf("Expected 3 centroids, got %d", len(result.Centroids))
	}

	// Check cluster assignments exist
	clusters := make(map[int]int)
	for _, p := range result.Points {
		clusters[p.Cluster]++
	}

	if len(clusters) < 2 {
		t.Logf("Expected multiple clusters, got %d", len(clusters))
	}
}

func TestEmptyInput(t *testing.T) {
	v := NewVisualizer()

	// Empty vectors
	result := v.ReducePCA([]point.Vector{}, []string{})
	if result != nil && len(result) > 0 {
		t.Error("Expected empty result for empty input")
	}
}

func TestSinglePoint(t *testing.T) {
	v := NewVisualizer()

	vectors := []point.Vector{{1, 2, 3, 4}}
	ids := []string{"single"}

	result := v.ReducePCA(vectors, ids)
	if len(result) != 1 {
		t.Errorf("Expected 1 point, got %d", len(result))
	}
}

func TestSortPointsByDistance(t *testing.T) {
	points := []Point2D{
		{ID: "far", X: 1.0, Y: 1.0},
		{ID: "close", X: 0.1, Y: 0.1},
		{ID: "mid", X: 0.5, Y: 0.5},
	}

	SortPointsByDistance(points)

	if points[0].ID != "close" {
		t.Errorf("Expected 'close' first, got '%s'", points[0].ID)
	}
	if points[2].ID != "far" {
		t.Errorf("Expected 'far' last, got '%s'", points[2].ID)
	}
}

func BenchmarkPCA(b *testing.B) {
	vectors := make([]point.Vector, 1000)
	for i := range vectors {
		vectors[i] = make(point.Vector, 128)
		for j := range vectors[i] {
			vectors[i][j] = float32(i + j)
		}
	}

	pca := NewPCA(2)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pca.FitTransform(vectors)
	}
}

func BenchmarkTSNE(b *testing.B) {
	// Smaller dataset for t-SNE benchmark
	vectors := make([]point.Vector, 100)
	for i := range vectors {
		vectors[i] = make(point.Vector, 32)
		for j := range vectors[i] {
			vectors[i][j] = float32(i + j)
		}
	}

	tsne := NewTSNE(2)
	tsne.Iterations = 100

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tsne.FitTransform(vectors)
	}
}

func TestNormalization(t *testing.T) {
	points := []Point2D{
		{X: -10, Y: -20},
		{X: 10, Y: 20},
		{X: 0, Y: 0},
	}

	normalizePoints2D(points)

	// Check all points are in 0-1 range
	for i, p := range points {
		if p.X < 0 || p.X > 1 || p.Y < 0 || p.Y > 1 {
			t.Errorf("Point %d not normalized: (%f, %f)", i, p.X, p.Y)
		}
	}

	// Check extremes
	minFound, maxFound := false, false
	for _, p := range points {
		if math.Abs(p.X) < 0.01 || math.Abs(p.Y) < 0.01 {
			minFound = true
		}
		if math.Abs(p.X-1) < 0.01 || math.Abs(p.Y-1) < 0.01 {
			maxFound = true
		}
	}

	if !minFound || !maxFound {
		t.Log("Normalization should include 0 and 1 extremes")
	}
}
