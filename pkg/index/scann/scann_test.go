package scann

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/point"
)

func TestNewScaNN(t *testing.T) {
	cfg := &Config{
		NumLeaves:   10,
		NumRerank:   50,
		Metric:      config.MetricCosine,
		Dimension:   128,
		MaxElements: 1000,
	}

	scann, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create ScaNN: %v", err)
	}

	if scann == nil {
		t.Fatal("ScaNN is nil")
	}

	if scann.Size() != 0 {
		t.Errorf("Expected size 0, got %d", scann.Size())
	}
}

func TestNewScaNNInvalidConfig(t *testing.T) {
	cfg := &Config{
		NumLeaves:   10,
		NumRerank:   50,
		Metric:      config.MetricCosine,
		Dimension:   0, // Invalid
		MaxElements: 1000,
	}

	_, err := New(cfg)
	if err == nil {
		t.Error("Expected error for invalid dimension")
	}
}

func TestScaNNInsertAndGet(t *testing.T) {
	cfg := &Config{
		NumLeaves:       10,
		NumRerank:       50,
		Metric:          config.MetricCosine,
		Dimension:       4,
		MaxElements:     1000,
		TrainingSamples: 50,
	}

	scann, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create ScaNN: %v", err)
	}

	// Insert a point
	p := &point.Point{
		ID:      "test-1",
		Vector:  []float32{1.0, 0.0, 0.0, 0.0},
		Payload: map[string]interface{}{"name": "test"},
	}

	if err := scann.Insert(p); err != nil {
		t.Fatalf("Failed to insert point: %v", err)
	}

	if scann.Size() != 1 {
		t.Errorf("Expected size 1, got %d", scann.Size())
	}

	// Get the point
	retrieved, err := scann.Get("test-1")
	if err != nil {
		t.Fatalf("Failed to get point: %v", err)
	}

	if retrieved.ID != p.ID {
		t.Errorf("Expected ID %s, got %s", p.ID, retrieved.ID)
	}
}

func TestScaNNDuplicateInsert(t *testing.T) {
	cfg := &Config{
		NumLeaves:   10,
		NumRerank:   50,
		Metric:      config.MetricCosine,
		Dimension:   4,
		MaxElements: 1000,
	}

	scann, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create ScaNN: %v", err)
	}

	p := &point.Point{
		ID:     "test-1",
		Vector: []float32{1.0, 0.0, 0.0, 0.0},
	}

	if err := scann.Insert(p); err != nil {
		t.Fatalf("Failed to insert point: %v", err)
	}

	// Try to insert duplicate
	err = scann.Insert(p)
	if err != ErrPointExists {
		t.Errorf("Expected ErrPointExists, got %v", err)
	}
}

func TestScaNNDimensionMismatch(t *testing.T) {
	cfg := &Config{
		NumLeaves:   10,
		NumRerank:   50,
		Metric:      config.MetricCosine,
		Dimension:   4,
		MaxElements: 1000,
	}

	scann, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create ScaNN: %v", err)
	}

	p := &point.Point{
		ID:     "test-1",
		Vector: []float32{1.0, 0.0, 0.0}, // Wrong dimension
	}

	err = scann.Insert(p)
	if err != ErrDimensionMismatch {
		t.Errorf("Expected ErrDimensionMismatch, got %v", err)
	}
}

func TestAnisotropicQuantizerTrain(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	dimension := 16
	numVectors := 100

	vectors := make([]point.Vector, numVectors)
	for i := range vectors {
		vectors[i] = randomVector(rng, dimension)
	}

	cfg := DefaultAnisotropicConfig(dimension)
	aq := NewAnisotropicQuantizer(cfg)

	if err := aq.Train(vectors); err != nil {
		t.Fatalf("Failed to train quantizer: %v", err)
	}

	if !aq.IsTrained() {
		t.Error("Expected quantizer to be trained")
	}
}

