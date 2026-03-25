package ivf

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/point"
)

func TestNewIVF(t *testing.T) {
	cfg := &Config{
		NumClusters: 10,
		Nprobe:      5,
		Metric:      config.MetricCosine,
		Dimension:   128,
		MaxElements: 1000,
	}

	ivf, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create IVF: %v", err)
	}

	if ivf == nil {
		t.Fatal("IVF is nil")
	}

	if ivf.Size() != 0 {
		t.Errorf("Expected size 0, got %d", ivf.Size())
	}
}

func TestNewIVFInvalidConfig(t *testing.T) {
	// Test with invalid dimension
	cfg := &Config{
		NumClusters: 10,
		Nprobe:      5,
		Metric:      config.MetricCosine,
		Dimension:   0, // Invalid
		MaxElements: 1000,
	}

	_, err := New(cfg)
	if err == nil {
		t.Error("Expected error for invalid dimension")
	}
}

func TestIVFInsertAndGet(t *testing.T) {
	cfg := &Config{
		NumClusters:     10,
		Nprobe:          5,
		Metric:          config.MetricCosine,
		Dimension:       4,
		MaxElements:     1000,
		TrainingSamples: 50, // Low threshold for testing
	}

	ivf, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create IVF: %v", err)
	}

	// Insert a point
	p := &point.Point{
		ID:      "test-1",
		Vector:  []float32{1.0, 0.0, 0.0, 0.0},
		Payload: map[string]interface{}{"name": "test"},
	}

	if err := ivf.Insert(p); err != nil {
		t.Fatalf("Failed to insert point: %v", err)
	}

	if ivf.Size() != 1 {
		t.Errorf("Expected size 1, got %d", ivf.Size())
	}

	// Get the point
	retrieved, err := ivf.Get("test-1")
	if err != nil {
		t.Fatalf("Failed to get point: %v", err)
	}

	if retrieved.ID != p.ID {
		t.Errorf("Expected ID %s, got %s", p.ID, retrieved.ID)
	}
}

func TestIVFDuplicateInsert(t *testing.T) {
	cfg := &Config{
		NumClusters: 10,
		Nprobe:      5,
		Metric:      config.MetricCosine,
		Dimension:   4,
		MaxElements: 1000,
	}

	ivf, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create IVF: %v", err)
	}

	p := &point.Point{
		ID:     "test-1",
		Vector: []float32{1.0, 0.0, 0.0, 0.0},
	}

	if err := ivf.Insert(p); err != nil {
		t.Fatalf("Failed to insert point: %v", err)
	}

	// Try to insert duplicate
	err = ivf.Insert(p)
	if err != ErrPointExists {
		t.Errorf("Expected ErrPointExists, got %v", err)
	}
}

func TestIVFDimensionMismatch(t *testing.T) {
	cfg := &Config{
		NumClusters: 10,
		Nprobe:      5,
		Metric:      config.MetricCosine,
		Dimension:   4,
		MaxElements: 1000,
	}

	ivf, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create IVF: %v", err)
	}

	p := &point.Point{
		ID:     "test-1",
		Vector: []float32{1.0, 0.0, 0.0}, // Wrong dimension
	}

	err = ivf.Insert(p)
	if err != ErrDimensionMismatch {
		t.Errorf("Expected ErrDimensionMismatch, got %v", err)
	}
}

func TestKMeansTrain(t *testing.T) {
	// Generate random vectors
	rng := rand.New(rand.NewSource(42))
	dimension := 8
	numVectors := 100
	numClusters := 5

	vectors := make([]point.Vector, numVectors)
	for i := range vectors {
		vectors[i] = randomVector(rng, dimension)
	}

	cfg := &Config{
		NumClusters:     numClusters,
		Nprobe:          2,
		Metric:          config.MetricCosine,
		Dimension:       dimension,
		MaxElements:     1000,
		TrainingSamples: 50,
	}

	ivf, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create IVF: %v", err)
	}

	// Train the index
	if err := ivf.Train(vectors); err != nil {
		t.Fatalf("Failed to train IVF: %v", err)
	}

	if !ivf.IsTrained() {
		t.Error("Expected IVF to be trained")
	}

	// Check that we have centroids
	centroids := ivf.Centroids()
	if len(centroids) != numClusters {
		t.Errorf("Expected %d centroids, got %d", numClusters, len(centroids))
	}
}

