package vectorizer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/limyedb/limyedb/pkg/point"
)

// Vectorizer is the interface for embedding models
type Vectorizer interface {
	// Vectorize converts text to a vector embedding
	Vectorize(ctx context.Context, text string) (point.Vector, error)

	// VectorizeBatch converts multiple texts to vector embeddings
	VectorizeBatch(ctx context.Context, texts []string) ([]point.Vector, error)

	// Dimension returns the output dimension of this vectorizer
	Dimension() int

	// Name returns the name of this vectorizer
	Name() string
}

// VectorizerType represents the type of vectorizer
type VectorizerType string

const (
	VectorizerOpenAI       VectorizerType = "openai"
	VectorizerCohere       VectorizerType = "cohere"
	VectorizerHuggingFace  VectorizerType = "huggingface"
	VectorizerLocal        VectorizerType = "local"
	VectorizerCustom       VectorizerType = "custom"
)

// VectorizerConfig holds configuration for a vectorizer
type VectorizerConfig struct {
	Type       VectorizerType `json:"type"`
	Model      string         `json:"model"`
	APIKey     string         `json:"api_key,omitempty"`
	Endpoint   string         `json:"endpoint,omitempty"`
	Dimension  int            `json:"dimension"`
	BatchSize  int            `json:"batch_size"`
	Timeout    time.Duration  `json:"timeout"`
	RetryCount int            `json:"retry_count"`
}

// DefaultVectorizerConfig returns default configuration
func DefaultVectorizerConfig() *VectorizerConfig {
	return &VectorizerConfig{
		Type:       VectorizerOpenAI,
		Model:      "text-embedding-3-small",
		Dimension:  1536,
		BatchSize:  100,
		Timeout:    30 * time.Second,
		RetryCount: 3,
	}
}

// VectorizerManager manages multiple vectorizers
type VectorizerManager struct {
	vectorizers map[string]Vectorizer
	configs     map[string]*VectorizerConfig
	mu          sync.RWMutex
}

// NewVectorizerManager creates a new vectorizer manager
func NewVectorizerManager() *VectorizerManager {
	return &VectorizerManager{
		vectorizers: make(map[string]Vectorizer),
		configs:     make(map[string]*VectorizerConfig),
	}
}

// Register registers a vectorizer with a name
func (m *VectorizerManager) Register(name string, cfg *VectorizerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	vectorizer, err := createVectorizer(cfg)
	if err != nil {
		return err
	}

	m.vectorizers[name] = vectorizer
	m.configs[name] = cfg
	return nil
}

// Get returns a vectorizer by name
func (m *VectorizerManager) Get(name string) (Vectorizer, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.vectorizers[name]
	return v, ok
}

// List returns all registered vectorizer names
func (m *VectorizerManager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.vectorizers))
	for name := range m.vectorizers {
		names = append(names, name)
	}
	return names
}

// Delete removes a vectorizer
func (m *VectorizerManager) Delete(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.vectorizers, name)
	delete(m.configs, name)
}

func createVectorizer(cfg *VectorizerConfig) (Vectorizer, error) {
	switch cfg.Type {
	case VectorizerOpenAI:
		return NewOpenAIVectorizer(cfg)
	case VectorizerCohere:
		return NewCohereVectorizer(cfg)
	case VectorizerHuggingFace:
		return NewHuggingFaceVectorizer(cfg)
	case VectorizerLocal:
		return NewLocalVectorizer(cfg)
	case VectorizerCustom:
		return NewCustomVectorizer(cfg)
	default:
		return nil, fmt.Errorf("unknown vectorizer type: %s", cfg.Type)
	}
}

// OpenAIVectorizer uses OpenAI's embedding API
type OpenAIVectorizer struct {
	client     *http.Client
	apiKey     string
	model      string
	dimension  int
	endpoint   string
	batchSize  int
	retryCount int
}

// NewOpenAIVectorizer creates an OpenAI vectorizer
func NewOpenAIVectorizer(cfg *VectorizerConfig) (*OpenAIVectorizer, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("OpenAI API key is required")
	}

	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/embeddings"
	}

	model := cfg.Model
	if model == "" {
		model = "text-embedding-3-small"
	}

	dimension := cfg.Dimension
	if dimension == 0 {
		// Default dimensions based on model
		switch model {
		case "text-embedding-3-small":
			dimension = 1536
		case "text-embedding-3-large":
			dimension = 3072
		case "text-embedding-ada-002":
			dimension = 1536
		default:
			dimension = 1536
		}
	}

	return &OpenAIVectorizer{
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		apiKey:     cfg.APIKey,
		model:      model,
		dimension:  dimension,
		endpoint:   endpoint,
		batchSize:  cfg.BatchSize,
		retryCount: cfg.RetryCount,
	}, nil
}

type openAIRequest struct {
	Input          interface{} `json:"input"`
	Model          string      `json:"model"`
	EncodingFormat string      `json:"encoding_format,omitempty"`
	Dimensions     int         `json:"dimensions,omitempty"`
}

type openAIResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (v *OpenAIVectorizer) Vectorize(ctx context.Context, text string) (point.Vector, error) {
	vectors, err := v.VectorizeBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, errors.New("no embedding returned")
	}
	return vectors[0], nil
}

func (v *OpenAIVectorizer) VectorizeBatch(ctx context.Context, texts []string) ([]point.Vector, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	// Process in batches
	var allVectors []point.Vector

	for i := 0; i < len(texts); i += v.batchSize {
		end := i + v.batchSize
		if end > len(texts) {
			end = len(texts)
		}

		batch := texts[i:end]
		vectors, err := v.vectorizeBatchInternal(ctx, batch)
		if err != nil {
			return nil, err
		}

		allVectors = append(allVectors, vectors...)
	}

	return allVectors, nil
}

func (v *OpenAIVectorizer) vectorizeBatchInternal(ctx context.Context, texts []string) ([]point.Vector, error) {
	reqBody := openAIRequest{
		Input: texts,
		Model: v.model,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	var resp *http.Response
	var lastErr error

	for attempt := 0; attempt <= v.retryCount; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "POST", v.endpoint, bytes.NewReader(jsonBody))
		if err != nil {
			return nil, err
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+v.apiKey)

		resp, err = v.client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}

		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			if closeErr := resp.Body.Close(); closeErr != nil {
				lastErr = fmt.Errorf("API returned status %d and failed to close response body: %v", resp.StatusCode, closeErr)
				time.Sleep(time.Duration(attempt+1) * time.Second)
				continue
			}
			lastErr = fmt.Errorf("API returned status %d", resp.StatusCode)
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}

		break
	}

	if resp == nil {
		return nil, lastErr
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apiResp openAIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("OpenAI API error: %s", apiResp.Error.Message)
	}

	vectors := make([]point.Vector, len(apiResp.Data))
	for _, data := range apiResp.Data {
		vec := make(point.Vector, len(data.Embedding))
		for i, val := range data.Embedding {
			vec[i] = float32(val)
		}
		vectors[data.Index] = vec
	}

	return vectors, nil
}

func (v *OpenAIVectorizer) Dimension() int {
	return v.dimension
}

func (v *OpenAIVectorizer) Name() string {
	return "openai/" + v.model
}

// CohereVectorizer uses Cohere's embedding API
type CohereVectorizer struct {
	client     *http.Client
	apiKey     string
	model      string
	dimension  int
	endpoint   string
	batchSize  int
	retryCount int
}

// NewCohereVectorizer creates a Cohere vectorizer
func NewCohereVectorizer(cfg *VectorizerConfig) (*CohereVectorizer, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("Cohere API key is required")
	}

	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = "https://api.cohere.ai/v1/embed"
	}

	model := cfg.Model
	if model == "" {
		model = "embed-english-v3.0"
	}

	dimension := cfg.Dimension
	if dimension == 0 {
		dimension = 1024 // Cohere default
	}

	return &CohereVectorizer{
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		apiKey:     cfg.APIKey,
		model:      model,
		dimension:  dimension,
		endpoint:   endpoint,
		batchSize:  cfg.BatchSize,
		retryCount: cfg.RetryCount,
	}, nil
}

type cohereRequest struct {
	Texts     []string `json:"texts"`
	Model     string   `json:"model"`
	InputType string   `json:"input_type"`
}

type cohereResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
	Error      *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (v *CohereVectorizer) Vectorize(ctx context.Context, text string) (point.Vector, error) {
	vectors, err := v.VectorizeBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, errors.New("no embedding returned")
	}
	return vectors[0], nil
}

func (v *CohereVectorizer) VectorizeBatch(ctx context.Context, texts []string) ([]point.Vector, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := cohereRequest{
		Texts:     texts,
		Model:     v.model,
		InputType: "search_document",
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", v.endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+v.apiKey)

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apiResp cohereResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("Cohere API error: %s", apiResp.Error.Message)
	}

	vectors := make([]point.Vector, len(apiResp.Embeddings))
	for i, emb := range apiResp.Embeddings {
		vec := make(point.Vector, len(emb))
		for j, val := range emb {
			vec[j] = float32(val)
		}
		vectors[i] = vec
	}

	return vectors, nil
}

func (v *CohereVectorizer) Dimension() int {
	return v.dimension
}

func (v *CohereVectorizer) Name() string {
	return "cohere/" + v.model
}

// HuggingFaceVectorizer uses HuggingFace Inference API
type HuggingFaceVectorizer struct {
	client     *http.Client
	apiKey     string
	model      string
	dimension  int
	endpoint   string
	batchSize  int
	retryCount int
}

