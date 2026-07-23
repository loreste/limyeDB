package tokenize

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// VLLMTokenizer counts tokens using a vLLM server's /tokenize endpoint.
//
// vLLM applies the served model's real tokenizer, so counts are exact -- the
// only provider in a typical self-hosted stack that offers this. Ollama has no
// equivalent endpoint, so it falls back to Estimator.
type VLLMTokenizer struct {
	client   *http.Client
	endpoint string
	model    string

	// fallback absorbs transport failures so a tokenizer outage degrades to an
	// estimate instead of failing the request.
	fallback *Estimator
}

// VLLMConfig configures a VLLMTokenizer.
type VLLMConfig struct {
	// BaseURL is the vLLM server root, e.g. http://localhost:8000.
	BaseURL string

	// Model is the served model name.
	Model string

	// Timeout bounds a single tokenize call. Defaults to 5s.
	Timeout time.Duration

	// HTTPClient overrides the client, primarily for tests.
	HTTPClient *http.Client
}

// NewVLLMTokenizer builds a tokenizer backed by a vLLM server.
func NewVLLMTokenizer(cfg VLLMConfig) (*VLLMTokenizer, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("tokenize: vLLM base URL is required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: cfg.Timeout}
	}

	return &VLLMTokenizer{
		client:   client,
		endpoint: strings.TrimRight(cfg.BaseURL, "/") + "/tokenize",
		model:    cfg.Model,
		fallback: NewEstimator(DefaultEstimatorConfig()),
	}, nil
}

// Name implements Tokenizer.
func (v *VLLMTokenizer) Name() string { return "vllm/" + v.model }

// estimate returns a degraded, non-exact count. Used when the server cannot be
// reached so budgeting degrades instead of failing.
func (v *VLLMTokenizer) estimate(text string) (Count, error) {
	return Count{Tokens: v.fallback.Estimate(text), Exact: false}, nil
}

type vllmTokenizeRequest struct {
	Model  string `json:"model,omitempty"`
	Prompt string `json:"prompt"`
}

type vllmTokenizeResponse struct {
	Count  int   `json:"count"`
	Tokens []int `json:"tokens"`
}

// Count implements Tokenizer.
//
// On any transport or protocol failure it falls back to a local estimate rather
// than returning an error, because a budgeting call should degrade rather than
// fail. The fallback is reported honestly as Exact: false, so a caller can add
// headroom for that specific value instead of trusting a count that only looks
// authoritative.
func (v *VLLMTokenizer) Count(ctx context.Context, text string) (Count, error) {
	if text == "" {
		return Count{Tokens: 0, Exact: true}, nil
	}

	body, err := json.Marshal(vllmTokenizeRequest{Model: v.model, Prompt: text})
	if err != nil {
		return v.estimate(text)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.endpoint, bytes.NewReader(body))
	if err != nil {
		return v.estimate(text)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := v.client.Do(req)
	if err != nil {
		return v.estimate(text)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return v.estimate(text)
	}

	var parsed vllmTokenizeResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return v.estimate(text)
	}

	// Prefer the explicit count; some builds return only the token array.
	if parsed.Count > 0 {
		return Count{Tokens: parsed.Count, Exact: true}, nil
	}
	if len(parsed.Tokens) > 0 {
		return Count{Tokens: len(parsed.Tokens), Exact: true}, nil
	}
	return v.estimate(text)
}
