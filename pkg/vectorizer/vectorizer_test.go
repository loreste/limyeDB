package vectorizer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/limyedb/limyedb/pkg/point"
)

func TestDefaultVectorizerConfig(t *testing.T) {
	cfg := DefaultVectorizerConfig()

	if cfg.Type != VectorizerOpenAI {
		t.Errorf("Type = %s, want %s", cfg.Type, VectorizerOpenAI)
	}
	if cfg.Model != "text-embedding-3-small" {
		t.Errorf("Model = %s, want text-embedding-3-small", cfg.Model)
	}
	if cfg.Dimension != 1536 {
		t.Errorf("Dimension = %d, want 1536", cfg.Dimension)
	}
	if cfg.BatchSize != 100 {
		t.Errorf("BatchSize = %d, want 100", cfg.BatchSize)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", cfg.Timeout)
	}
	if cfg.RetryCount != 3 {
		t.Errorf("RetryCount = %d, want 3", cfg.RetryCount)
	}
}

func TestVectorizerManager(t *testing.T) {
	m := NewVectorizerManager()
	if m == nil {
		t.Fatal("NewVectorizerManager returned nil")
	}

	// Test empty list
	names := m.List()
	if len(names) != 0 {
		t.Errorf("List() on empty manager = %d, want 0", len(names))
	}

	// Test Get on non-existent
	_, ok := m.Get("nonexistent")
	if ok {
		t.Error("Get(nonexistent) should return false")
	}
}

func TestVectorizerManagerRegister(t *testing.T) {
	m := NewVectorizerManager()

	// Register a custom vectorizer (doesn't need API key)
	cfg := &VectorizerConfig{
		Type:      VectorizerCustom,
		Endpoint:  "http://localhost:8080/embed",
		Dimension: 512,
		Timeout:   10 * time.Second,
	}

	err := m.Register("test-vectorizer", cfg)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Verify it's registered
	v, ok := m.Get("test-vectorizer")
	if !ok {
		t.Fatal("Get() should return true after Register")
	}
	if v.Dimension() != 512 {
		t.Errorf("Dimension() = %d, want 512", v.Dimension())
	}
	if v.Name() != "custom" {
		t.Errorf("Name() = %s, want custom", v.Name())
	}

	// Verify it's in the list
	names := m.List()
	if len(names) != 1 {
		t.Errorf("List() = %d, want 1", len(names))
	}

	// Delete it
	m.Delete("test-vectorizer")
	_, ok = m.Get("test-vectorizer")
	if ok {
		t.Error("Get() should return false after Delete")
	}
}

func TestVectorizerManagerRegisterErrors(t *testing.T) {
	m := NewVectorizerManager()

	tests := []struct {
		name string
		cfg  *VectorizerConfig
	}{
		{
			name: "openai_missing_api_key",
			cfg: &VectorizerConfig{
				Type:    VectorizerOpenAI,
				Timeout: 10 * time.Second,
			},
		},
		{
			name: "cohere_missing_api_key",
			cfg: &VectorizerConfig{
				Type:    VectorizerCohere,
				Timeout: 10 * time.Second,
			},
		},
		{
			name: "custom_missing_endpoint",
			cfg: &VectorizerConfig{
				Type:    VectorizerCustom,
				Timeout: 10 * time.Second,
			},
		},
		{
			name: "unknown_type",
			cfg: &VectorizerConfig{
				Type:    "unknown",
				Timeout: 10 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.Register(tt.name, tt.cfg)
			if err == nil {
				t.Error("Register() expected error, got nil")
			}
		})
	}
}

func TestNewOpenAIVectorizer(t *testing.T) {
	cfg := &VectorizerConfig{
		Type:       VectorizerOpenAI,
		APIKey:     "test-api-key",
		Model:      "text-embedding-3-large",
		Dimension:  3072,
		Timeout:    15 * time.Second,
		BatchSize:  50,
		RetryCount: 2,
	}

	v, err := NewOpenAIVectorizer(cfg)
	if err != nil {
		t.Fatalf("NewOpenAIVectorizer() error = %v", err)
	}

	if v.Dimension() != 3072 {
		t.Errorf("Dimension() = %d, want 3072", v.Dimension())
	}
	if v.Name() != "openai/text-embedding-3-large" {
		t.Errorf("Name() = %s, want openai/text-embedding-3-large", v.Name())
	}
}

