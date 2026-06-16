package reranker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewCohereReranker(t *testing.T) {
	t.Parallel()

	r, err := New(Config{Provider: "cohere", APIKey: "test-key"})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if r == nil {
		t.Fatal("Expected non-nil reranker")
	}
}

func TestNewHTTPReranker(t *testing.T) {
	t.Parallel()

	r, err := New(Config{Provider: "http", Endpoint: "http://localhost:8080/rerank"})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if r == nil {
		t.Fatal("Expected non-nil reranker")
	}
}

func TestNewUnsupportedProvider(t *testing.T) {
	t.Parallel()

	_, err := New(Config{Provider: "unknown"})
	if err == nil {
		t.Fatal("Expected error for unsupported provider")
	}
}

func TestNewCohereNoAPIKey(t *testing.T) {
	t.Parallel()

	_, err := New(Config{Provider: "cohere"})
	if err == nil {
		t.Fatal("Expected error for missing API key")
	}
}

func TestNewHTTPNoEndpoint(t *testing.T) {
	t.Parallel()

	_, err := New(Config{Provider: "http"})
	if err == nil {
		t.Fatal("Expected error for missing endpoint")
	}
}

func TestHTTPRerankEmptyResults(t *testing.T) {
	t.Parallel()

	r, _ := New(Config{Provider: "http", Endpoint: "http://localhost:8080/rerank"})
	results, err := r.Rerank(context.Background(), "test query", nil, 10)
	if err != nil {
		t.Fatalf("Rerank() failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}
}

func TestHTTPRerankerIntegration(t *testing.T) {
	t.Parallel()

	// Create a mock rerank server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req httpRerankRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Return documents in reverse order with decreasing scores
		resp := httpRerankResponse{}
		for i := len(req.Documents) - 1; i >= 0; i-- {
			resp.Results = append(resp.Results, struct {
				Index int     `json:"index"`
				Score float64 `json:"score"`
			}{
				Index: i,
				Score: float64(len(req.Documents)-i) / float64(len(req.Documents)),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	r, err := New(Config{Provider: "http", Endpoint: server.URL})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	results := []Result{
		{ID: "1", Text: "first document", Score: 0.9},
		{ID: "2", Text: "second document", Score: 0.8},
		{ID: "3", Text: "third document", Score: 0.7},
	}

	reranked, err := r.Rerank(context.Background(), "test query", results, 0)
	if err != nil {
		t.Fatalf("Rerank() failed: %v", err)
	}

	if len(reranked) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(reranked))
	}

	// The mock gives highest score to index 0 (ID "1"), so it should be first
	if reranked[0].ID != "1" {
		t.Errorf("Expected first result ID=1, got %s", reranked[0].ID)
	}

	// Scores should be descending
	for i := 1; i < len(reranked); i++ {
		if reranked[i].Score > reranked[i-1].Score {
			t.Errorf("Results not sorted by score: %.2f > %.2f at index %d", reranked[i].Score, reranked[i-1].Score, i)
		}
	}
}

func TestHTTPRerankerWithTopN(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := httpRerankResponse{
			Results: []struct {
				Index int     `json:"index"`
				Score float64 `json:"score"`
			}{
				{Index: 2, Score: 0.95},
				{Index: 0, Score: 0.85},
				{Index: 1, Score: 0.75},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	r, _ := New(Config{Provider: "http", Endpoint: server.URL})

	results := []Result{
		{ID: "1", Text: "doc1"},
		{ID: "2", Text: "doc2"},
		{ID: "3", Text: "doc3"},
	}

	reranked, err := r.Rerank(context.Background(), "query", results, 2)
	if err != nil {
		t.Fatalf("Rerank() failed: %v", err)
	}

	if len(reranked) != 2 {
		t.Errorf("Expected 2 results with topN=2, got %d", len(reranked))
	}
}

func TestHTTPRerankerServerError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer server.Close()

	r, _ := New(Config{Provider: "http", Endpoint: server.URL})

	results := []Result{{ID: "1", Text: "doc"}}
	_, err := r.Rerank(context.Background(), "query", results, 0)
	if err == nil {
		t.Fatal("Expected error on server error response")
	}
}

func TestHTTPRerankerWithAuth(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		resp := httpRerankResponse{
			Results: []struct {
				Index int     `json:"index"`
				Score float64 `json:"score"`
			}{{Index: 0, Score: 0.9}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	r, _ := New(Config{Provider: "http", Endpoint: server.URL, APIKey: "test-key"})
	results := []Result{{ID: "1", Text: "doc"}}

	reranked, err := r.Rerank(context.Background(), "query", results, 0)
	if err != nil {
		t.Fatalf("Rerank() failed: %v", err)
	}
	if len(reranked) != 1 {
		t.Errorf("Expected 1 result, got %d", len(reranked))
	}
}
