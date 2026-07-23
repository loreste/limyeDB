package vectorizer

import (
	"errors"
	"net/http"
	"strings"
	"time"
)

// Default wiring for a local vLLM server.
const (
	defaultVLLMEndpoint = "http://localhost:8000/v1/embeddings"
	defaultVLLMTimeout  = 30 * time.Second
)

// NewVLLMVectorizer creates a vectorizer backed by a vLLM server.
//
// vLLM exposes an OpenAI-compatible /v1/embeddings endpoint, so this reuses
// OpenAIVectorizer's request, batching, and retry logic rather than duplicating
// it. It exists as a separate constructor because two things differ and both
// blocked vLLM entirely:
//
//   - No API key. NewOpenAIVectorizer rejects an empty key, but a self-hosted
//     vLLM normally has no authentication, so the key is optional here and the
//     Authorization header is omitted when it is absent.
//   - A different default endpoint, since there is no hosted URL to fall back to.
//
// The Ollama path (VectorizerLocal) cannot serve this role: it speaks Ollama's
// native {"model","prompt"} shape and reads back a single {"embedding"}, which
// is not what vLLM returns.
func NewVLLMVectorizer(cfg *VectorizerConfig) (*OpenAIVectorizer, error) {
	if cfg == nil {
		return nil, errors.New("vllm: config is required")
	}
	if cfg.Model == "" {
		// vLLM serves one model per process but still requires the name in the
		// request, and there is no sensible default to invent.
		return nil, errors.New("vllm: model is required")
	}

	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = defaultVLLMEndpoint
	} else {
		endpoint = normalizeVLLMEndpoint(endpoint)
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultVLLMTimeout
	}

	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 32
	}

	return &OpenAIVectorizer{
		client:     &http.Client{Timeout: timeout},
		apiKey:     cfg.APIKey, // optional; omitted from the request when empty
		model:      cfg.Model,
		dimension:  cfg.Dimension,
		endpoint:   endpoint,
		batchSize:  batchSize,
		retryCount: cfg.RetryCount,
		provider:   "vllm",
	}, nil
}

// normalizeVLLMEndpoint accepts either a server root or a full embeddings URL.
//
// Operators reasonably configure "http://host:8000", and silently posting
// embeddings to the server root would fail with a confusing 404.
func normalizeVLLMEndpoint(endpoint string) string {
	trimmed := strings.TrimRight(endpoint, "/")
	if strings.HasSuffix(trimmed, "/embeddings") {
		return trimmed
	}
	if strings.HasSuffix(trimmed, "/v1") {
		return trimmed + "/embeddings"
	}
	return trimmed + "/v1/embeddings"
}