func TestNewOpenAIVectorizerDefaults(t *testing.T) {
	tests := []struct {
		model         string
		wantDimension int
	}{
		{"text-embedding-3-small", 1536},
		{"text-embedding-3-large", 3072},
		{"text-embedding-ada-002", 1536},
		{"unknown-model", 1536}, // defaults to 1536
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			cfg := &VectorizerConfig{
				Type:    VectorizerOpenAI,
				APIKey:  "test-key",
				Model:   tt.model,
				Timeout: 10 * time.Second,
			}

			v, err := NewOpenAIVectorizer(cfg)
			if err != nil {
				t.Fatalf("NewOpenAIVectorizer() error = %v", err)
			}

			if v.Dimension() != tt.wantDimension {
				t.Errorf("Dimension() = %d, want %d", v.Dimension(), tt.wantDimension)
			}
		})
	}
}

func TestNewOpenAIVectorizerMissingAPIKey(t *testing.T) {
	cfg := &VectorizerConfig{
		Type:    VectorizerOpenAI,
		Timeout: 10 * time.Second,
	}

	_, err := NewOpenAIVectorizer(cfg)
	if err == nil {
		t.Error("NewOpenAIVectorizer() expected error for missing API key")
	}
}

func TestNewCohereVectorizer(t *testing.T) {
	cfg := &VectorizerConfig{
		Type:      VectorizerCohere,
		APIKey:    "test-api-key",
		Model:     "embed-multilingual-v3.0",
		Dimension: 1024,
		Timeout:   15 * time.Second,
	}

	v, err := NewCohereVectorizer(cfg)
	if err != nil {
		t.Fatalf("NewCohereVectorizer() error = %v", err)
	}

	if v.Dimension() != 1024 {
		t.Errorf("Dimension() = %d, want 1024", v.Dimension())
	}
	if v.Name() != "cohere/embed-multilingual-v3.0" {
		t.Errorf("Name() = %s, want cohere/embed-multilingual-v3.0", v.Name())
	}
}

func TestNewCohereVectorizerDefaults(t *testing.T) {
	cfg := &VectorizerConfig{
		Type:    VectorizerCohere,
		APIKey:  "test-key",
		Timeout: 10 * time.Second,
	}

	v, err := NewCohereVectorizer(cfg)
	if err != nil {
		t.Fatalf("NewCohereVectorizer() error = %v", err)
	}

	if v.model != "embed-english-v3.0" {
		t.Errorf("model = %s, want embed-english-v3.0", v.model)
	}
	if v.Dimension() != 1024 {
		t.Errorf("Dimension() = %d, want 1024", v.Dimension())
	}
}

func TestNewHuggingFaceVectorizer(t *testing.T) {
	cfg := &VectorizerConfig{
		Type:      VectorizerHuggingFace,
		APIKey:    "test-api-key",
		Model:     "sentence-transformers/all-mpnet-base-v2",
		Dimension: 768,
		Timeout:   15 * time.Second,
	}

	v, err := NewHuggingFaceVectorizer(cfg)
	if err != nil {
		t.Fatalf("NewHuggingFaceVectorizer() error = %v", err)
	}

	if v.Dimension() != 768 {
		t.Errorf("Dimension() = %d, want 768", v.Dimension())
	}
	if v.Name() != "huggingface/sentence-transformers/all-mpnet-base-v2" {
		t.Errorf("Name() = %s", v.Name())
	}
}

func TestNewHuggingFaceVectorizerDefaults(t *testing.T) {
	cfg := &VectorizerConfig{
		Type:    VectorizerHuggingFace,
		Timeout: 10 * time.Second,
	}

	v, err := NewHuggingFaceVectorizer(cfg)
	if err != nil {
		t.Fatalf("NewHuggingFaceVectorizer() error = %v", err)
	}

	if v.model != "sentence-transformers/all-MiniLM-L6-v2" {
		t.Errorf("model = %s, want sentence-transformers/all-MiniLM-L6-v2", v.model)
	}
	if v.Dimension() != 384 {
		t.Errorf("Dimension() = %d, want 384", v.Dimension())
	}
}

func TestNewLocalVectorizer(t *testing.T) {
	cfg := &VectorizerConfig{
		Type:      VectorizerLocal,
		Model:     "nomic-embed-text",
		Endpoint:  "http://localhost:11434/api/embeddings",
		Dimension: 768,
		Timeout:   10 * time.Second,
	}

	v, err := NewLocalVectorizer(cfg)
	if err != nil {
		t.Fatalf("NewLocalVectorizer() error = %v", err)
	}

	if v.Dimension() != 768 {
		t.Errorf("Dimension() = %d, want 768", v.Dimension())
	}
	if v.Name() != "local/nomic-embed-text" {
		t.Errorf("Name() = %s, want local/nomic-embed-text", v.Name())
	}
}

func TestNewLocalVectorizerDefaults(t *testing.T) {
	cfg := &VectorizerConfig{
		Type:    VectorizerLocal,
		Timeout: 10 * time.Second,
	}

	v, err := NewLocalVectorizer(cfg)
	if err != nil {
		t.Fatalf("NewLocalVectorizer() error = %v", err)
	}

	if v.endpoint != "http://localhost:11434/api/embeddings" {
		t.Errorf("endpoint = %s, want default Ollama endpoint", v.endpoint)
	}
}