// NewHuggingFaceVectorizer creates a HuggingFace vectorizer
func NewHuggingFaceVectorizer(cfg *VectorizerConfig) (*HuggingFaceVectorizer, error) {
	model := cfg.Model
	if model == "" {
		model = "sentence-transformers/all-MiniLM-L6-v2"
	}

	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://api-inference.huggingface.co/pipeline/feature-extraction/%s", model)
	}

	dimension := cfg.Dimension
	if dimension == 0 {
		dimension = 384 // all-MiniLM-L6-v2 default
	}

	return &HuggingFaceVectorizer{
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		apiKey:     cfg.APIKey,
		model:      model,
		dimension:  dimension,
		endpoint:   endpoint,
		batchSize:  cfg.BatchSize,
		retryCount: cfg.RetryCount,
	}, nil
}

func (v *HuggingFaceVectorizer) Vectorize(ctx context.Context, text string) (point.Vector, error) {
	vectors, err := v.VectorizeBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, errors.New("no embedding returned")
	}
	return vectors[0], nil
}

func (v *HuggingFaceVectorizer) VectorizeBatch(ctx context.Context, texts []string) ([]point.Vector, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := map[string]interface{}{
		"inputs": texts,
		"options": map[string]bool{
			"wait_for_model": true,
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", v.endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if v.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+v.apiKey)
	}

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// HuggingFace returns array of arrays for batch
	var embeddings [][]float64
	if err := json.Unmarshal(body, &embeddings); err != nil {
		// Try single response format
		var single []float64
		if err2 := json.Unmarshal(body, &single); err2 != nil {
			return nil, fmt.Errorf("failed to parse response: %s", string(body))
		}
		embeddings = [][]float64{single}
	}

	vectors := make([]point.Vector, len(embeddings))
	for i, emb := range embeddings {
		vec := make(point.Vector, len(emb))
		for j, val := range emb {
			vec[j] = float32(val)
		}
		vectors[i] = vec
	}

	return vectors, nil
}

func (v *HuggingFaceVectorizer) Dimension() int {
	return v.dimension
}

func (v *HuggingFaceVectorizer) Name() string {
	return "huggingface/" + v.model
}

// LocalVectorizer uses a local embedding server (e.g., ollama, llama.cpp)
type LocalVectorizer struct {
	client    *http.Client
	endpoint  string
	model     string
	dimension int
	batchSize int
}

// NewLocalVectorizer creates a local vectorizer
func NewLocalVectorizer(cfg *VectorizerConfig) (*LocalVectorizer, error) {
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = "http://localhost:11434/api/embeddings" // Ollama default
	}

	return &LocalVectorizer{
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		endpoint:  endpoint,
		model:     cfg.Model,
		dimension: cfg.Dimension,
		batchSize: cfg.BatchSize,
	}, nil
}

func (v *LocalVectorizer) Vectorize(ctx context.Context, text string) (point.Vector, error) {
	reqBody := map[string]interface{}{
		"model":  v.model,
		"prompt": text,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", v.endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Embedding []float64 `json:"embedding"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	vec := make(point.Vector, len(result.Embedding))
	for i, val := range result.Embedding {
		vec[i] = float32(val)
	}

	return vec, nil
}

func (v *LocalVectorizer) VectorizeBatch(ctx context.Context, texts []string) ([]point.Vector, error) {
	vectors := make([]point.Vector, len(texts))

	for i, text := range texts {
		vec, err := v.Vectorize(ctx, text)
		if err != nil {
			return nil, err
		}
		vectors[i] = vec
	}

	return vectors, nil
}

func (v *LocalVectorizer) Dimension() int {
	return v.dimension
}

func (v *LocalVectorizer) Name() string {
	return "local/" + v.model
}

// CustomVectorizer uses a custom HTTP endpoint
type CustomVectorizer struct {
	client    *http.Client
	endpoint  string
	dimension int
	headers   map[string]string
}

// NewCustomVectorizer creates a custom vectorizer
func NewCustomVectorizer(cfg *VectorizerConfig) (*CustomVectorizer, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("endpoint is required for custom vectorizer")
	}

	return &CustomVectorizer{
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		endpoint:  cfg.Endpoint,
		dimension: cfg.Dimension,
		headers:   make(map[string]string),
	}, nil
}

func (v *CustomVectorizer) Vectorize(ctx context.Context, text string) (point.Vector, error) {
	vectors, err := v.VectorizeBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, errors.New("no embedding returned")
	}
	return vectors[0], nil
}

func (v *CustomVectorizer) VectorizeBatch(ctx context.Context, texts []string) ([]point.Vector, error) {
	reqBody := map[string]interface{}{
		"texts": texts,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", v.endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	for k, val := range v.headers {
		req.Header.Set(k, val)
	}

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Embeddings [][]float64 `json:"embeddings"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	vectors := make([]point.Vector, len(result.Embeddings))
	for i, emb := range result.Embeddings {
		vec := make(point.Vector, len(emb))
		for j, val := range emb {
			vec[j] = float32(val)
		}
		vectors[i] = vec
	}

	return vectors, nil
}

func (v *CustomVectorizer) Dimension() int {
	return v.dimension
}

func (v *CustomVectorizer) Name() string {
	return "custom"
}

// SetHeader sets a custom header for requests
func (v *CustomVectorizer) SetHeader(key, value string) {
	v.headers[key] = value
}
