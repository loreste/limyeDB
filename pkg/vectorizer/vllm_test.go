package vectorizer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// vllmEmbeddingServer returns a fake OpenAI-compatible embeddings server and
// records what the client sent.
func vllmEmbeddingServer(t *testing.T, dim int, seen *openAIRequest, authHeader *string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authHeader != nil {
			*authHeader = r.Header.Get("Authorization")
		}

		var req openAIRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if seen != nil {
			*seen = req
		}

		inputs, ok := req.Input.([]interface{})
		if !ok {
			t.Errorf("input is %T, want a list", req.Input)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		resp := openAIResponse{}
		for i := range inputs {
			emb := make([]float64, dim)
			for j := range emb {
				emb[j] = float64(i+1) / float64(j+1)
			}
			resp.Data = append(resp.Data, struct {
				Embedding []float64 `json:"embedding"`
				Index     int       `json:"index"`
			}{Embedding: emb, Index: i})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func TestVLLMVectorizerEmbedsText(t *testing.T) {
	var seen openAIRequest
	srv := vllmEmbeddingServer(t, 4, &seen, nil)
	defer srv.Close()

	v, err := NewVLLMVectorizer(&VectorizerConfig{
		Model:     "intfloat/e5-small",
		Endpoint:  srv.URL,
		Dimension: 4,
	})
	if err != nil {
		t.Fatalf("NewVLLMVectorizer: %v", err)
	}

	vec, err := v.Vectorize(context.Background(), "hello vllm")
	if err != nil {
		t.Fatalf("Vectorize: %v", err)
	}
	if len(vec) != 4 {
		t.Errorf("vector length = %d, want 4", len(vec))
	}
	if seen.Model != "intfloat/e5-small" {
		t.Errorf("request model = %q, want the configured model", seen.Model)
	}
	if got := v.Name(); got != "vllm/intfloat/e5-small" {
		t.Errorf("Name() = %q, want it to report vllm, not openai", got)
	}
}

// TestVLLMVectorizerSendsNoAuthHeaderWithoutKey is the reason vLLM needs its own
// constructor: NewOpenAIVectorizer rejects an empty key, and some self-hosted
// servers reject an unexpected bearer token.
func TestVLLMVectorizerSendsNoAuthHeaderWithoutKey(t *testing.T) {
	var auth string
	srv := vllmEmbeddingServer(t, 3, nil, &auth)
	defer srv.Close()

	v, err := NewVLLMVectorizer(&VectorizerConfig{
		Model:     "e5",
		Endpoint:  srv.URL,
		Dimension: 3,
	})
	if err != nil {
		t.Fatalf("NewVLLMVectorizer: %v", err)
	}

	if _, err := v.Vectorize(context.Background(), "no auth please"); err != nil {
		t.Fatalf("Vectorize: %v", err)
	}
	if auth != "" {
		t.Errorf("Authorization header = %q, want it omitted when no key is set", auth)
	}
}

// TestVLLMVectorizerSendsAuthHeaderWithKey covers a gateway-fronted deployment.
func TestVLLMVectorizerSendsAuthHeaderWithKey(t *testing.T) {
	var auth string
	srv := vllmEmbeddingServer(t, 3, nil, &auth)
	defer srv.Close()

	v, _ := NewVLLMVectorizer(&VectorizerConfig{
		Model:     "e5",
		Endpoint:  srv.URL,
		APIKey:    "gateway-token",
		Dimension: 3,
	})

	if _, err := v.Vectorize(context.Background(), "with auth"); err != nil {
		t.Fatalf("Vectorize: %v", err)
	}
	if auth != "Bearer gateway-token" {
		t.Errorf("Authorization = %q, want \"Bearer gateway-token\"", auth)
	}
}

func TestVLLMVectorizerBatch(t *testing.T) {
	srv := vllmEmbeddingServer(t, 2, nil, nil)
	defer srv.Close()

	v, _ := NewVLLMVectorizer(&VectorizerConfig{
		Model:     "e5",
		Endpoint:  srv.URL,
		Dimension: 2,
		BatchSize: 2,
	})

	texts := []string{"a", "b", "c", "d", "e"}
	vecs, err := v.VectorizeBatch(context.Background(), texts)
	if err != nil {
		t.Fatalf("VectorizeBatch: %v", err)
	}
	if len(vecs) != len(texts) {
		t.Errorf("got %d vectors for %d inputs, want one each", len(vecs), len(texts))
	}
}

// TestVLLMEndpointNormalization covers operators configuring a server root
// rather than the full embeddings path. Posting to the root would 404.
func TestVLLMEndpointNormalization(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"server root", "http://host:8000", "http://host:8000/v1/embeddings"},
		{"trailing slash", "http://host:8000/", "http://host:8000/v1/embeddings"},
		{"v1 only", "http://host:8000/v1", "http://host:8000/v1/embeddings"},
		{"full path", "http://host:8000/v1/embeddings", "http://host:8000/v1/embeddings"},
		{"full path slash", "http://host:8000/v1/embeddings/", "http://host:8000/v1/embeddings"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeVLLMEndpoint(tt.in); got != tt.want {
				t.Errorf("normalizeVLLMEndpoint(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestVLLMVectorizerDefaultEndpoint(t *testing.T) {
	v, err := NewVLLMVectorizer(&VectorizerConfig{Model: "e5"})
	if err != nil {
		t.Fatalf("NewVLLMVectorizer: %v", err)
	}
	if v.endpoint != defaultVLLMEndpoint {
		t.Errorf("endpoint = %q, want %q", v.endpoint, defaultVLLMEndpoint)
	}
}

func TestVLLMVectorizerValidation(t *testing.T) {
	if _, err := NewVLLMVectorizer(nil); err == nil {
		t.Error("nil config succeeded, want error")
	}
	if _, err := NewVLLMVectorizer(&VectorizerConfig{}); err == nil {
		t.Error("missing model succeeded, want error")
	}
}

// TestVLLMRegistersThroughFactory confirms the type is reachable from config,
// which is how an operator actually selects it.
func TestVLLMRegistersThroughFactory(t *testing.T) {
	mgr := NewVectorizerManager()

	err := mgr.Register("vllm-local", &VectorizerConfig{
		Type:      VectorizerVLLM,
		Model:     "e5",
		Dimension: 384,
	})
	if err != nil {
		t.Fatalf("Register vllm vectorizer: %v", err)
	}

	v, ok := mgr.Get("vllm-local")
	if !ok {
		t.Fatal("Get(\"vllm-local\") not found after Register")
	}
	if got := v.Name(); got != "vllm/e5" {
		t.Errorf("Name() = %q, want \"vllm/e5\"", got)
	}
}

// TestOpenAIStillRequiresAPIKey guards the hosted path: making the key optional
// for vLLM must not let a misconfigured OpenAI vectorizer through silently.
func TestOpenAIStillRequiresAPIKey(t *testing.T) {
	if _, err := NewOpenAIVectorizer(&VectorizerConfig{Model: "text-embedding-3-small"}); err == nil {
		t.Error("OpenAI vectorizer with no API key succeeded, want error")
	}
}