func TestNewCustomVectorizer(t *testing.T) {
	cfg := &VectorizerConfig{
		Type:      VectorizerCustom,
		Endpoint:  "http://my-embedder:8080/embed",
		Dimension: 256,
		Timeout:   10 * time.Second,
	}

	v, err := NewCustomVectorizer(cfg)
	if err != nil {
		t.Fatalf("NewCustomVectorizer() error = %v", err)
	}

	if v.Dimension() != 256 {
		t.Errorf("Dimension() = %d, want 256", v.Dimension())
	}
	if v.Name() != "custom" {
		t.Errorf("Name() = %s, want custom", v.Name())
	}
}

func TestNewCustomVectorizerMissingEndpoint(t *testing.T) {
	cfg := &VectorizerConfig{
		Type:    VectorizerCustom,
		Timeout: 10 * time.Second,
	}

	_, err := NewCustomVectorizer(cfg)
	if err == nil {
		t.Error("NewCustomVectorizer() expected error for missing endpoint")
	}
}

func TestCustomVectorizerSetHeader(t *testing.T) {
	cfg := &VectorizerConfig{
		Type:      VectorizerCustom,
		Endpoint:  "http://localhost:8080/embed",
		Dimension: 256,
		Timeout:   10 * time.Second,
	}

	v, err := NewCustomVectorizer(cfg)
	if err != nil {
		t.Fatalf("NewCustomVectorizer() error = %v", err)
	}

	v.SetHeader("X-API-Key", "secret")
	v.SetHeader("X-Custom", "value")

	if v.headers["X-API-Key"] != "secret" {
		t.Errorf("headers[X-API-Key] = %s, want secret", v.headers["X-API-Key"])
	}
	if v.headers["X-Custom"] != "value" {
		t.Errorf("headers[X-Custom] = %s, want value", v.headers["X-Custom"])
	}
}

func TestOpenAIVectorizerVectorize(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Method = %s, want POST", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("Authorization header incorrect")
		}

		var req openAIRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Return mock embedding
		resp := openAIResponse{
			Data: []struct {
				Embedding []float64 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{
					Embedding: make([]float64, 1536),
					Index:     0,
				},
			},
		}
		for i := range resp.Data[0].Embedding {
			resp.Data[0].Embedding[i] = float64(i) * 0.001
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	v := &OpenAIVectorizer{
		client:     &http.Client{Timeout: 5 * time.Second},
		apiKey:     "test-key",
		model:      "text-embedding-3-small",
		dimension:  1536,
		endpoint:   server.URL,
		batchSize:  100,
		retryCount: 0,
	}

	ctx := context.Background()
	vec, err := v.Vectorize(ctx, "test text")
	if err != nil {
		t.Fatalf("Vectorize() error = %v", err)
	}

	if len(vec) != 1536 {
		t.Errorf("len(vec) = %d, want 1536", len(vec))
	}
}

func TestOpenAIVectorizerVectorizeBatchEmpty(t *testing.T) {
	v := &OpenAIVectorizer{
		client:    &http.Client{},
		apiKey:    "test-key",
		model:     "text-embedding-3-small",
		dimension: 1536,
		endpoint:  "http://localhost",
	}

	ctx := context.Background()
	vecs, err := v.VectorizeBatch(ctx, []string{})
	if err != nil {
		t.Fatalf("VectorizeBatch() error = %v", err)
	}
	if vecs != nil {
		t.Errorf("VectorizeBatch(empty) = %v, want nil", vecs)
	}
}

func TestOpenAIVectorizerAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openAIResponse{
			Error: &struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			}{
				Message: "Invalid API key",
				Type:    "invalid_request_error",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	v := &OpenAIVectorizer{
		client:     &http.Client{Timeout: 5 * time.Second},
		apiKey:     "invalid-key",
		model:      "text-embedding-3-small",
		dimension:  1536,
		endpoint:   server.URL,
		retryCount: 0,
	}

	ctx := context.Background()
	_, err := v.Vectorize(ctx, "test")
	if err == nil {
		t.Error("Vectorize() expected error for API error response")
	}
}

func TestCohereVectorizerVectorizeBatchEmpty(t *testing.T) {
	v := &CohereVectorizer{
		client:    &http.Client{},
		apiKey:    "test-key",
		model:     "embed-english-v3.0",
		dimension: 1024,
		endpoint:  "http://localhost",
	}

	ctx := context.Background()
	vecs, err := v.VectorizeBatch(ctx, []string{})
	if err != nil {
		t.Fatalf("VectorizeBatch() error = %v", err)
	}
	if vecs != nil {
		t.Errorf("VectorizeBatch(empty) = %v, want nil", vecs)
	}
}

