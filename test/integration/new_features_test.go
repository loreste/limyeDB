package integration

import (
	"testing"

	"github.com/limyedb/limyedb/pkg/collection"
	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/point"
)

func TestNamedVectors(t *testing.T) {
	// Create a collection with named vectors
	cfg := &config.CollectionConfig{
		Name: "named_vectors_test",
		Vectors: map[string]config.VectorConfig{
			"image": {
				Dimension: 4,
				Metric:    config.MetricCosine,
				HNSW: config.HNSWConfig{
					M:              16,
					EfConstruction: 100,
					EfSearch:       50,
					MaxElements:    1000,
				},
			},
			"text": {
				Dimension: 8,
				Metric:    config.MetricCosine,
				HNSW: config.HNSWConfig{
					M:              16,
					EfConstruction: 100,
					EfSearch:       50,
					MaxElements:    1000,
				},
			},
		},
	}

	coll, err := collection.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create collection: %v", err)
	}

	// Verify named vectors are configured
	if !coll.HasNamedVectors() {
		t.Error("Expected collection to have named vectors")
	}

	names := coll.VectorNames()
	if len(names) != 2 {
		t.Errorf("Expected 2 vector names, got %d", len(names))
	}

	// Insert a point with named vectors
	p := &point.PointV2{
		ID: "p1",
		Vectors: point.NamedVectors{
			"image": []float32{0.1, 0.2, 0.3, 0.4},
			"text":  []float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8},
		},
		Payload: map[string]interface{}{
			"category": "test",
		},
	}

	if err := coll.InsertV2(p); err != nil {
		t.Fatalf("Failed to insert point: %v", err)
	}

	// Search using image vector
	result, err := coll.SearchV2([]float32{0.1, 0.2, 0.3, 0.4}, "image", 10)
	if err != nil {
		t.Fatalf("Failed to search: %v", err)
	}

	if len(result.Points) != 1 {
		t.Errorf("Expected 1 result, got %d", len(result.Points))
	}

	// Search using text vector
	result, err = coll.SearchV2([]float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8}, "text", 10)
	if err != nil {
		t.Fatalf("Failed to search text vector: %v", err)
	}

	if len(result.Points) != 1 {
		t.Errorf("Expected 1 result for text search, got %d", len(result.Points))
	}
}

func TestScrollPagination(t *testing.T) {
	cfg := &config.CollectionConfig{
		Name:      "scroll_test",
		Dimension: 4,
		Metric:    config.MetricCosine,
		HNSW: config.HNSWConfig{
			M:              16,
			EfConstruction: 100,
			EfSearch:       50,
			MaxElements:    1000,
		},
	}

	coll, err := collection.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create collection: %v", err)
	}

	// Insert multiple points
	for i := 0; i < 25; i++ {
		p := point.NewPointWithID(
			string(rune('a'+i)),
			[]float32{float32(i) * 0.1, float32(i) * 0.2, float32(i) * 0.3, float32(i) * 0.4},
			map[string]interface{}{"index": i},
		)
		if err := coll.Insert(p); err != nil {
			t.Fatalf("Failed to insert point %d: %v", i, err)
		}
	}

	// Test scroll with limit
	result, err := coll.Scroll(&collection.ScrollParams{
		Limit:       10,
		WithPayload: true,
	})
	if err != nil {
		t.Fatalf("Failed to scroll: %v", err)
	}

	if len(result.Points) != 10 {
		t.Errorf("Expected 10 results in first page, got %d", len(result.Points))
	}

	if result.NextOffset == "" {
		t.Error("Expected next_offset to be set")
	}

	// Test second page
	result2, err := coll.Scroll(&collection.ScrollParams{
		Offset: result.NextOffset,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("Failed to scroll second page: %v", err)
	}

	if len(result2.Points) != 10 {
		t.Errorf("Expected 10 results in second page, got %d", len(result2.Points))
	}
}

func TestFacetedSearch(t *testing.T) {
	cfg := &config.CollectionConfig{
		Name:      "facet_test",
		Dimension: 4,
		Metric:    config.MetricCosine,
		HNSW: config.HNSWConfig{
			M:              16,
			EfConstruction: 100,
			EfSearch:       50,
			MaxElements:    1000,
		},
	}

	coll, err := collection.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create collection: %v", err)
	}

	// Insert points with categories
	categories := []string{"electronics", "electronics", "clothing", "clothing", "clothing", "food"}
	for i, cat := range categories {
		p := point.NewPointWithID(
			string(rune('a'+i)),
			[]float32{float32(i) * 0.1, float32(i) * 0.2, float32(i) * 0.3, float32(i) * 0.4},
			map[string]interface{}{"category": cat},
		)
		if err := coll.Insert(p); err != nil {
			t.Fatalf("Failed to insert point: %v", err)
		}
	}

	// Test facet
	result, err := coll.Facet(&collection.FacetParams{
		Field: "category",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Failed to get facets: %v", err)
	}

	if len(result.Values) != 3 {
		t.Errorf("Expected 3 facet values, got %d", len(result.Values))
	}

	// Verify counts
	countMap := make(map[string]int64)
	for _, v := range result.Values {
		if s, ok := v.Value.(string); ok {
			countMap[s] = v.Count
		}
	}

	if countMap["clothing"] != 3 {
		t.Errorf("Expected clothing count 3, got %d", countMap["clothing"])
	}
	if countMap["electronics"] != 2 {
		t.Errorf("Expected electronics count 2, got %d", countMap["electronics"])
	}
	if countMap["food"] != 1 {
		t.Errorf("Expected food count 1, got %d", countMap["food"])
	}
}

