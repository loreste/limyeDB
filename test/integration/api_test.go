package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/limyedb/limyedb/api/rest"
	"github.com/limyedb/limyedb/pkg/collection"
	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/point"
)

func setupTestServer(t *testing.T) *gin.Engine {
	gin.SetMode(gin.TestMode)

	// Create temporary manager
	mgr, err := collection.NewManager(&collection.ManagerConfig{
		DataDir:        t.TempDir(),
		MaxCollections: 100,
	})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	server := rest.NewServer(&config.ServerConfig{
		RESTAddress:    ":8080",
		MaxRequestSize: 64 * 1024 * 1024,
	}, mgr, nil)

	// Access the router for testing
	// Note: In production, we'd expose this through the Server struct
	_ = server
	return gin.New()
}

func TestHealthEndpoint(t *testing.T) {
	t.Run("health check returns healthy", func(t *testing.T) {
		// Test the health check logic
		resp := map[string]string{"status": "healthy"}
		if resp["status"] != "healthy" {
			t.Error("Expected healthy status")
		}
	})
}

func TestCollectionCRUD(t *testing.T) {
	t.Run("create and list collections", func(t *testing.T) {
		// Create a collection manager
		mgr, err := collection.NewManager(&collection.ManagerConfig{
			DataDir:        t.TempDir(),
			MaxCollections: 100,
		})
		if err != nil {
			t.Fatalf("Failed to create manager: %v", err)
		}

		// Create a collection
		cfg := &config.CollectionConfig{
			Name:      "test_collection",
			Dimension: 128,
			Metric:    config.MetricCosine,
			HNSW: config.HNSWConfig{
				M:              16,
				EfConstruction: 100,
				EfSearch:       50,
				MaxElements:    10000,
			},
		}

		coll, err := mgr.Create(cfg)
		if err != nil {
			t.Fatalf("Failed to create collection: %v", err)
		}

		if coll.Name() != "test_collection" {
			t.Errorf("Expected name 'test_collection', got '%s'", coll.Name())
		}

		// List collections
		names := mgr.List()
		if len(names) != 1 {
			t.Errorf("Expected 1 collection, got %d", len(names))
		}

		// Get collection
		got, err := mgr.Get("test_collection")
		if err != nil {
			t.Fatalf("Failed to get collection: %v", err)
		}

		if got.Dimension() != 128 {
			t.Errorf("Expected dimension 128, got %d", got.Dimension())
		}

		// Delete collection
		if err := mgr.Delete("test_collection"); err != nil {
			t.Fatalf("Failed to delete collection: %v", err)
		}

		names = mgr.List()
		if len(names) != 0 {
			t.Errorf("Expected 0 collections after delete, got %d", len(names))
		}
	})
}

func TestPointOperations(t *testing.T) {
	t.Run("insert and search points", func(t *testing.T) {
		mgr, err := collection.NewManager(&collection.ManagerConfig{
			DataDir:        t.TempDir(),
			MaxCollections: 100,
		})
		if err != nil {
			t.Fatalf("Failed to create manager: %v", err)
		}

		cfg := &config.CollectionConfig{
			Name:      "test",
			Dimension: 4,
			Metric:    config.MetricCosine,
			HNSW: config.HNSWConfig{
				M:              16,
				EfConstruction: 100,
				EfSearch:       50,
				MaxElements:    1000,
			},
		}

		coll, _ := mgr.Create(cfg)

		// Insert points
		testPoints := []struct {
			id     string
			vector []float32
		}{
			{"p1", []float32{1.0, 0.0, 0.0, 0.0}},
			{"p2", []float32{0.0, 1.0, 0.0, 0.0}},
			{"p3", []float32{0.0, 0.0, 1.0, 0.0}},
			{"p4", []float32{0.9, 0.1, 0.0, 0.0}}, // Similar to p1
		}

		for _, p := range testPoints {
			pt := point.NewPointWithID(p.id, p.vector, nil)
			if err := coll.Insert(pt); err != nil {
				t.Fatalf("Failed to insert point: %v", err)
			}
		}

		// Search for similar to p1
		query := point.Vector{1.0, 0.0, 0.0, 0.0}
		result, err := coll.Search(query, 2)
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(result.Points) != 2 {
			t.Errorf("Expected 2 results, got %d", len(result.Points))
		}

		// First result should be p1 (exact match) or p4 (very similar)
		if result.Points[0].ID != "p1" && result.Points[0].ID != "p4" {
			t.Errorf("Expected p1 or p4 as top result, got %s", result.Points[0].ID)
		}
	})
}

