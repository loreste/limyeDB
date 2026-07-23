package tokenize

import (
	"strings"
	"testing"
)

// The tests below pin the estimator's accuracy envelope from both directions.
// Under-counting overflows a context window, which is a hard failure. But
// over-counting silently wastes the window, so a wildly pessimistic estimator
// is not "safe" either -- it just fails quietly. Each case here caught a real
// defect during development.

// TestWhitespaceIsNotFree guards a defect where whitespace cost nothing, so 500
// newlines were charged as a single token. Newline-dense content -- code,
// markdown, JSONL -- was under-counted severely.
func TestWhitespaceIsNotFree(t *testing.T) {
	e := NewEstimator(DefaultEstimatorConfig())

	got := e.Estimate(strings.Repeat("\n", 500))
	if got < 50 {
		t.Errorf("500 newlines = %d tokens, implausibly low; line breaks must cost tokens", got)
	}
}

// TestNewlineDenseDocumentCost checks that a document of short lines is charged
// for both content and structure.
func TestNewlineDenseDocumentCost(t *testing.T) {
	e := NewEstimator(DefaultEstimatorConfig())

	var b strings.Builder
	for i := 0; i < 200; i++ {
		b.WriteString("x = 1\n")
	}

	if got := e.Estimate(b.String()); got < 200 {
		t.Errorf("200 short lines = %d tokens, want >= 200", got)
	}
}

// TestMultiByteSymbolCost guards a defect where an emoji was charged as one
// token. Byte-level BPE works on UTF-8 bytes, so multi-byte runes cost more.
func TestMultiByteSymbolCost(t *testing.T) {
	e := NewEstimator(DefaultEstimatorConfig())

	if got := e.Estimate(strings.Repeat("🙂", 100)); got < 200 {
		t.Errorf("100 emoji = %d tokens, want >= 200", got)
	}
}

// TestDigitRunsAreDense guards a defect where digits were charged at prose
// density. Tokenizers split numbers into groups of one to three digits.
func TestDigitRunsAreDense(t *testing.T) {
	e := NewEstimator(DefaultEstimatorConfig())

	if got := e.Estimate(strings.Repeat("7", 120)); got < 40 {
		t.Errorf("120 digits = %d tokens, want >= 40", got)
	}
}

// TestProseStaysCalibrated is the counterweight to the under-count tests. Plain
// prose really does land near four characters per token, so the estimator must
// not drift far above that or every budget wastes most of the window.
func TestProseStaysCalibrated(t *testing.T) {
	e := NewEstimator(DefaultEstimatorConfig())

	prose := "Retrieval augmented generation systems must budget their context " +
		"carefully, because the model cannot read more than its window allows " +
		"and every wasted token displaces evidence that might have answered " +
		"the question."

	naive := float64(len(prose)) / 4.0
	got := float64(e.Estimate(prose))
	ratio := got / naive

	// Never below the naive ratio (that would be an under-count), and not more
	// than 1.5x above it (that would waste a third of the window on prose).
	if ratio < 1.0 {
		t.Errorf("prose estimate %.0f is %.2fx naive %.1f, want >= 1.0x", got, ratio, naive)
	}
	if ratio > 1.5 {
		t.Errorf("prose estimate %.0f is %.2fx naive %.1f, want <= 1.5x", got, ratio, naive)
	}
}

// TestStructuredDataStaysReasonable pins JSON cost. Charging one token per
// punctuation character made JSON 3.1x the naive ratio; tokenizers merge
// adjacent punctuation, and JSON payloads are the common case for this database.
func TestStructuredDataStaysReasonable(t *testing.T) {
	e := NewEstimator(DefaultEstimatorConfig())

	doc := `{"id":"abc","score":0.91,"payload":{"title":"x"}}`

	naive := float64(len(doc)) / 4.0
	ratio := float64(e.Estimate(doc)) / naive

	// Structured data is genuinely denser than prose, so above 1.0x is correct,
	// but it should stay well under the one-token-per-symbol worst case.
	if ratio < 1.0 {
		t.Errorf("JSON estimate is %.2fx naive, want >= 1.0x", ratio)
	}
	if ratio > 2.5 {
		t.Errorf("JSON estimate is %.2fx naive, want <= 2.5x", ratio)
	}
}

// TestMixedAlphanumericRuns checks that class transitions inside a token-like
// string are charged, since identifiers such as "abc123def" split at boundaries.
func TestMixedAlphanumericRuns(t *testing.T) {
	e := NewEstimator(DefaultEstimatorConfig())

	mixed := e.Estimate("abc123def456ghi789")
	letters := e.Estimate("abcdefghi")

	if mixed <= letters {
		t.Errorf("mixed alphanumeric = %d, plain letters = %d; digits must add cost",
			mixed, letters)
	}
}