func TestHuggingFaceVectorizerVectorizeBatchEmpty(t *testing.T) {
	v := &HuggingFaceVectorizer{
		client:    &http.Client{},
		model:     "all-MiniLM-L6-v2",
		dimension: 384,
		endpoint:  "http://localhost",
	}

	ctx := context.Background()
	vecs, err := v.VectorizeBatch(ctx, []string{})
	if err != nil {
		t.Fatalf("VectorizeBatch() error = %v", err)
	}
	if vecs != nil {
		t.Errorf("VectorizeBatch(empty) = %v, want nil", vecs)
	}
}

func TestVectorizerConfigJSON(t *testing.T) {
	cfg := &VectorizerConfig{
		Type:       VectorizerOpenAI,
		Model:      "text-embedding-3-small",
		APIKey:     "secret-key",
		Endpoint:   "https://api.openai.com/v1/embeddings",
		Dimension:  1536,
		BatchSize:  100,
		Timeout:    30 * time.Second,
		RetryCount: 3,
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded VectorizerConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.Type != cfg.Type {
		t.Errorf("Type = %s, want %s", decoded.Type, cfg.Type)
	}
	if decoded.Model != cfg.Model {
		t.Errorf("Model = %s, want %s", decoded.Model, cfg.Model)
	}
	if decoded.Dimension != cfg.Dimension {
		t.Errorf("Dimension = %d, want %d", decoded.Dimension, cfg.Dimension)
	}
}

func TestVectorizerTypes(t *testing.T) {
	tests := []struct {
		vType    VectorizerType
		expected string
	}{
		{VectorizerOpenAI, "openai"},
		{VectorizerCohere, "cohere"},
		{VectorizerHuggingFace, "huggingface"},
		{VectorizerLocal, "local"},
		{VectorizerCustom, "custom"},
	}

	for _, tt := range tests {
		if string(tt.vType) != tt.expected {
			t.Errorf("VectorizerType = %s, want %s", tt.vType, tt.expected)
		}
	}
}

// mockVectorizer implements Vectorizer for testing
type mockVectorizer struct {
	name      string
	dimension int
	err       error
}

func (m *mockVectorizer) Vectorize(ctx context.Context, text string) (point.Vector, error) {
	if m.err != nil {
		return nil, m.err
	}
	return make(point.Vector, m.dimension), nil
}

func (m *mockVectorizer) VectorizeBatch(ctx context.Context, texts []string) ([]point.Vector, error) {
	if m.err != nil {
		return nil, m.err
	}
	vecs := make([]point.Vector, len(texts))
	for i := range texts {
		vecs[i] = make(point.Vector, m.dimension)
	}
	return vecs, nil
}

func (m *mockVectorizer) Dimension() int {
	return m.dimension
}

func (m *mockVectorizer) Name() string {
	return m.name
}

func TestVectorizerInterface(t *testing.T) {
	mock := &mockVectorizer{
		name:      "mock",
		dimension: 128,
	}

	var v Vectorizer = mock

	if v.Dimension() != 128 {
		t.Errorf("Dimension() = %d, want 128", v.Dimension())
	}
	if v.Name() != "mock" {
		t.Errorf("Name() = %s, want mock", v.Name())
	}

	ctx := context.Background()
	vec, err := v.Vectorize(ctx, "test")
	if err != nil {
		t.Fatalf("Vectorize() error = %v", err)
	}
	if len(vec) != 128 {
		t.Errorf("len(vec) = %d, want 128", len(vec))
	}
}

func TestVectorizerInterfaceError(t *testing.T) {
	mock := &mockVectorizer{
		name:      "mock",
		dimension: 128,
		err:       errors.New("mock error"),
	}

	ctx := context.Background()
	_, err := mock.Vectorize(ctx, "test")
	if err == nil {
		t.Error("Vectorize() expected error")
	}

	_, err = mock.VectorizeBatch(ctx, []string{"test1", "test2"})
	if err == nil {
		t.Error("VectorizeBatch() expected error")
	}
}

func BenchmarkOpenAIVectorizer(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openAIResponse{
			Data: []struct {
				Embedding []float64 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Embedding: make([]float64, 1536), Index: 0},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	v := &OpenAIVectorizer{
		client:     &http.Client{Timeout: 5 * time.Second},
		apiKey:     "test-key",
		model:      "text-embedding-3-small",
		dimension:  1536,
		endpoint:   server.URL,
		retryCount: 0,
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = v.Vectorize(ctx, "benchmark test text")
	}
}

func BenchmarkVectorizerManagerGet(b *testing.B) {
	m := NewVectorizerManager()
	m.vectorizers["test"] = &mockVectorizer{name: "test", dimension: 128}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.Get("test")
	}
}