func TestAnisotropicQuantizerEncodeDecode(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	dimension := 16
	numVectors := 100

	vectors := make([]point.Vector, numVectors)
	for i := range vectors {
		vectors[i] = randomVector(rng, dimension)
	}

	cfg := DefaultAnisotropicConfig(dimension)
	aq := NewAnisotropicQuantizer(cfg)

	if err := aq.Train(vectors); err != nil {
		t.Fatalf("Failed to train quantizer: %v", err)
	}

	// Test encode/decode
	testVec := vectors[0]
	encoded, err := aq.Encode(testVec)
	if err != nil {
		t.Fatalf("Failed to encode vector: %v", err)
	}

	decoded, err := aq.Decode(encoded)
	if err != nil {
		t.Fatalf("Failed to decode vector: %v", err)
	}

	// Decoded should be an approximation (not exact)
	if len(decoded) == 0 {
		t.Error("Decoded vector is empty")
	}
}

func TestScaNNSearchBeforeTraining(t *testing.T) {
	cfg := &Config{
		NumLeaves:       10,
		NumRerank:       50,
		Metric:          config.MetricCosine,
		Dimension:       4,
		MaxElements:     1000,
		TrainingSamples: 1000, // High threshold
	}

	scann, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create ScaNN: %v", err)
	}

	// Insert points
	for i := 0; i < 10; i++ {
		p := &point.Point{
			ID:     fmt.Sprintf("point-%d", i),
			Vector: []float32{float32(i), float32(i), float32(i), float32(i)},
		}
		scann.Insert(p)
	}

	// Search should work (brute force fallback)
	query := []float32{5.0, 5.0, 5.0, 5.0}
	results, err := scann.Search(query, 3)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}
}

func TestScaNNSearchAfterTraining(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	dimension := 16
	numVectors := 200
	numLeaves := 10

	cfg := &Config{
		NumLeaves:       numLeaves,
		NumRerank:       50,
		Metric:          config.MetricEuclidean,
		Dimension:       dimension,
		MaxElements:     1000,
		TrainingSamples: 50,
	}

	scann, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create ScaNN: %v", err)
	}

	// Insert vectors
	vectors := make([]point.Vector, numVectors)
	for i := range vectors {
		vectors[i] = randomVector(rng, dimension)
		p := &point.Point{
			ID:     fmt.Sprintf("point-%d", i),
			Vector: vectors[i],
		}
		scann.Insert(p)
	}

	// Train
	if err := scann.Train(vectors); err != nil {
		t.Fatalf("Failed to train ScaNN: %v", err)
	}

	// Search
	query := vectors[0]
	results, err := scann.Search(query, 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected at least one result")
	}

	// First result should be query itself (or very close)
	if results[0].Distance > 0.01 {
		t.Errorf("Expected first result to have distance ~0, got %f", results[0].Distance)
	}

	// Results should be sorted
	for i := 1; i < len(results); i++ {
		if results[i].Distance < results[i-1].Distance {
			t.Error("Results not sorted by distance")
		}
	}
}

func TestScaNNSearchWithFilter(t *testing.T) {
	cfg := &Config{
		NumLeaves:       5,
		NumRerank:       50,
		Metric:          config.MetricCosine,
		Dimension:       4,
		MaxElements:     1000,
		TrainingSamples: 20,
	}

	scann, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create ScaNN: %v", err)
	}

	// Insert points with categories
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
		scann.Insert(p)
	}

	// Search with filter
	query := []float32{5.0, 5.0, 5.0, 5.0}
	params := &SearchParams{
		K:         10,
		NumLeaves: 5,
		NumRerank: 50,
		Filter: func(id string, payload map[string]interface{}) bool {
			cat, ok := payload["category"].(string)
			return ok && cat == "A"
		},
	}

	results, err := scann.SearchWithFilter(query, params)
	if err != nil {
		t.Fatalf("Filtered search failed: %v", err)
	}

	// All results should have category A
	for _, r := range results {
		p, _ := scann.Get(scann.GetPointID(r.ID))
		cat, ok := p.Payload["category"].(string)
		if !ok || cat != "A" {
			t.Errorf("Expected category A, got %v", p.Payload["category"])
		}
	}
}

