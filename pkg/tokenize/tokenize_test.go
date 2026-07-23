package tokenize

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEstimatorEmptyAndMinimum(t *testing.T) {
	e := NewEstimator(DefaultEstimatorConfig())

	if got := e.Estimate(""); got != 0 {
		t.Errorf("Estimate(\"\") = %d, want 0", got)
	}
	if got := e.Estimate("a"); got < 1 {
		t.Errorf("Estimate(\"a\") = %d, want at least 1", got)
	}
}

// TestEstimatorContentDensity is the reason this is not length/4: token density
// varies sharply by content, and a flat ratio under-counts dense text.
func TestEstimatorContentDensity(t *testing.T) {
	e := NewEstimator(DefaultEstimatorConfig())

	prose := "the quick brown fox jumps over the lazy dog and keeps running"
	// Same rune count, but CJK costs far more tokens.
	cjk := strings.Repeat("字", len([]rune(prose)))

	proseTokens := e.Estimate(prose)
	cjkTokens := e.Estimate(cjk)

	if cjkTokens <= proseTokens {
		t.Errorf("CJK estimate %d should exceed Latin estimate %d for equal rune counts",
			cjkTokens, proseTokens)
	}

	// Punctuation-dense input should also cost more than plain prose of the
	// same length, since symbols rarely merge into multi-character tokens.
	symbols := strings.Repeat("{}", len([]rune(prose))/2)
	if got := e.Estimate(symbols); got <= proseTokens {
		t.Errorf("symbol-dense estimate %d should exceed prose estimate %d", got, proseTokens)
	}
}

func TestEstimatorIsConservative(t *testing.T) {
	// A safety margin above 1 must produce a count no lower than the raw ratio,
	// since under-counting overflows a real context window.
	text := strings.Repeat("token budgeting matters ", 40)

	plain := NewEstimator(EstimatorConfig{CharsPerToken: 4, SafetyMargin: 1.0})
	margined := NewEstimator(DefaultEstimatorConfig())

	if margined.Estimate(text) < plain.Estimate(text) {
		t.Errorf("margined estimate %d < unmargined %d, want conservative",
			margined.Estimate(text), plain.Estimate(text))
	}
}

func TestEstimatorConfigFallbacks(t *testing.T) {
	// A zero-valued config must not produce a zero or negative estimate.
	e := NewEstimator(EstimatorConfig{})
	if got := e.Estimate("some reasonable sentence here"); got <= 0 {
		t.Errorf("estimate with empty config = %d, want > 0", got)
	}
}

func TestEstimatorImplementsTokenizer(t *testing.T) {
	var tk Tokenizer = NewEstimator(DefaultEstimatorConfig())

	got, err := tk.Count(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Count returned error: %v", err)
	}
	if got.Tokens <= 0 {
		t.Errorf("Count.Tokens = %d, want > 0", got.Tokens)
	}
	if got.Exact {
		t.Error("Count.Exact = true, want false for an estimator")
	}
	if tk.Name() == "" {
		t.Error("Name() is empty")
	}
}

func TestRegistryUnknownModel(t *testing.T) {
	r := NewRegistry()

	if _, err := r.ContextWindow("nonexistent"); !errors.Is(err, ErrUnknownModel) {
		t.Errorf("ContextWindow error = %v, want ErrUnknownModel", err)
	}
}

func TestRegistryRegisterValidation(t *testing.T) {
	r := NewRegistry()

	if err := r.Register(ModelInfo{Name: "", ContextWindow: 1000}); err == nil {
		t.Error("Register with empty name succeeded, want error")
	}
	if err := r.Register(ModelInfo{Name: "m", ContextWindow: 0}); err == nil {
		t.Error("Register with zero window succeeded, want error")
	}
	if err := r.Register(ModelInfo{Name: "m", ContextWindow: 4096}); err != nil {
		t.Fatalf("Register valid model: %v", err)
	}

	got, err := r.ContextWindow("m")
	if err != nil {
		t.Fatalf("ContextWindow: %v", err)
	}
	if got != 4096 {
		t.Errorf("ContextWindow = %d, want 4096", got)
	}
}