func TestDiscoverySearch(t *testing.T) {
	cfg := &config.CollectionConfig{
		Name:      "discovery_test",
		Dimension: 4,
		Metric:    config.MetricCosine,
		HNSW: config.HNSWConfig{
			M:              16,
			EfConstruction: 100,
			EfSearch:       50,
			MaxElements:    1000,
		},
	}

	coll, err := collection.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create collection: %v", err)
	}

	// Insert points
	points := []struct {
		id      string
		vector  []float32
		payload map[string]interface{}
	}{
		{"p1", []float32{1.0, 0.0, 0.0, 0.0}, map[string]interface{}{"type": "positive"}},
		{"p2", []float32{0.9, 0.1, 0.0, 0.0}, map[string]interface{}{"type": "positive"}},
		{"p3", []float32{0.0, 1.0, 0.0, 0.0}, map[string]interface{}{"type": "negative"}},
		{"p4", []float32{0.8, 0.2, 0.0, 0.0}, map[string]interface{}{"type": "positive"}},
		{"p5", []float32{0.0, 0.0, 1.0, 0.0}, map[string]interface{}{"type": "neutral"}},
	}

	for _, p := range points {
		pt := point.NewPointWithID(p.id, p.vector, p.payload)
		if err := coll.Insert(pt); err != nil {
			t.Fatalf("Failed to insert point: %v", err)
		}
	}

	// Test discovery with positive example
	result, err := coll.Discover(&collection.DiscoveryParams{
		Target: []float32{0.95, 0.05, 0.0, 0.0},
		Context: &collection.DiscoveryContext{
			Positive: []collection.ContextExample{
				{ID: "p1"},
			},
			Negative: []collection.ContextExample{
				{ID: "p3"},
			},
		},
		K:           5,
		WithPayload: true,
	})

	if err != nil {
		t.Fatalf("Failed discovery search: %v", err)
	}

	if len(result.Points) == 0 {
		t.Error("Expected some results from discovery search")
	}

	// The top results should be similar to p1 (positive) and dissimilar to p3 (negative)
	// p1, p2, p4 should rank higher than p3, p5
}

func TestQueryExplain(t *testing.T) {
	cfg := &config.CollectionConfig{
		Name:      "explain_test",
		Dimension: 4,
		Metric:    config.MetricCosine,
		HNSW: config.HNSWConfig{
			M:              16,
			EfConstruction: 100,
			EfSearch:       50,
			MaxElements:    1000,
		},
	}

	coll, err := collection.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create collection: %v", err)
	}

	// Insert some points
	for i := 0; i < 10; i++ {
		p := point.NewPointWithID(
			string(rune('a'+i)),
			[]float32{float32(i) * 0.1, float32(i) * 0.2, float32(i) * 0.3, float32(i) * 0.4},
			nil,
		)
		if err := coll.Insert(p); err != nil {
			t.Fatalf("Failed to insert point: %v", err)
		}
	}

	// Test explain
	plan, err := coll.Explain(&collection.ExplainParams{
		Query: []float32{0.5, 0.5, 0.5, 0.5},
		K:     5,
	})
	if err != nil {
		t.Fatalf("Failed to explain: %v", err)
	}

	if plan.QueryType != "vector_search" {
		t.Errorf("Expected query type 'vector_search', got '%s'", plan.QueryType)
	}

	if plan.IndexInfo == nil {
		t.Error("Expected index info to be present")
	}

	if plan.EstimatedCost == nil {
		t.Error("Expected estimated cost to be present")
	}

	// Test explain with analyze
	plan2, err := coll.Explain(&collection.ExplainParams{
		Query:   []float32{0.5, 0.5, 0.5, 0.5},
		K:       5,
		Analyze: true,
	})
	if err != nil {
		t.Fatalf("Failed to explain with analyze: %v", err)
	}

	if plan2.ExecutionStats == nil {
		t.Error("Expected execution stats when analyze=true")
	}
}

