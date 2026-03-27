package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type CohereEmbedder struct {
	APIKey string
	Model  string
	client *http.Client
}

func NewCohereEmbedder(apiKey, model string) *CohereEmbedder {
	if model == "" {
		model = "embed-english-v3.0"
	}
	return &CohereEmbedder{
		APIKey: apiKey,
		Model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

type cohereRequest struct {
	Texts     []string `json:"texts"`
	Model     string   `json:"model"`
	InputType string   `json:"input_type"`
}

type cohereResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

func (e *CohereEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody := cohereRequest{
		Texts:     texts,
		Model:     e.Model,
		InputType: "search_document",
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.cohere.ai/v1/embed", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+e.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Request-Source", "limyedb")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Cohere API returned status %d", resp.StatusCode)
	}

	var payload cohereResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	if len(payload.Embeddings) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(payload.Embeddings))
	}

	return payload.Embeddings, nil
}

func (e *CohereEmbedder) Dimension() int {
	return 1024 // standard embed-english-v3.0 dimension
}