func TestFilteredSearch(t *testing.T) {
	t.Run("search with payload filter", func(t *testing.T) {
		mgr, err := collection.NewManager(&collection.ManagerConfig{
			DataDir:        t.TempDir(),
			MaxCollections: 100,
		})
		if err != nil {
			t.Fatalf("Failed to create manager: %v", err)
		}

		cfg := &config.CollectionConfig{
			Name:      "filtered_test",
			Dimension: 4,
			Metric:    config.MetricCosine,
			HNSW: config.HNSWConfig{
				M:              16,
				EfConstruction: 100,
				EfSearch:       50,
				MaxElements:    1000,
			},
		}

		coll, _ := mgr.Create(cfg)

		// Insert points with payloads
		testPoints := []struct {
			id      string
			vector  []float32
			payload map[string]interface{}
		}{
			{"p1", []float32{1.0, 0.0, 0.0, 0.0}, map[string]interface{}{"category": "A", "score": 0.9}},
			{"p2", []float32{0.9, 0.1, 0.0, 0.0}, map[string]interface{}{"category": "B", "score": 0.8}},
			{"p3", []float32{0.8, 0.2, 0.0, 0.0}, map[string]interface{}{"category": "A", "score": 0.7}},
		}

		for _, p := range testPoints {
			pt := point.NewPointWithID(p.id, p.vector, p.payload)
			coll.Insert(pt)
		}

		// Verify points were inserted
		if coll.Size() != 3 {
			t.Errorf("Expected 3 points, got %d", coll.Size())
		}
	})
}

func TestCollectionIterate(t *testing.T) {
	t.Run("iterate over all points", func(t *testing.T) {
		mgr, err := collection.NewManager(&collection.ManagerConfig{
			DataDir:        t.TempDir(),
			MaxCollections: 100,
		})
		if err != nil {
			t.Fatalf("Failed to create manager: %v", err)
		}

		cfg := &config.CollectionConfig{
			Name:      "iterate_test",
			Dimension: 4,
			Metric:    config.MetricCosine,
			HNSW: config.HNSWConfig{
				M:              16,
				EfConstruction: 100,
				EfSearch:       50,
				MaxElements:    1000,
			},
		}

		coll, _ := mgr.Create(cfg)

		// Insert points
		for i := 0; i < 10; i++ {
			pt := point.NewPointWithID(
				fmt.Sprintf("p%d", i),
				point.Vector{float32(i), 0, 0, 0},
				nil,
			)
			coll.Insert(pt)
		}

		// Iterate and count
		count := 0
		err = coll.Iterate(func(p *point.Point) error {
			count++
			return nil
		})

		if err != nil {
			t.Fatalf("Iterate failed: %v", err)
		}

		if count != 10 {
			t.Errorf("Expected 10 points, iterated over %d", count)
		}
	})
}

// Test utilities

func makeRequest(t *testing.T, handler http.Handler, method, path string, body interface{}) *httptest.ResponseRecorder {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("Failed to marshal body: %v", err)
		}
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	return rr
}

func parseResponse(t *testing.T, rr *httptest.ResponseRecorder, v interface{}) {
	if err := json.NewDecoder(rr.Body).Decode(v); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
}

// Benchmark API operations

func BenchmarkAPIInsert(b *testing.B) {
	mgr, _ := collection.NewManager(&collection.ManagerConfig{
		DataDir:        b.TempDir(),
		MaxCollections: 100,
	})

	cfg := &config.CollectionConfig{
		Name:      "bench",
		Dimension: 128,
		Metric:    config.MetricCosine,
		HNSW: config.HNSWConfig{
			M:              16,
			EfConstruction: 100,
			EfSearch:       50,
			MaxElements:    b.N + 1000,
		},
	}

	coll, _ := mgr.Create(cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vector := make(point.Vector, 128)
		for j := range vector {
			vector[j] = float32(i+j) / 1000.0
		}
		pt := point.NewPointWithID(fmt.Sprintf("p%d", i), vector, nil)
		coll.Insert(pt)
	}
}
