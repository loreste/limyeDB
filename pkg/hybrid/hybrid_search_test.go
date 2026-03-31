package hybrid

import (
	"testing"

	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/index/hnsw"
	"github.com/limyedb/limyedb/pkg/point"
)

func createTestVectorIndex(t *testing.T, dim int) *hnsw.HNSW {
	cfg := &hnsw.Config{
		M:              16,
		EfConstruction: 100,
		EfSearch:       50,
		MaxElements:    1000,
		Metric:         config.MetricCosine,
		Dimension:      dim,
	}

	idx, err := hnsw.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create HNSW index: %v", err)
	}
	return idx
}

func TestNewHybridSearcher(t *testing.T) {
	t.Parallel()

	vectorIdx := createTestVectorIndex(t, 4)
	textIdx := NewBM25Index(nil)

	// With nil config
	searcher := NewHybridSearcher(vectorIdx, textIdx, nil)
	if searcher == nil {
		t.Fatal("NewHybridSearcher returned nil")
	}

	if searcher.fusionMethod != FusionRRF {
		t.Errorf("Expected default fusion method RRF, got %s", searcher.fusionMethod)
	}
}

func TestNewHybridSearcherWithConfig(t *testing.T) {
	t.Parallel()

	vectorIdx := createTestVectorIndex(t, 4)
	textIdx := NewBM25Index(nil)

	cfg := &HybridConfig{
		FusionMethod: FusionWeighted,
		VectorWeight: 0.8,
		TextWeight:   0.2,
		RRF_K:        30,
	}

	searcher := NewHybridSearcher(vectorIdx, textIdx, cfg)

	if searcher.fusionMethod != FusionWeighted {
		t.Errorf("Expected fusion method Weighted, got %s", searcher.fusionMethod)
	}
	if searcher.vectorWeight != 0.8 {
		t.Errorf("Expected vectorWeight=0.8, got %f", searcher.vectorWeight)
	}
}

func TestHybridSearchVectorOnly(t *testing.T) {
	t.Parallel()

	vectorIdx := createTestVectorIndex(t, 4)
	textIdx := NewBM25Index(nil)

	// Add some vectors
	points := []struct {
		id  string
		vec point.Vector
	}{
		{"p1", point.Vector{1, 0, 0, 0}},
		{"p2", point.Vector{0.9, 0.1, 0, 0}},
		{"p3", point.Vector{0, 1, 0, 0}},
	}

	for _, p := range points {
		pt := point.NewPointWithID(p.id, p.vec, nil)
		if err := vectorIdx.Insert(pt); err != nil {
			t.Fatalf("Failed to insert point: %v", err)
		}
	}

	searcher := NewHybridSearcher(vectorIdx, textIdx, nil)

	params := &HybridSearchParams{
		Vector: point.Vector{1, 0, 0, 0},
		Limit:  10,
	}

	result, err := searcher.Search(params)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(result.Points) == 0 {
		t.Error("Expected results from vector-only search")
	}
}

func TestHybridSearchTextOnly(t *testing.T) {
	t.Parallel()

	vectorIdx := createTestVectorIndex(t, 4)
	textIdx := NewBM25Index(nil)

	// Add some documents
	docs := []*Document{
		{ID: "doc1", Content: "the quick brown fox"},
		{ID: "doc2", Content: "lazy dog sleeps"},
	}

	for _, doc := range docs {
		_ = textIdx.Index(doc)
	}

	searcher := NewHybridSearcher(vectorIdx, textIdx, nil)

	params := &HybridSearchParams{
		Query: "fox",
		Limit: 10,
	}

	result, err := searcher.Search(params)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(result.Points) == 0 {
		t.Error("Expected results from text-only search")
	}
}

func TestHybridSearchCombined(t *testing.T) {
	t.Parallel()

	vectorIdx := createTestVectorIndex(t, 4)
	textIdx := NewBM25Index(nil)

	// Add vectors and text with matching IDs
	points := []struct {
		id      string
		vec     point.Vector
		content string
	}{
		{"item1", point.Vector{1, 0, 0, 0}, "quick brown fox"},
		{"item2", point.Vector{0.9, 0.1, 0, 0}, "lazy dog"},
		{"item3", point.Vector{0, 1, 0, 0}, "fox jumps high"},
	}

	for _, p := range points {
		pt := point.NewPointWithID(p.id, p.vec, nil)
		_ = vectorIdx.Insert(pt)

		doc := &Document{ID: p.id, Content: p.content}
		_ = textIdx.Index(doc)
	}

	searcher := NewHybridSearcher(vectorIdx, textIdx, nil)

	params := &HybridSearchParams{
		Vector: point.Vector{1, 0, 0, 0},
		Query:  "fox",
		Limit:  10,
	}

	result, err := searcher.Search(params)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(result.Points) == 0 {
		t.Error("Expected results from combined search")
	}
}

