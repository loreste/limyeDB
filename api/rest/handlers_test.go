package rest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/limyedb/limyedb/pkg/collection"
	"github.com/limyedb/limyedb/pkg/config"
)

// setupTestServer creates a minimal Server backed by a temp-dir collection
// manager. The returned cleanup function removes the temp directory.
func setupTestServer(t *testing.T) (*Server, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "limyedb-rest-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	mgr, err := collection.NewManager(&collection.ManagerConfig{
		DataDir:        tmpDir,
		MaxCollections: 100,
	})
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create collection manager: %v", err)
	}

	cfg := &config.ServerConfig{
		RESTAddress:    ":0",
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		MaxRequestSize: 1 << 20, // 1 MiB
	}

	srv := NewServer(cfg, mgr, nil)

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}
	return srv, cleanup
}

// doJSON fires an HTTP request with a JSON body against the test router and
// returns the recorded response.
func doJSON(srv *Server, method, path string, body interface{}) *httptest.ResponseRecorder {
	var buf *bytes.Buffer
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewBuffer(b)
	} else {
		buf = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	return w
}

// decodeBody unmarshals a recorder body into a generic map.
func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("failed to decode response body: %v\nraw: %s", err, w.Body.String())
	}
	return m
}

// createCollection is a test helper that creates a collection via the API.
func createCollection(t *testing.T, srv *Server, name string, dim int) {
	t.Helper()
	w := doJSON(srv, http.MethodPost, "/collections", map[string]interface{}{
		"name":      name,
		"dimension": dim,
		"metric":    "cosine",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create collection %q: want 201, got %d; body: %s", name, w.Code, w.Body.String())
	}
}

// upsertPoints is a test helper that upserts points via the API.
func upsertPoints(t *testing.T, srv *Server, coll string, pts []map[string]interface{}) {
	t.Helper()
	w := doJSON(srv, http.MethodPut, "/collections/"+coll+"/points", map[string]interface{}{
		"points": pts,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("upsert points: want 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

// vec returns a float32 slice of the given dimension filled with val.
func vec(dim int, val float32) []float32 {
	v := make([]float32, dim)
	for i := range v {
		v[i] = val
	}
	return v
}

// ---------------------------------------------------------------------------
// Health / Readiness
// ---------------------------------------------------------------------------

func TestHandleHealth(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	w := doJSON(srv, http.MethodGet, "/health", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body := decodeBody(t, w)
	if body["status"] != "healthy" {
		t.Errorf("want status=healthy, got %v", body["status"])
	}
}

func TestHandleHealthContainsVersion(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	w := doJSON(srv, http.MethodGet, "/health", nil)
	body := decodeBody(t, w)
	if _, ok := body["version"]; !ok {
		t.Error("health response should contain a version field")
	}
}

func TestHandleReadiness(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	w := doJSON(srv, http.MethodGet, "/readiness", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body := decodeBody(t, w)
	if body["ready"] != true {
		t.Errorf("want ready=true, got %v", body["ready"])
	}
}

// ---------------------------------------------------------------------------
// Collections: Create
// ---------------------------------------------------------------------------

func TestCreateCollection(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	tests := []struct {
		name       string
		body       map[string]interface{}
		wantStatus int
	}{
		{
			name:       "valid collection",
			body:       map[string]interface{}{"name": "vecs", "dimension": 128, "metric": "cosine"},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "missing name",
			body:       map[string]interface{}{"dimension": 128},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing dimension",
			body:       map[string]interface{}{"name": "no-dim"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty body",
			body:       nil,
			wantStatus: http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doJSON(srv, http.MethodPost, "/collections", tt.body)
			if w.Code != tt.wantStatus {
				t.Errorf("want %d, got %d; body: %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestCreateCollectionDuplicate(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	createCollection(t, srv, "dup", 64)

	w := doJSON(srv, http.MethodPost, "/collections", map[string]interface{}{
		"name": "dup", "dimension": 64, "metric": "cosine",
	})
	if w.Code != http.StatusConflict {
		t.Errorf("want 409 for duplicate, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestCreateCollectionWithHNSW(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	w := doJSON(srv, http.MethodPost, "/collections", map[string]interface{}{
		"name":      "hnsw-coll",
		"dimension": 256,
		"metric":    "euclidean",
		"hnsw":      map[string]interface{}{"m": 32, "ef_construction": 400, "ef_search": 200},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d; body: %s", w.Code, w.Body.String())
	}
	body := decodeBody(t, w)
	if body["success"] != true {
		t.Errorf("want success=true, got %v", body["success"])
	}
}

// ---------------------------------------------------------------------------
// Collections: List
// ---------------------------------------------------------------------------

func TestListCollectionsEmpty(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	w := doJSON(srv, http.MethodGet, "/collections", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body := decodeBody(t, w)
	cols := body["collections"].([]interface{})
	if len(cols) != 0 {
		t.Errorf("want 0 collections, got %d", len(cols))
	}
}

func TestListCollections(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	createCollection(t, srv, "alpha", 32)
	createCollection(t, srv, "beta", 64)

	w := doJSON(srv, http.MethodGet, "/collections", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body := decodeBody(t, w)
	cols := body["collections"].([]interface{})
	if len(cols) != 2 {
		t.Errorf("want 2 collections, got %d", len(cols))
	}
}

// ---------------------------------------------------------------------------
// Collections: Get
// ---------------------------------------------------------------------------

func TestGetCollection(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	createCollection(t, srv, "get-me", 64)

	w := doJSON(srv, http.MethodGet, "/collections/get-me", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := decodeBody(t, w)
	data := body["data"].(map[string]interface{})
	if data["name"] != "get-me" {
		t.Errorf("want name=get-me, got %v", data["name"])
	}
	if int(data["dimension"].(float64)) != 64 {
		t.Errorf("want dimension=64, got %v", data["dimension"])
	}
}

func TestGetCollectionNotFound(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	w := doJSON(srv, http.MethodGet, "/collections/nonexistent", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Collections: Delete
// ---------------------------------------------------------------------------

func TestDeleteCollection(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	createCollection(t, srv, "to-delete", 64)

	w := doJSON(srv, http.MethodDelete, "/collections/to-delete", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d; body: %s", w.Code, w.Body.String())
	}

	// Confirm deleted
	w = doJSON(srv, http.MethodGet, "/collections/to-delete", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404 after delete, got %d", w.Code)
	}
}

func TestDeleteCollectionNotFound(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	w := doJSON(srv, http.MethodDelete, "/collections/ghost", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Points: Upsert
// ---------------------------------------------------------------------------

func TestUpsertAndGetPoint(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	const dim = 4
	createCollection(t, srv, "pts", dim)

	upsertPoints(t, srv, "pts", []map[string]interface{}{
		{"id": "p1", "vector": vec(dim, 0.5), "payload": map[string]interface{}{"color": "red"}},
	})

	w := doJSON(srv, http.MethodGet, "/collections/pts/points/p1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := decodeBody(t, w)
	data := body["data"].(map[string]interface{})
	if data["id"] != "p1" {
		t.Errorf("want id=p1, got %v", data["id"])
	}
}

func TestUpsertPointsCollectionNotFound(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	w := doJSON(srv, http.MethodPut, "/collections/missing/points", map[string]interface{}{
		"points": []map[string]interface{}{{"id": "x", "vector": []float32{1, 2, 3}}},
	})
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}

func TestUpsertPointsInvalidJSON(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	createCollection(t, srv, "bad-json", 4)

	req := httptest.NewRequest(http.MethodPut, "/collections/bad-json/points",
		bytes.NewBufferString(`{invalid json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestUpsertPointsMissingRequiredFields(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	createCollection(t, srv, "mf", 4)

	// Missing "points" key should be rejected
	w := doJSON(srv, http.MethodPut, "/collections/mf/points", map[string]interface{}{})
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for missing points key, got %d; body: %s", w.Code, w.Body.String())
	}

	// Empty points array is accepted by the server (zero inserts, no error)
	w = doJSON(srv, http.MethodPut, "/collections/mf/points", map[string]interface{}{
		"points": []interface{}{},
	})
	if w.Code != http.StatusOK {
		t.Errorf("want 200 for empty points array, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestUpsertSameIDTwice(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	const dim = 4
	createCollection(t, srv, "ow", dim)

	// First insert
	upsertPoints(t, srv, "ow", []map[string]interface{}{
		{"id": "ow1", "vector": vec(dim, 0.1), "payload": map[string]interface{}{"v": 1}},
	})

	// Second insert with the same ID should not error
	upsertPoints(t, srv, "ow", []map[string]interface{}{
		{"id": "ow1", "vector": vec(dim, 0.9), "payload": map[string]interface{}{"v": 2}},
	})

	// Point should still be retrievable
	w := doJSON(srv, http.MethodGet, "/collections/ow/points/ow1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body := decodeBody(t, w)
	data := body["data"].(map[string]interface{})
	if data["id"] != "ow1" {
		t.Errorf("want id=ow1, got %v", data["id"])
	}
}

// ---------------------------------------------------------------------------
// Points: Get (error)
// ---------------------------------------------------------------------------

func TestGetPointNotFound(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	createCollection(t, srv, "np", 4)

	w := doJSON(srv, http.MethodGet, "/collections/np/points/nonexistent", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Points: Delete
// ---------------------------------------------------------------------------

func TestDeletePoint(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	const dim = 4
	createCollection(t, srv, "dp", dim)
	upsertPoints(t, srv, "dp", []map[string]interface{}{
		{"id": "d1", "vector": vec(dim, 1.0)},
	})

	w := doJSON(srv, http.MethodDelete, "/collections/dp/points/d1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d; body: %s", w.Code, w.Body.String())
	}

	w = doJSON(srv, http.MethodGet, "/collections/dp/points/d1", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404 after delete, got %d", w.Code)
	}
}

func TestDeletePointNotFound(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	createCollection(t, srv, "dp2", 4)

	w := doJSON(srv, http.MethodDelete, "/collections/dp2/points/ghost", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d; body: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Search
// ---------------------------------------------------------------------------

func TestSearch(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	const dim = 4
	createCollection(t, srv, "sc", dim)

	pts := make([]map[string]interface{}, 5)
	for i := range pts {
		pts[i] = map[string]interface{}{
			"id":      fmt.Sprintf("s%d", i),
			"vector":  vec(dim, float32(i+1)*0.1),
			"payload": map[string]interface{}{"idx": i},
		}
	}
	upsertPoints(t, srv, "sc", pts)

	w := doJSON(srv, http.MethodPost, "/collections/sc/search", map[string]interface{}{
		"vector": vec(dim, 0.5),
		"limit":  3,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := decodeBody(t, w)
	results, ok := body["result"].([]interface{})
	if !ok {
		t.Fatalf("expected result array, got %v", body)
	}
	if len(results) == 0 {
		t.Error("expected non-empty search results")
	}
	if len(results) > 3 {
		t.Errorf("want at most 3 results, got %d", len(results))
	}
}

func TestSearchCollectionNotFound(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	w := doJSON(srv, http.MethodPost, "/collections/nope/search", map[string]interface{}{
		"vector": []float32{1, 2, 3, 4},
		"limit":  5,
	})
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}

func TestSearchMissingVector(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	createCollection(t, srv, "sbad", 4)

	w := doJSON(srv, http.MethodPost, "/collections/sbad/search", map[string]interface{}{
		"limit": 5,
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for missing vector, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestSearchDefaultLimit(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	const dim = 4
	createCollection(t, srv, "sdl", dim)

	pts := make([]map[string]interface{}, 15)
	for i := range pts {
		pts[i] = map[string]interface{}{
			"id":     fmt.Sprintf("dl%d", i),
			"vector": vec(dim, float32(i+1)*0.05),
		}
	}
	upsertPoints(t, srv, "sdl", pts)

	// No limit specified; handler defaults to 10
	w := doJSON(srv, http.MethodPost, "/collections/sdl/search", map[string]interface{}{
		"vector": vec(dim, 0.3),
	})
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := decodeBody(t, w)
	results := body["result"].([]interface{})
	if len(results) > 10 {
		t.Errorf("want at most 10 (default), got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// Batch Upsert
// ---------------------------------------------------------------------------

func TestBatchUpsert(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	const dim = 4
	createCollection(t, srv, "batch", dim)

	pts := make([]map[string]interface{}, 10)
	for i := range pts {
		pts[i] = map[string]interface{}{
			"id":     fmt.Sprintf("b%d", i),
			"vector": vec(dim, float32(i)*0.1),
		}
	}

	w := doJSON(srv, http.MethodPost, "/collections/batch/points/batch", map[string]interface{}{
		"points": pts,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d; body: %s", w.Code, w.Body.String())
	}
	body := decodeBody(t, w)
	data := body["data"].(map[string]interface{})
	if int(data["succeeded"].(float64)) != 10 {
		t.Errorf("want 10 succeeded, got %v", data["succeeded"])
	}
}

func TestBatchUpsertCollectionNotFound(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	w := doJSON(srv, http.MethodPost, "/collections/void/points/batch", map[string]interface{}{
		"points": []map[string]interface{}{{"id": "x", "vector": []float32{1, 2, 3, 4}}},
	})
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Full Lifecycle
// ---------------------------------------------------------------------------

func TestFullLifecycle(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	const dim = 4

	// 1. Create
	createCollection(t, srv, "lifecycle", dim)

	// 2. List
	w := doJSON(srv, http.MethodGet, "/collections", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d", w.Code)
	}
	body := decodeBody(t, w)
	if len(body["collections"].([]interface{})) != 1 {
		t.Fatal("expected 1 collection")
	}

	// 3. Upsert
	upsertPoints(t, srv, "lifecycle", []map[string]interface{}{
		{"id": "lc1", "vector": vec(dim, 0.2), "payload": map[string]interface{}{"tag": "a"}},
		{"id": "lc2", "vector": vec(dim, 0.8), "payload": map[string]interface{}{"tag": "b"}},
	})

	// 4. Get point
	w = doJSON(srv, http.MethodGet, "/collections/lifecycle/points/lc1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get point: want 200, got %d", w.Code)
	}

	// 5. Search
	w = doJSON(srv, http.MethodPost, "/collections/lifecycle/search", map[string]interface{}{
		"vector": vec(dim, 0.2),
		"limit":  2,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("search: want 200, got %d; body: %s", w.Code, w.Body.String())
	}

	// 6. Delete point
	w = doJSON(srv, http.MethodDelete, "/collections/lifecycle/points/lc1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete point: want 200, got %d", w.Code)
	}

	// 7. Confirm point gone
	w = doJSON(srv, http.MethodGet, "/collections/lifecycle/points/lc1", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("deleted point: want 404, got %d", w.Code)
	}

	// 8. Delete collection
	w = doJSON(srv, http.MethodDelete, "/collections/lifecycle", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete collection: want 200, got %d", w.Code)
	}

	// 9. Confirm collection gone
	w = doJSON(srv, http.MethodGet, "/collections/lifecycle", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("deleted collection: want 404, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Wrong HTTP Method
// ---------------------------------------------------------------------------

func TestWrongHTTPMethod(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"POST to health", http.MethodPost, "/health"},
		{"DELETE to health", http.MethodDelete, "/health"},
		{"PUT to collections list", http.MethodPut, "/collections"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doJSON(srv, tt.method, tt.path, nil)
			if w.Code == http.StatusOK || w.Code == http.StatusCreated {
				t.Errorf("wrong method should not succeed, got %d", w.Code)
			}
		})
	}
}
