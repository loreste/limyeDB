package embedder

import (
	"context"
	"fmt"
)

// Embedder defines the generic interface for mapping text into dense contextual vectors.
type Embedder interface {
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	Dimension() int
}

// Config represents the dynamic provider configuration securely initialized via V2 REST payloads.
type Config struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	APIKey   string `json:"api_key"`
}

// New initializes the correct Embedder implementation dynamically.
func New(cfg Config) (Embedder, error) {
	switch cfg.Provider {
	case "openai":
		return NewOpenAIEmbedder(cfg.APIKey, cfg.Model), nil
	case "cohere":
		return NewCohereEmbedder(cfg.APIKey, cfg.Model), nil
	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", cfg.Provider)
	}
}