func TestFusionRRF(t *testing.T) {
	t.Parallel()

	vectorIdx := createTestVectorIndex(t, 4)
	textIdx := NewBM25Index(nil)

	searcher := NewHybridSearcher(vectorIdx, textIdx, &HybridConfig{
		FusionMethod: FusionRRF,
		RRF_K:        60,
	})

	// Add test data
	for i := 0; i < 5; i++ {
		id := string(rune('a' + i))
		vec := make(point.Vector, 4)
		vec[i%4] = 1.0

		pt := point.NewPointWithID(id, vec, nil)
		_ = vectorIdx.Insert(pt)

		doc := &Document{ID: id, Content: "test content " + id}
		_ = textIdx.Index(doc)
	}

	params := &HybridSearchParams{
		Vector: point.Vector{1, 0, 0, 0},
		Query:  "test",
		Limit:  5,
	}

	result, err := searcher.Search(params)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Check that scores are computed
	for _, p := range result.Points {
		if p.Score <= 0 {
			t.Error("Expected positive RRF score")
		}
	}
}

func TestFusionWeighted(t *testing.T) {
	t.Parallel()

	vectorIdx := createTestVectorIndex(t, 4)
	textIdx := NewBM25Index(nil)

	searcher := NewHybridSearcher(vectorIdx, textIdx, &HybridConfig{
		FusionMethod: FusionWeighted,
		VectorWeight: 0.7,
		TextWeight:   0.3,
	})

	// Add test data
	pt := point.NewPointWithID("item1", point.Vector{1, 0, 0, 0}, nil)
	_ = vectorIdx.Insert(pt)

	doc := &Document{ID: "item1", Content: "test content"}
	_ = textIdx.Index(doc)

	params := &HybridSearchParams{
		Vector: point.Vector{1, 0, 0, 0},
		Query:  "test",
		Limit:  5,
	}

	result, err := searcher.Search(params)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(result.Points) > 0 {
		if result.Points[0].VectorScore <= 0 {
			t.Error("Expected positive vector score")
		}
	}
}

func TestFusionConvex(t *testing.T) {
	t.Parallel()

	vectorIdx := createTestVectorIndex(t, 4)
	textIdx := NewBM25Index(nil)

	searcher := NewHybridSearcher(vectorIdx, textIdx, &HybridConfig{
		FusionMethod: FusionConvex,
		VectorWeight: 0.5,
	})

	pt := point.NewPointWithID("item1", point.Vector{1, 0, 0, 0}, nil)
	_ = vectorIdx.Insert(pt)

	doc := &Document{ID: "item1", Content: "test"}
	_ = textIdx.Index(doc)

	params := &HybridSearchParams{
		Vector: point.Vector{1, 0, 0, 0},
		Query:  "test",
		Limit:  5,
	}

	result, err := searcher.Search(params)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(result.Points) == 0 {
		t.Error("Expected results with convex fusion")
	}
}

func TestHybridSearchWithOffset(t *testing.T) {
	t.Parallel()

	vectorIdx := createTestVectorIndex(t, 4)
	textIdx := NewBM25Index(nil)

	// Add several items
	for i := 0; i < 10; i++ {
		id := string(rune('a' + i))
		vec := make(point.Vector, 4)
		vec[0] = float32(10 - i)

		pt := point.NewPointWithID(id, vec, nil)
		_ = vectorIdx.Insert(pt)

		doc := &Document{ID: id, Content: "test content"}
		_ = textIdx.Index(doc)
	}

	searcher := NewHybridSearcher(vectorIdx, textIdx, nil)

	// Get all results
	params := &HybridSearchParams{
		Query: "test",
		Limit: 10,
	}
	fullResult, _ := searcher.Search(params)

	// Get with offset
	params.Offset = 3
	offsetResult, _ := searcher.Search(params)

	if len(offsetResult.Points) >= len(fullResult.Points) {
		t.Error("Offset should reduce result count")
	}
}

func TestHybridSearchWithPayload(t *testing.T) {
	t.Parallel()

	vectorIdx := createTestVectorIndex(t, 4)
	textIdx := NewBM25Index(nil)

	payload := map[string]interface{}{"key": "value"}
	pt := point.NewPointWithID("item1", point.Vector{1, 0, 0, 0}, payload)
	_ = vectorIdx.Insert(pt)

	doc := &Document{ID: "item1", Content: "test"}
	_ = textIdx.Index(doc)

	searcher := NewHybridSearcher(vectorIdx, textIdx, nil)

	params := &HybridSearchParams{
		Vector:      point.Vector{1, 0, 0, 0},
		Limit:       5,
		WithPayload: true,
	}

	result, err := searcher.Search(params)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(result.Points) > 0 {
		if result.Points[0].Payload == nil {
			t.Error("Expected payload in result")
		}
	}
}