func TestRegistryBudget(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(ModelInfo{Name: "local/qwen", ContextWindow: 8000}); err != nil {
		t.Fatal(err)
	}

	// Explicit budget wins even for an unregistered model, so an operator is
	// never blocked by a missing registry entry.
	got, err := r.Budget("totally-unknown", 1234, 0)
	if err != nil {
		t.Fatalf("explicit budget: %v", err)
	}
	if got != 1234 {
		t.Errorf("Budget explicit = %d, want 1234", got)
	}

	// Fraction of a known window.
	got, err = r.Budget("local/qwen", 0, 0.25)
	if err != nil {
		t.Fatalf("fractional budget: %v", err)
	}
	if got != 2000 {
		t.Errorf("Budget fraction = %d, want 2000", got)
	}

	// Out-of-range fraction falls back to half the window.
	got, err = r.Budget("local/qwen", 0, 5)
	if err != nil {
		t.Fatalf("clamped budget: %v", err)
	}
	if got != 4000 {
		t.Errorf("Budget clamped = %d, want 4000", got)
	}

	// No explicit budget and no registration must fail loudly rather than
	// guessing a window.
	if _, err := r.Budget("unregistered", 0, 0.5); !errors.Is(err, ErrUnknownModel) {
		t.Errorf("Budget error = %v, want ErrUnknownModel", err)
	}
}

func TestVLLMTokenizerExactCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tokenize" {
			t.Errorf("path = %q, want /tokenize", r.URL.Path)
		}
		var req vllmTokenizeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if req.Prompt == "" {
			t.Error("prompt is empty")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(vllmTokenizeResponse{Count: 42})
	}))
	defer srv.Close()

	tk, err := NewVLLMTokenizer(VLLMConfig{BaseURL: srv.URL, Model: "qwen"})
	if err != nil {
		t.Fatalf("NewVLLMTokenizer: %v", err)
	}

	got, err := tk.Count(context.Background(), "count me")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if got.Tokens != 42 {
		t.Errorf("Count.Tokens = %d, want 42 from the server", got.Tokens)
	}
	if !got.Exact {
		t.Error("Count.Exact = false, want true for a successful vLLM count")
	}
}

// TestVLLMTokenizerFallsBackToTokenArray covers builds that return only tokens.
func TestVLLMTokenizerFallsBackToTokenArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(vllmTokenizeResponse{Tokens: []int{1, 2, 3, 4, 5}})
	}))
	defer srv.Close()

	tk, _ := NewVLLMTokenizer(VLLMConfig{BaseURL: srv.URL, Model: "qwen"})
	got, err := tk.Count(context.Background(), "five tokens please")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if got.Tokens != 5 {
		t.Errorf("Count.Tokens = %d, want 5 from token array length", got.Tokens)
	}
	if !got.Exact {
		t.Error("Count.Exact = false, want true when the server answered")
	}
}

// TestVLLMTokenizerDegradesOnFailure asserts a tokenizer outage does not fail
// the caller: budgeting continues with an estimate, and the result says so.
// Reporting Exact: true here would invite a caller to skip its headroom and
// overflow the very window this package exists to protect.
func TestVLLMTokenizerDegradesOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tk, _ := NewVLLMTokenizer(VLLMConfig{BaseURL: srv.URL, Model: "qwen"})

	got, err := tk.Count(context.Background(), "still needs a number")
	if err != nil {
		t.Fatalf("Count returned error, want graceful degradation: %v", err)
	}
	if got.Tokens <= 0 {
		t.Errorf("Count.Tokens = %d, want a positive estimate on fallback", got.Tokens)
	}
	if got.Exact {
		t.Error("Count.Exact = true after a server failure, want false; a fallback estimate must not claim to be exact")
	}
}

func TestVLLMTokenizerRequiresBaseURL(t *testing.T) {
	if _, err := NewVLLMTokenizer(VLLMConfig{}); err == nil {
		t.Error("NewVLLMTokenizer with no base URL succeeded, want error")
	}
}

func TestVLLMTokenizerEmptyText(t *testing.T) {
	tk, _ := NewVLLMTokenizer(VLLMConfig{BaseURL: "http://127.0.0.1:1", Model: "m"})

	// Must not make a network call for empty input.
	got, err := tk.Count(context.Background(), "")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if got.Tokens != 0 {
		t.Errorf("Count(\"\").Tokens = %d, want 0", got.Tokens)
	}
}
