// Package tokenize provides token accounting for context-window budgeting.
//
// Retrieval for an LLM is bounded by tokens, not by result count, so every
// candidate chunk needs a token cost before it can be packed into a budget.
// Tokenization is model-specific and there is no single correct answer across
// providers, so this package exposes a Tokenizer interface with two kinds of
// implementation:
//
//   - Exact, where a provider can tell us the true count (vLLM's /tokenize).
//   - Estimated, a deterministic local heuristic used when no such endpoint
//     exists (Ollama, hosted APIs that do not expose a tokenizer).
//
// Estimates are deliberately biased to over-count. Under-counting overflows the
// model's context window, which is a hard failure; over-counting merely leaves
// some of the window unused.
package tokenize

import (
	"context"
	"errors"
	"unicode"
	"unicode/utf8"
)

// ErrUnknownModel is returned when a context window is requested for a model
// that has not been registered.
var ErrUnknownModel = errors.New("tokenize: unknown model, pass an explicit token budget")

// Count is the result of counting tokens.
//
// Exact is reported per call rather than per tokenizer on purpose. A
// provider-backed tokenizer that normally returns authoritative counts may
// degrade to a local estimate when the provider is unreachable, and a caller
// that must not overflow a window has to be able to tell the difference at that
// moment. A static capability flag would claim exactness the value does not have.
type Count struct {
	// Tokens is the token count, exact or estimated.
	Tokens int

	// Exact reports whether this specific count is authoritative. When false,
	// apply headroom before committing the value to a budget.
	Exact bool
}

// Tokenizer reports how many tokens a piece of text will occupy.
type Tokenizer interface {
	// Count returns the token count for text, and whether it is exact.
	Count(ctx context.Context, text string) (Count, error)

	// Name identifies the tokenizer in diagnostics.
	Name() string
}

// EstimatorConfig tunes the heuristic estimator.
type EstimatorConfig struct {
	// CharsPerToken is the assumed density for Latin-script text. Typical
	// BPE tokenizers land near 4 characters per token for prose.
	CharsPerToken float64

	// DigitsPerToken is the assumed density for digit runs. BPE tokenizers
	// split numbers into groups of one to three digits, far denser than prose.
	DigitsPerToken float64

	// SymbolsPerToken is the assumed density for runs of ASCII punctuation.
	// Tokenizers merge common sequences such as ":" and ,"  into single
	// tokens, so charging one token per symbol over-counts structured data
	// like JSON badly.
	SymbolsPerToken float64

	// TokensPerWideRune is the assumed cost of CJK and similar scripts, which
	// most tokenizers split far more finely than Latin text.
	TokensPerWideRune float64

	// SafetyMargin scales the final count. Values above 1 over-count on
	// purpose so a budget is not exceeded.
	SafetyMargin float64
}

// DefaultEstimatorConfig returns conservative defaults.
func DefaultEstimatorConfig() EstimatorConfig {
	return EstimatorConfig{
		CharsPerToken:     4.0,
		DigitsPerToken:    2.0,
		SymbolsPerToken:   2.0,
		TokensPerWideRune: 1.0,
		SafetyMargin:      1.15,
	}
}

// Estimator is a deterministic, offline tokenizer approximation.
//
// It classifies runes into runs rather than dividing the total length, because
// token density varies sharply by content. Only Latin-script prose approaches
// the familiar four-characters-per-token ratio; CJK is near one token per
// character, digits split into groups of one to three, line breaks are tokens
// of their own, and multi-byte symbols cost several byte-level tokens. Dividing
// raw length by four under-counts all of them.
//
// Measured against that naive length/4 ratio, the defaults land at roughly
// 1.05x for prose, 2.0x for Go source, and 2.0x for JSON. Structured data is
// genuinely denser than prose, so exceeding 1.0x there is correct rather than
// pessimistic. The envelope is enforced by tests in estimator_density_test.go
// from both sides: never under-counting, and never so far above that a budget
// wastes most of the window.
type Estimator struct {
	cfg EstimatorConfig
}

// NewEstimator returns an Estimator. Zero or negative fields fall back to the
// defaults so a partially filled config is still usable.
func NewEstimator(cfg EstimatorConfig) *Estimator {
	def := DefaultEstimatorConfig()
	if cfg.CharsPerToken <= 0 {
		cfg.CharsPerToken = def.CharsPerToken
	}
	if cfg.DigitsPerToken <= 0 {
		cfg.DigitsPerToken = def.DigitsPerToken
	}
	if cfg.SymbolsPerToken <= 0 {
		cfg.SymbolsPerToken = def.SymbolsPerToken
	}
	if cfg.TokensPerWideRune <= 0 {
		cfg.TokensPerWideRune = def.TokensPerWideRune
	}
	if cfg.SafetyMargin <= 0 {
		cfg.SafetyMargin = def.SafetyMargin
	}
	return &Estimator{cfg: cfg}
}

// Name implements Tokenizer.
func (e *Estimator) Name() string { return "heuristic-estimator" }

// Count implements Tokenizer. It never returns an error, so it is safe to use
// on paths that must not fail. Estimates are never exact.
func (e *Estimator) Count(_ context.Context, text string) (Count, error) {
	return Count{Tokens: e.Estimate(text), Exact: false}, nil
}

