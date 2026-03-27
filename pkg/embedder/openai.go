package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type OpenAIEmbedder struct {
	APIKey string
	Model  string
	client *http.Client
}

func NewOpenAIEmbedder(apiKey, model string) *OpenAIEmbedder {
	if model == "" {
		model = "text-embedding-3-small"
	}
	return &OpenAIEmbedder{
		APIKey: apiKey,
		Model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

type openAIRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type openAIResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

func (e *OpenAIEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody := openAIRequest{
		Input: texts,
		Model: e.Model,
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/embeddings", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+e.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI API returned status %d", resp.StatusCode)
	}

	var payload openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	if len(payload.Data) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(payload.Data))
	}

	result := make([][]float32, len(texts))
	for i, d := range payload.Data {
		result[i] = d.Embedding
	}

	return result, nil
}

func (e *OpenAIEmbedder) Dimension() int {
	return 1536 // Default for text-embedding-3-small, will evolve natively.
}
