// Package reranker provides cross-encoder re-ranking for search results.
// After initial vector retrieval, a re-ranker can reorder results using
// a more expensive but more accurate cross-encoder model.
package reranker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"
)

// Result represents a single search result to be re-ranked.
type Result struct {
	ID      string  `json:"id"`
	Text    string  `json:"text"`
	Score   float32 `json:"score"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

// Reranker reorders search results using a cross-encoder model.
type Reranker interface {
	// Rerank reorders results based on relevance to the query.
	// Returns results sorted by relevance score (highest first).
	Rerank(ctx context.Context, query string, results []Result, topN int) ([]Result, error)
}

// Config holds reranker configuration.
type Config struct {
	Provider string `json:"provider"` // "cohere", "http"
	Model    string `json:"model"`
	APIKey   string `json:"api_key"`
	Endpoint string `json:"endpoint"` // For custom HTTP endpoints
	TopN     int    `json:"top_n"`    // Max results to return (0 = all)
	Timeout  time.Duration `json:"timeout"`
}

// New creates a Reranker for the given provider.
func New(cfg Config) (Reranker, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	switch cfg.Provider {
	case "cohere":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("cohere reranker requires api_key")
		}
		if cfg.Model == "" {
			cfg.Model = "rerank-english-v3.0"
		}
		return &cohereReranker{
			apiKey: cfg.APIKey,
			model:  cfg.Model,
			client: &http.Client{Timeout: cfg.Timeout},
		}, nil
	case "http":
		if cfg.Endpoint == "" {
			return nil, fmt.Errorf("http reranker requires endpoint")
		}
		return &httpReranker{
			endpoint: cfg.Endpoint,
			apiKey:   cfg.APIKey,
			client:   &http.Client{Timeout: cfg.Timeout},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported reranker provider: %s", cfg.Provider)
	}
}

// cohereReranker uses the Cohere Rerank API.
type cohereReranker struct {
	apiKey string
	model  string
	client *http.Client
}

type cohereRerankRequest struct {
	Model           string   `json:"model"`
	Query           string   `json:"query"`
	Documents       []string `json:"documents"`
	TopN            int      `json:"top_n,omitempty"`
	ReturnDocuments bool     `json:"return_documents"`
}

type cohereRerankResponse struct {
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
}

func (c *cohereReranker) Rerank(ctx context.Context, query string, results []Result, topN int) ([]Result, error) {
	if len(results) == 0 {
		return results, nil
	}

	docs := make([]string, len(results))
	for i, r := range results {
		docs[i] = r.Text
	}

	reqBody := cohereRerankRequest{
		Model:           c.model,
		Query:           query,
		Documents:       docs,
		TopN:            topN,
		ReturnDocuments: false,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal rerank request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.cohere.ai/v1/rerank", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rerank request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("cohere rerank API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var rerankResp cohereRerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&rerankResp); err != nil {
		return nil, fmt.Errorf("failed to decode rerank response: %w", err)
	}

	reranked := make([]Result, 0, len(rerankResp.Results))
	for _, r := range rerankResp.Results {
		if r.Index >= 0 && r.Index < len(results) {
			result := results[r.Index]
			result.Score = float32(r.RelevanceScore)
			reranked = append(reranked, result)
		}
	}

	return reranked, nil
}

// httpReranker calls a generic HTTP reranking endpoint.
// The endpoint receives a JSON body with "query" and "documents" fields
// and returns a JSON array of {"index": int, "score": float} objects.
type httpReranker struct {
	endpoint string
	apiKey   string
	client   *http.Client
}

type httpRerankRequest struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n,omitempty"`
}

type httpRerankResponse struct {
	Results []struct {
		Index int     `json:"index"`
		Score float64 `json:"score"`
	} `json:"results"`
}

func (h *httpReranker) Rerank(ctx context.Context, query string, results []Result, topN int) ([]Result, error) {
	if len(results) == 0 {
		return results, nil
	}

	docs := make([]string, len(results))
	for i, r := range results {
		docs[i] = r.Text
	}

	reqBody := httpRerankRequest{
		Query:     query,
		Documents: docs,
		TopN:      topN,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal rerank request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if h.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.apiKey)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rerank request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("rerank endpoint error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var rerankResp httpRerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&rerankResp); err != nil {
		return nil, fmt.Errorf("failed to decode rerank response: %w", err)
	}

	reranked := make([]Result, 0, len(rerankResp.Results))
	for _, r := range rerankResp.Results {
		if r.Index >= 0 && r.Index < len(results) {
			result := results[r.Index]
			result.Score = float32(r.Score)
			reranked = append(reranked, result)
		}
	}

	// Sort by score descending
	sort.Slice(reranked, func(i, j int) bool {
		return reranked[i].Score > reranked[j].Score
	})

	if topN > 0 && len(reranked) > topN {
		reranked = reranked[:topN]
	}

	return reranked, nil
}