func TestScaNNBatchSearch(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	dimension := 16
	numVectors := 100

	cfg := &Config{
		NumLeaves:       10,
		NumRerank:       50,
		Metric:          config.MetricEuclidean,
		Dimension:       dimension,
		MaxElements:     1000,
		TrainingSamples: 50,
	}

	scann, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create ScaNN: %v", err)
	}

	// Insert vectors
	vectors := make([]point.Vector, numVectors)
	for i := range vectors {
		vectors[i] = randomVector(rng, dimension)
		p := &point.Point{
			ID:     fmt.Sprintf("point-%d", i),
			Vector: vectors[i],
		}
		scann.Insert(p)
	}

	// Train
	scann.Train(vectors)

	// Batch search
	queries := []point.Vector{
		vectors[0],
		vectors[10],
		vectors[20],
	}

	results, err := scann.BatchSearch(queries, 5)
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

func TestScaNNRangeSearch(t *testing.T) {
	cfg := &Config{
		NumLeaves:       5,
		NumRerank:       50,
		Metric:          config.MetricEuclidean,
		Dimension:       4,
		MaxElements:     1000,
		TrainingSamples: 20,
	}

	scann, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create ScaNN: %v", err)
	}

	// Insert points in a grid
	for i := 0; i < 10; i++ {
		for j := 0; j < 10; j++ {
			p := &point.Point{
				ID:     fmt.Sprintf("point-%d-%d", i, j),
				Vector: []float32{float32(i), float32(j), 0, 0},
			}
			scann.Insert(p)
		}
	}

	// Train
	vectors := make([]point.Vector, 100)
	for i := 0; i < 10; i++ {
		for j := 0; j < 10; j++ {
			vectors[i*10+j] = []float32{float32(i), float32(j), 0, 0}
		}
	}
	scann.Train(vectors)

	// Range search
	query := []float32{5.0, 5.0, 0.0, 0.0}
	results, err := scann.RangeSearch(query, 2.0)
	if err != nil {
		t.Fatalf("Range search failed: %v", err)
	}

	// All results should be within radius
	for _, r := range results {
		if r.Distance > 2.0 {
			t.Errorf("Found point outside radius: distance=%f", r.Distance)
		}
	}
}

func TestScaNNRecommend(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	dimension := 8

	cfg := &Config{
		NumLeaves:       5,
		NumRerank:       50,
		Metric:          config.MetricEuclidean,
		Dimension:       dimension,
		MaxElements:     1000,
		TrainingSamples: 50,
	}

	scann, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create ScaNN: %v", err)
	}

	// Insert vectors
	vectors := make([]point.Vector, 100)
	for i := range vectors {
		vectors[i] = randomVector(rng, dimension)
		p := &point.Point{
			ID:     fmt.Sprintf("point-%d", i),
			Vector: vectors[i],
		}
		scann.Insert(p)
	}

	// Train
	scann.Train(vectors)

	// Recommend
	results, err := scann.Recommend("point-0", 5)
	if err != nil {
		t.Fatalf("Recommend failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected at least one recommendation")
	}

	// Verify point-0 is not in results
	for _, r := range results {
		if scann.GetPointID(r.ID) == "point-0" {
			t.Error("Recommendation should not include query point")
		}
	}
}

func TestTreePartitioner(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	dimension := 8
	numVectors := 100
	numLeaves := 10

	vectors := make([]point.Vector, numVectors)
	for i := range vectors {
		vectors[i] = randomVector(rng, dimension)
	}

	tp := NewTreePartitioner(dimension, numLeaves, nil)
	if err := tp.Train(vectors); err != nil {
		t.Fatalf("Failed to train partitioner: %v", err)
	}

	// Check number of leaves
	actualLeaves := tp.NumLeaves()
	if actualLeaves == 0 {
		t.Error("Expected at least one leaf")
	}

	// Test finding nearest leaves
	nearest := tp.FindNearestLeaves(vectors[0], 3)
	if len(nearest) == 0 {
		t.Error("Expected at least one nearest leaf")
	}
}