func TestHybridSearchWithVector(t *testing.T) {
	t.Parallel()

	vectorIdx := createTestVectorIndex(t, 4)
	textIdx := NewBM25Index(nil)

	vec := point.Vector{1, 0, 0, 0}
	pt := point.NewPointWithID("item1", vec, nil)
	_ = vectorIdx.Insert(pt)

	doc := &Document{ID: "item1", Content: "test"}
	_ = textIdx.Index(doc)

	searcher := NewHybridSearcher(vectorIdx, textIdx, nil)

	params := &HybridSearchParams{
		Vector:     vec,
		Limit:      5,
		WithVector: true,
	}

	result, err := searcher.Search(params)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(result.Points) > 0 {
		if len(result.Points[0].Vector) == 0 {
			t.Error("Expected vector in result")
		}
	}
}

func TestIndexText(t *testing.T) {
	t.Parallel()

	vectorIdx := createTestVectorIndex(t, 4)
	textIdx := NewBM25Index(nil)

	searcher := NewHybridSearcher(vectorIdx, textIdx, nil)

	err := searcher.IndexText("doc1", "main content", map[string]string{"title": "Title"})
	if err != nil {
		t.Fatalf("IndexText failed: %v", err)
	}

	results := textIdx.Search("content", 10)
	if len(results) == 0 {
		t.Error("Expected indexed document to be searchable")
	}
}

func TestRemoveText(t *testing.T) {
	t.Parallel()

	vectorIdx := createTestVectorIndex(t, 4)
	textIdx := NewBM25Index(nil)

	searcher := NewHybridSearcher(vectorIdx, textIdx, nil)

	_ = searcher.IndexText("doc1", "test content", nil)
	err := searcher.RemoveText("doc1")
	if err != nil {
		t.Fatalf("RemoveText failed: %v", err)
	}

	results := textIdx.Search("test", 10)
	if len(results) != 0 {
		t.Error("Expected document to be removed")
	}
}

func TestUpdateWeights(t *testing.T) {
	t.Parallel()

	vectorIdx := createTestVectorIndex(t, 4)
	textIdx := NewBM25Index(nil)

	searcher := NewHybridSearcher(vectorIdx, textIdx, nil)

	searcher.UpdateWeights(0.9, 0.1)

	if searcher.vectorWeight != 0.9 {
		t.Errorf("Expected vectorWeight=0.9, got %f", searcher.vectorWeight)
	}
	if searcher.textWeight != 0.1 {
		t.Errorf("Expected textWeight=0.1, got %f", searcher.textWeight)
	}
}

func TestSetFusionMethod(t *testing.T) {
	t.Parallel()

	vectorIdx := createTestVectorIndex(t, 4)
	textIdx := NewBM25Index(nil)

	searcher := NewHybridSearcher(vectorIdx, textIdx, nil)

	searcher.SetFusionMethod(FusionWeighted)
	if searcher.fusionMethod != FusionWeighted {
		t.Errorf("Expected FusionWeighted, got %s", searcher.fusionMethod)
	}
}

func TestHybridSearchTiming(t *testing.T) {
	t.Parallel()

	vectorIdx := createTestVectorIndex(t, 4)
	textIdx := NewBM25Index(nil)

	// Add some data
	pt := point.NewPointWithID("item1", point.Vector{1, 0, 0, 0}, nil)
	_ = vectorIdx.Insert(pt)
	_ = textIdx.Index(&Document{ID: "item1", Content: "test"})

	searcher := NewHybridSearcher(vectorIdx, textIdx, nil)

	params := &HybridSearchParams{
		Vector: point.Vector{1, 0, 0, 0},
		Query:  "test",
		Limit:  5,
	}

	result, err := searcher.Search(params)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if result.TookMs < 0 {
		t.Error("Expected non-negative TookMs")
	}
}

func TestDefaultHybridConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultHybridConfig()

	if cfg.FusionMethod != FusionRRF {
		t.Errorf("Expected default FusionMethod=RRF, got %s", cfg.FusionMethod)
	}
	if cfg.VectorWeight != 0.7 {
		t.Errorf("Expected default VectorWeight=0.7, got %f", cfg.VectorWeight)
	}
	if cfg.TextWeight != 0.3 {
		t.Errorf("Expected default TextWeight=0.3, got %f", cfg.TextWeight)
	}
	if cfg.RRF_K != 60 {
		t.Errorf("Expected default RRF_K=60, got %d", cfg.RRF_K)
	}
}