func TestIVFSearchBeforeTraining(t *testing.T) {
	cfg := &Config{
		NumClusters:     10,
		Nprobe:          5,
		Metric:          config.MetricCosine,
		Dimension:       4,
		MaxElements:     1000,
		TrainingSamples: 1000, // High threshold so we don't auto-train
	}

	ivf, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create IVF: %v", err)
	}

	// Insert some points
	for i := 0; i < 10; i++ {
		p := &point.Point{
			ID:     fmt.Sprintf("point-%d", i),
			Vector: []float32{float32(i), float32(i), float32(i), float32(i)},
		}
		if err := ivf.Insert(p); err != nil {
			t.Fatalf("Failed to insert point: %v", err)
		}
	}

	// Search should still work (brute force fallback)
	query := []float32{5.0, 5.0, 5.0, 5.0}
	results, err := ivf.Search(query, 3)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}
}

func TestIVFSearchAfterTraining(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	dimension := 8
	numVectors := 200
	numClusters := 10

	cfg := &Config{
		NumClusters:     numClusters,
		Nprobe:          3,
		Metric:          config.MetricEuclidean,
		Dimension:       dimension,
		MaxElements:     1000,
		TrainingSamples: 50,
	}

	ivf, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create IVF: %v", err)
	}

	// Generate and insert training vectors
	vectors := make([]point.Vector, numVectors)
	for i := range vectors {
		vectors[i] = randomVector(rng, dimension)
		p := &point.Point{
			ID:     fmt.Sprintf("point-%d", i),
			Vector: vectors[i],
		}
		if err := ivf.Insert(p); err != nil && err != ErrPointExists {
			t.Fatalf("Failed to insert point: %v", err)
		}
	}

	// Train the index
	if err := ivf.Train(vectors); err != nil {
		t.Fatalf("Failed to train IVF: %v", err)
	}

	// Search for nearest neighbors
	query := vectors[0] // Use first vector as query
	results, err := ivf.Search(query, 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected at least one result")
	}

	// First result should be the query vector itself (distance ~0)
	if results[0].Distance > 0.01 {
		t.Errorf("Expected first result to have distance ~0, got %f", results[0].Distance)
	}

	// Results should be sorted by distance
	for i := 1; i < len(results); i++ {
		if results[i].Distance < results[i-1].Distance {
			t.Error("Results not sorted by distance")
		}
	}
}

func TestIVFSearchWithFilter(t *testing.T) {
	cfg := &Config{
		NumClusters:     5,
		Nprobe:          3,
		Metric:          config.MetricCosine,
		Dimension:       4,
		MaxElements:     1000,
		TrainingSamples: 20,
	}

	ivf, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create IVF: %v", err)
	}

	// Insert points with different categories
	for i := 0; i < 100; i++ {
		category := "A"
		if i%2 == 0 {
			category = "B"
		}
		p := &point.Point{
			ID:      fmt.Sprintf("point-%d", i),
			Vector:  []float32{float32(i % 10), float32(i % 10), float32(i % 10), float32(i % 10)},
			Payload: map[string]interface{}{"category": category},
		}
		if err := ivf.Insert(p); err != nil {
			t.Fatalf("Failed to insert point: %v", err)
		}
	}

	// Search with filter for category A only
	query := []float32{5.0, 5.0, 5.0, 5.0}
	params := &SearchParams{
		K:      10,
		Nprobe: 5,
		Filter: func(id string, payload map[string]interface{}) bool {
			cat, ok := payload["category"].(string)
			return ok && cat == "A"
		},
	}

	results, err := ivf.SearchWithFilter(query, params)
	if err != nil {
		t.Fatalf("Filtered search failed: %v", err)
	}

	// All results should have category A
	for _, r := range results {
		p, _ := ivf.Get(ivf.GetPointID(r.ID))
		cat, ok := p.Payload["category"].(string)
		if !ok || cat != "A" {
			t.Errorf("Expected category A, got %v", p.Payload["category"])
		}
	}
}