func TestCollectionAliases(t *testing.T) {
	// Create alias manager
	am, err := collection.NewAliasManager("/tmp/limyedb_test_aliases")
	if err != nil {
		t.Fatalf("Failed to create alias manager: %v", err)
	}

	// Create alias
	if err := am.Create("prod", "collection_v1"); err != nil {
		t.Fatalf("Failed to create alias: %v", err)
	}

	// Resolve alias
	resolved := am.Resolve("prod")
	if resolved != "collection_v1" {
		t.Errorf("Expected resolved alias to be 'collection_v1', got '%s'", resolved)
	}

	// Switch alias (blue-green deployment)
	if err := am.Switch("prod", "collection_v2"); err != nil {
		t.Fatalf("Failed to switch alias: %v", err)
	}

	resolved = am.Resolve("prod")
	if resolved != "collection_v2" {
		t.Errorf("Expected resolved alias to be 'collection_v2', got '%s'", resolved)
	}

	// List aliases
	aliases := am.List()
	if len(aliases) != 1 {
		t.Errorf("Expected 1 alias, got %d", len(aliases))
	}

	// Delete alias
	if err := am.Delete("prod"); err != nil {
		t.Fatalf("Failed to delete alias: %v", err)
	}

	aliases = am.List()
	if len(aliases) != 0 {
		t.Errorf("Expected 0 aliases after delete, got %d", len(aliases))
	}
}

func TestSharding(t *testing.T) {
	vcfg := &config.VectorConfig{
		Dimension: 4,
		Metric:    config.MetricCosine,
		HNSW: config.HNSWConfig{
			M:              16,
			EfConstruction: 100,
			EfSearch:       50,
			MaxElements:    1000,
		},
	}

	sm, err := collection.NewShardManager(&collection.ShardManagerConfig{
		CollectionName: "shard_test",
		ShardCount:     4,
		ReplicaFactor:  1,
		VectorConfig:   vcfg,
		DataDir:        "/tmp/limyedb_shard_test",
	})
	if err != nil {
		t.Fatalf("Failed to create shard manager: %v", err)
	}

	// Insert points across shards
	for i := 0; i < 100; i++ {
		p := point.NewPointWithID(
			string(rune('a'+i%26))+string(rune('0'+i/26)),
			[]float32{float32(i) * 0.01, float32(i) * 0.02, float32(i) * 0.03, float32(i) * 0.04},
			map[string]interface{}{"index": i},
		)
		if err := sm.Insert(p); err != nil {
			t.Fatalf("Failed to insert point %d: %v", i, err)
		}
	}

	// Verify total size
	totalSize := sm.Size()
	if totalSize != 100 {
		t.Errorf("Expected 100 points total, got %d", totalSize)
	}

	// Verify distribution across shards
	infos := sm.ShardInfo()
	if len(infos) != 4 {
		t.Errorf("Expected 4 shards, got %d", len(infos))
	}

	// Each shard should have some points (not all in one)
	pointsPerShard := make([]int64, len(infos))
	for i, info := range infos {
		pointsPerShard[i] = info.Size
	}

	t.Logf("Points per shard: %v", pointsPerShard)

	// Search across all shards
	query := []float32{0.5, 0.5, 0.5, 0.5}
	candidates, err := sm.Search(query, 10, 50)
	if err != nil {
		t.Fatalf("Failed to search: %v", err)
	}

	if len(candidates) == 0 {
		t.Error("Expected some search results")
	}

	if len(candidates) > 10 {
		t.Errorf("Expected at most 10 results, got %d", len(candidates))
	}
}

func TestPayloadIndexConfiguration(t *testing.T) {
	cfg := &config.CollectionConfig{
		Name:      "payload_index_test",
		Dimension: 4,
		Metric:    config.MetricCosine,
		HNSW: config.HNSWConfig{
			M:              16,
			EfConstruction: 100,
			EfSearch:       50,
			MaxElements:    1000,
		},
	}

	coll, err := collection.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create collection: %v", err)
	}

	// Insert some points
	for i := 0; i < 10; i++ {
		p := point.NewPointWithID(
			string(rune('a'+i)),
			[]float32{float32(i) * 0.1, float32(i) * 0.2, float32(i) * 0.3, float32(i) * 0.4},
			map[string]interface{}{
				"category": "test",
				"price":    float64(i * 10),
			},
		)
		if err := coll.Insert(p); err != nil {
			t.Fatalf("Failed to insert point: %v", err)
		}
	}

	// Create payload index
	indexCfg := &collection.PayloadIndexConfig{
		FieldName: "price",
		FieldType: collection.FieldTypeFloat,
		IndexType: collection.IndexTypeNumeric,
	}

	if err := coll.CreatePayloadIndex(indexCfg); err != nil {
		t.Fatalf("Failed to create payload index: %v", err)
	}

	// Get indexes
	indexes := coll.GetPayloadIndexes()
	if len(indexes) == 0 {
		t.Error("Expected at least one payload index")
	}

	// Delete index
	if err := coll.DeletePayloadIndex("price"); err != nil {
		t.Fatalf("Failed to delete payload index: %v", err)
	}
}