// Estimate returns the approximate token count for text.
func (e *Estimator) Estimate(text string) int {
	if text == "" {
		return 0
	}

	var (
		wide      int     // CJK and similar, ~1 token each
		symbolAcc float64 // line breaks and multi-byte symbols
		letterRun int     // current run of letters
		digitRun  int     // current run of digits
		symbolRun int     // current run of ASCII punctuation
		runCost   float64 // accumulated cost of completed runs
	)

	// runCostFor charges a completed run, with a one-token floor: even a
	// single character occupies a token.
	runCostFor := func(n int, perToken float64) float64 {
		if n == 0 {
			return 0
		}
		if cost := float64(n) / perToken; cost > 1 {
			return cost
		}
		return 1
	}

	// Runs of one class end when a character of another class appears, so each
	// branch closes every run except the one it extends.
	flushLetters := func() {
		runCost += runCostFor(letterRun, e.cfg.CharsPerToken)
		letterRun = 0
	}
	flushDigits := func() {
		runCost += runCostFor(digitRun, e.cfg.DigitsPerToken)
		digitRun = 0
	}
	flushSymbols := func() {
		runCost += runCostFor(symbolRun, e.cfg.SymbolsPerToken)
		symbolRun = 0
	}
	flushAll := func() {
		flushLetters()
		flushDigits()
		flushSymbols()
	}

	for _, r := range text {
		switch {
		case isWideScript(r):
			flushAll()
			wide++
		case unicode.IsLetter(r):
			flushDigits()
			flushSymbols()
			letterRun++
		case unicode.IsDigit(r):
			flushLetters()
			flushSymbols()
			digitRun++
		case r == ' ':
			// A single space is normally absorbed into an adjacent token.
			flushAll()
		case unicode.IsSpace(r):
			// Line breaks and tabs are their own tokens in byte-level BPE, and
			// they dominate code, markdown, and JSONL. Charging them as free
			// under-counted such content severely. Long runs do get merged by
			// real tokenizers, so charging one each over-counts slightly --
			// the safe direction.
			flushAll()
			symbolAcc++
		case r < utf8.RuneSelf:
			// ASCII punctuation. Accumulated as a run because tokenizers merge
			// adjacent punctuation, which is what makes JSON cheaper than one
			// token per character.
			flushLetters()
			flushDigits()
			symbolRun++
		default:
			// Multi-byte symbols and emoji cost several byte-level tokens.
			flushAll()
			symbolAcc += symbolCost(r)
		}
	}
	flushAll()

	total := runCost +
		float64(wide)*e.cfg.TokensPerWideRune +
		symbolAcc

	total *= e.cfg.SafetyMargin

	// Any non-empty input costs at least one token.
	if n := int(total + 0.999999); n > 0 {
		return n
	}
	return 1
}

// symbolCost returns the assumed token cost of a single symbol rune.
//
// Byte-level BPE operates on UTF-8 bytes, so a multi-byte rune such as an emoji
// costs several tokens rather than one. Charging one token per rune under-counts
// emoji-heavy text by a wide margin.
func symbolCost(r rune) float64 {
	n := utf8.RuneLen(r)
	if n <= 1 {
		// Plain ASCII punctuation is a single token.
		return 1
	}
	// Roughly two bytes per token, with a floor of two tokens for any
	// multi-byte symbol.
	if cost := float64(n) / 2.0; cost > 2 {
		return cost
	}
	return 2
}

// isWideScript reports whether r belongs to a script that tokenizers split
// close to one token per character.
func isWideScript(r rune) bool {
	return unicode.In(r,
		unicode.Han,
		unicode.Hiragana,
		unicode.Katakana,
		unicode.Hangul,
		unicode.Thai,
	)
}

// ModelInfo describes a model's context limit.
type ModelInfo struct {
	// Name is the provider-qualified model identifier.
	Name string

	// ContextWindow is the total token capacity, prompt plus completion.
	ContextWindow int
}

// Registry maps model names to their context windows.
//
// It is intentionally empty by default and holds no built-in table of model
// sizes. Self-hosted deployments (Ollama, vLLM) serve arbitrary models whose
// windows this package cannot know, and a stale hardcoded number would silently
// overflow a real window. Operators register what they run, and callers may
// always pass an explicit budget instead.
type Registry struct {
	models map[string]ModelInfo
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{models: make(map[string]ModelInfo)}
}

// Register adds or replaces a model entry. A non-positive ContextWindow is
// rejected, since it cannot produce a usable budget.
func (r *Registry) Register(info ModelInfo) error {
	if info.Name == "" {
		return errors.New("tokenize: model name is required")
	}
	if info.ContextWindow <= 0 {
		return errors.New("tokenize: context window must be positive")
	}
	r.models[info.Name] = info
	return nil
}

// ContextWindow returns the registered window for model, or ErrUnknownModel.
func (r *Registry) ContextWindow(model string) (int, error) {
	info, ok := r.models[model]
	if !ok {
		return 0, ErrUnknownModel
	}
	return info.ContextWindow, nil
}

// Budget resolves a usable token budget.
//
// An explicit budget always wins, so a caller is never blocked by an
// unregistered model. Otherwise the model's window is scaled by fraction to
// leave room for the system prompt, the question, and the completion.
func (r *Registry) Budget(model string, explicit int, fraction float64) (int, error) {
	if explicit > 0 {
		return explicit, nil
	}
	window, err := r.ContextWindow(model)
	if err != nil {
		return 0, err
	}
	if fraction <= 0 || fraction > 1 {
		fraction = 0.5
	}
	budget := int(float64(window) * fraction)
	if budget <= 0 {
		return 0, errors.New("tokenize: resolved budget is not positive")
	}
	return budget, nil
}