func TestIVFBatchSearch(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	dimension := 8
	numVectors := 100

	cfg := &Config{
		NumClusters:     10,
		Nprobe:          3,
		Metric:          config.MetricEuclidean,
		Dimension:       dimension,
		MaxElements:     1000,
		TrainingSamples: 50,
	}

	ivf, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create IVF: %v", err)
	}

	// Insert vectors
	vectors := make([]point.Vector, numVectors)
	for i := range vectors {
		vectors[i] = randomVector(rng, dimension)
		p := &point.Point{
			ID:     fmt.Sprintf("point-%d", i),
			Vector: vectors[i],
		}
		ivf.Insert(p)
	}

	// Train
	ivf.Train(vectors)

	// Batch search
	queries := []point.Vector{
		vectors[0],
		vectors[10],
		vectors[20],
	}

	results, err := ivf.BatchSearch(queries, 5)
	if err != nil {
		t.Fatalf("Batch search failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 result sets, got %d", len(results))
	}

	for i, r := range results {
		if len(r) == 0 {
			t.Errorf("Query %d returned no results", i)
		}
	}
}

func TestIVFRangeSearch(t *testing.T) {
	cfg := &Config{
		NumClusters:     5,
		Nprobe:          5,
		Metric:          config.MetricEuclidean,
		Dimension:       4,
		MaxElements:     1000,
		TrainingSamples: 20,
	}

	ivf, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create IVF: %v", err)
	}

	// Insert points in a grid pattern
	for i := 0; i < 10; i++ {
		for j := 0; j < 10; j++ {
			p := &point.Point{
				ID:     fmt.Sprintf("point-%d-%d", i, j),
				Vector: []float32{float32(i), float32(j), 0, 0},
			}
			ivf.Insert(p)
		}
	}

	// Train
	vectors := make([]point.Vector, 100)
	for i := 0; i < 10; i++ {
		for j := 0; j < 10; j++ {
			vectors[i*10+j] = []float32{float32(i), float32(j), 0, 0}
		}
	}
	ivf.Train(vectors)

	// Range search with radius 2.0
	query := []float32{5.0, 5.0, 0.0, 0.0}
	results, err := ivf.RangeSearch(query, 2.0)
	if err != nil {
		t.Fatalf("Range search failed: %v", err)
	}

	// Should find points within radius
	for _, r := range results {
		if r.Distance > 2.0 {
			t.Errorf("Found point outside radius: distance=%f", r.Distance)
		}
	}
}

func TestIVFRecommend(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	dimension := 8

	cfg := &Config{
		NumClusters:     5,
		Nprobe:          3,
		Metric:          config.MetricEuclidean,
		Dimension:       dimension,
		MaxElements:     1000,
		TrainingSamples: 50,
	}

	ivf, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create IVF: %v", err)
	}

	// Insert vectors
	vectors := make([]point.Vector, 100)
	for i := range vectors {
		vectors[i] = randomVector(rng, dimension)
		p := &point.Point{
			ID:     fmt.Sprintf("point-%d", i),
			Vector: vectors[i],
		}
		ivf.Insert(p)
	}

	// Train
	ivf.Train(vectors)

	// Get recommendations for point-0
	results, err := ivf.Recommend("point-0", 5)
	if err != nil {
		t.Fatalf("Recommend failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected at least one recommendation")
	}

	// Verify that point-0 is not in the results
	for _, r := range results {
		if ivf.GetPointID(r.ID) == "point-0" {
			t.Error("Recommendation should not include the query point itself")
		}
	}
}