func TestPrecomputedDistanceTable(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	dimension := 16
	numVectors := 100

	vectors := make([]point.Vector, numVectors)
	for i := range vectors {
		vectors[i] = randomVector(rng, dimension)
	}

	cfg := DefaultAnisotropicConfig(dimension)
	aq := NewAnisotropicQuantizer(cfg)
	aq.Train(vectors)

	// Precompute distances
	query := vectors[0]
	table := aq.PrecomputeDistances(query)

	if table == nil {
		t.Fatal("Distance table is nil")
	}

	// Encode a vector and lookup distance
	encoded, _ := aq.Encode(vectors[1])
	dist1 := table.LookupDistance(encoded)
	dist2 := aq.AsymmetricDistance(query, encoded)

	// Distances should match
	if math.Abs(float64(dist1-dist2)) > 1e-5 {
		t.Errorf("Distance mismatch: table=%f, direct=%f", dist1, dist2)
	}
}

func BenchmarkScaNNInsert(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	dimension := 128

	cfg := &Config{
		NumLeaves:       100,
		NumRerank:       100,
		Metric:          config.MetricCosine,
		Dimension:       dimension,
		MaxElements:     b.N + 1000,
		TrainingSamples: 1000,
	}

	scann, _ := New(cfg)

	// Pre-train
	trainVectors := make([]point.Vector, 1000)
	for i := range trainVectors {
		trainVectors[i] = randomVector(rng, dimension)
	}
	scann.Train(trainVectors)

	// Prepare vectors
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
		scann.Insert(p)
	}
}

func BenchmarkScaNNSearch(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	dimension := 128
	numVectors := 10000

	cfg := &Config{
		NumLeaves:       100,
		NumRerank:       100,
		Metric:          config.MetricCosine,
		Dimension:       dimension,
		MaxElements:     numVectors,
		TrainingSamples: numVectors,
	}

	scann, _ := New(cfg)

	// Insert and train
	vectors := make([]point.Vector, numVectors)
	for i := range vectors {
		vectors[i] = randomVector(rng, dimension)
		p := &point.Point{
			ID:     fmt.Sprintf("point-%d", i),
			Vector: vectors[i],
		}
		scann.Insert(p)
	}
	scann.Train(vectors)

	// Prepare queries
	queries := make([]point.Vector, b.N)
	for i := range queries {
		queries[i] = randomVector(rng, dimension)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scann.Search(queries[i], 10)
	}
}

func BenchmarkScaNNSearchNumRerank(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	dimension := 128
	numVectors := 10000

	rerankValues := []int{10, 50, 100, 200}

	for _, numRerank := range rerankValues {
		b.Run(fmt.Sprintf("rerank=%d", numRerank), func(b *testing.B) {
			cfg := &Config{
				NumLeaves:       100,
				NumRerank:       numRerank,
				Metric:          config.MetricCosine,
				Dimension:       dimension,
				MaxElements:     numVectors,
				TrainingSamples: numVectors,
			}

			scann, _ := New(cfg)

			// Insert and train
			vectors := make([]point.Vector, numVectors)
			for i := range vectors {
				vectors[i] = randomVector(rng, dimension)
				p := &point.Point{
					ID:     fmt.Sprintf("point-%d", i),
					Vector: vectors[i],
				}
				scann.Insert(p)
			}
			scann.Train(vectors)

			// Prepare queries
			queries := make([]point.Vector, b.N)
			for i := range queries {
				queries[i] = randomVector(rng, dimension)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				scann.Search(queries[i], 10)
			}
		})
	}
}

// Helper function
func randomVector(rng *rand.Rand, dimension int) point.Vector {
	vec := make(point.Vector, dimension)
	var norm float32
	for i := range vec {
		vec[i] = rng.Float32()*2 - 1
		norm += vec[i] * vec[i]
	}
	norm = float32(math.Sqrt(float64(norm)))
	if norm > 0 {
		for i := range vec {
			vec[i] /= norm
		}
	}
	return vec
}