func TestClusterSizes(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	dimension := 8
	numVectors := 200
	numClusters := 10

	cfg := &Config{
		NumClusters:     numClusters,
		Nprobe:          3,
		Metric:          config.MetricEuclidean,
		Dimension:       dimension,
		MaxElements:     1000,
		TrainingSamples: 50,
	}

	ivf, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create IVF: %v", err)
	}

	// Insert vectors
	vectors := make([]point.Vector, numVectors)
	for i := range vectors {
		vectors[i] = randomVector(rng, dimension)
		p := &point.Point{
			ID:     fmt.Sprintf("point-%d", i),
			Vector: vectors[i],
		}
		ivf.Insert(p)
	}

	// Train
	ivf.Train(vectors)

	// Check cluster sizes
	sizes := ivf.ClusterSizes()

	totalSize := 0
	for _, size := range sizes {
		totalSize += size
	}

	if totalSize != numVectors {
		t.Errorf("Expected total cluster size %d, got %d", numVectors, totalSize)
	}
}

func BenchmarkIVFInsert(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	dimension := 128

	cfg := &Config{
		NumClusters:     100,
		Nprobe:          10,
		Metric:          config.MetricCosine,
		Dimension:       dimension,
		MaxElements:     b.N + 1000,
		TrainingSamples: 1000,
	}

	ivf, _ := New(cfg)

	// Pre-train with some vectors
	trainVectors := make([]point.Vector, 1000)
	for i := range trainVectors {
		trainVectors[i] = randomVector(rng, dimension)
	}
	ivf.Train(trainVectors)

	// Prepare vectors for insertion
	vectors := make([]point.Vector, b.N)
	for i := range vectors {
		vectors[i] = randomVector(rng, dimension)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := &point.Point{
			ID:     fmt.Sprintf("point-%d", i),
			Vector: vectors[i],
		}
		ivf.Insert(p)
	}
}

func BenchmarkIVFSearch(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	dimension := 128
	numVectors := 10000

	cfg := &Config{
		NumClusters:     100,
		Nprobe:          10,
		Metric:          config.MetricCosine,
		Dimension:       dimension,
		MaxElements:     numVectors,
		TrainingSamples: numVectors,
	}

	ivf, _ := New(cfg)

	// Insert vectors
	vectors := make([]point.Vector, numVectors)
	for i := range vectors {
		vectors[i] = randomVector(rng, dimension)
		p := &point.Point{
			ID:     fmt.Sprintf("point-%d", i),
			Vector: vectors[i],
		}
		ivf.Insert(p)
	}

	// Train
	ivf.Train(vectors)

	// Prepare queries
	queries := make([]point.Vector, b.N)
	for i := range queries {
		queries[i] = randomVector(rng, dimension)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ivf.Search(queries[i], 10)
	}
}

func BenchmarkIVFSearchNprobe(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	dimension := 128
	numVectors := 10000

	nprobeValues := []int{1, 5, 10, 20, 50}

	for _, nprobe := range nprobeValues {
		b.Run(fmt.Sprintf("nprobe=%d", nprobe), func(b *testing.B) {
			cfg := &Config{
				NumClusters:     100,
				Nprobe:          nprobe,
				Metric:          config.MetricCosine,
				Dimension:       dimension,
				MaxElements:     numVectors,
				TrainingSamples: numVectors,
			}

			ivf, _ := New(cfg)

			// Insert and train
			vectors := make([]point.Vector, numVectors)
			for i := range vectors {
				vectors[i] = randomVector(rng, dimension)
				p := &point.Point{
					ID:     fmt.Sprintf("point-%d", i),
					Vector: vectors[i],
				}
				ivf.Insert(p)
			}
			ivf.Train(vectors)

			// Prepare queries
			queries := make([]point.Vector, b.N)
			for i := range queries {
				queries[i] = randomVector(rng, dimension)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ivf.Search(queries[i], 10)
			}
		})
	}
}

// Helper function to generate random unit vectors
func randomVector(rng *rand.Rand, dimension int) point.Vector {
	vec := make(point.Vector, dimension)
	var norm float32
	for i := range vec {
		vec[i] = rng.Float32()*2 - 1 // [-1, 1]
		norm += vec[i] * vec[i]
	}
	// Normalize
	norm = float32(math.Sqrt(float64(norm)))
	if norm > 0 {
		for i := range vec {
			vec[i] /= norm
		}
	}
	return vec
}
