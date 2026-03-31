package hybrid

import (
	"testing"
)

func TestNewBM25Index(t *testing.T) {
	t.Parallel()

	// Test with nil config
	idx := NewBM25Index(nil)
	if idx == nil {
		t.Fatal("NewBM25Index returned nil")
	}

	if idx.k1 != 1.2 {
		t.Errorf("Expected default k1=1.2, got %f", idx.k1)
	}
	if idx.b != 0.75 {
		t.Errorf("Expected default b=0.75, got %f", idx.b)
	}
}

func TestNewBM25IndexWithConfig(t *testing.T) {
	t.Parallel()

	cfg := &BM25Config{
		K1:        1.5,
		B:         0.5,
		Tokenizer: NewDefaultTokenizer(),
		StopWords: []string{"the", "a", "an"},
	}

	idx := NewBM25Index(cfg)
	if idx.k1 != 1.5 {
		t.Errorf("Expected k1=1.5, got %f", idx.k1)
	}
	if idx.b != 0.5 {
		t.Errorf("Expected b=0.5, got %f", idx.b)
	}
}

func TestDefaultTokenizer(t *testing.T) {
	t.Parallel()

	tokenizer := NewDefaultTokenizer()

	testCases := []struct {
		input    string
		expected []string
	}{
		{"Hello World", []string{"hello", "world"}},
		{"hello, world!", []string{"hello", "world"}},
		{"Hello123 World456", []string{"hello123", "world456"}},
		{"a b c", []string{}}, // Too short (minLength = 2)
		{"", []string{}},
		{"   spaces   everywhere   ", []string{"spaces", "everywhere"}},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			tokens := tokenizer.Tokenize(tc.input)
			if len(tokens) != len(tc.expected) {
				t.Errorf("Input %q: expected %d tokens, got %d", tc.input, len(tc.expected), len(tokens))
				return
			}
			for i, token := range tokens {
				if token != tc.expected[i] {
					t.Errorf("Input %q: token %d expected %q, got %q", tc.input, i, tc.expected[i], token)
				}
			}
		})
	}
}

func TestBM25IndexDocument(t *testing.T) {
	t.Parallel()

	idx := NewBM25Index(nil)

	doc := &Document{
		ID:      "doc1",
		Content: "The quick brown fox jumps over the lazy dog",
	}

	err := idx.Index(doc)
	if err != nil {
		t.Fatalf("Index() failed: %v", err)
	}

	if idx.Size() != 1 {
		t.Errorf("Expected size=1, got %d", idx.Size())
	}
}

func TestBM25IndexDocumentWithFields(t *testing.T) {
	t.Parallel()

	idx := NewBM25Index(nil)

	doc := &Document{
		ID:      "doc1",
		Content: "Main content here",
		Fields: map[string]string{
			"title":       "Document Title",
			"description": "A short description",
		},
	}

	err := idx.Index(doc)
	if err != nil {
		t.Fatalf("Index() failed: %v", err)
	}

	if idx.Size() != 1 {
		t.Errorf("Expected size=1, got %d", idx.Size())
	}
}

func TestBM25IndexReplace(t *testing.T) {
	t.Parallel()

	idx := NewBM25Index(nil)

	doc1 := &Document{
		ID:      "doc1",
		Content: "First version",
	}
	doc2 := &Document{
		ID:      "doc1",
		Content: "Second version",
	}

	_ = idx.Index(doc1)
	_ = idx.Index(doc2)

	// Should still be 1 document
	if idx.Size() != 1 {
		t.Errorf("Expected size=1 after replace, got %d", idx.Size())
	}
}

func TestBM25Remove(t *testing.T) {
	t.Parallel()

	idx := NewBM25Index(nil)

	doc := &Document{
		ID:      "doc1",
		Content: "Some content here",
	}

	_ = idx.Index(doc)
	err := idx.Remove("doc1")
	if err != nil {
		t.Fatalf("Remove() failed: %v", err)
	}

	if idx.Size() != 0 {
		t.Errorf("Expected size=0 after remove, got %d", idx.Size())
	}
}

func TestBM25RemoveNonexistent(t *testing.T) {
	t.Parallel()

	idx := NewBM25Index(nil)

	// Should not error
	err := idx.Remove("nonexistent")
	if err != nil {
		t.Errorf("Remove() should not error for nonexistent doc: %v", err)
	}
}

func TestBM25Search(t *testing.T) {
	t.Parallel()

	idx := NewBM25Index(nil)

	docs := []*Document{
		{ID: "doc1", Content: "The quick brown fox jumps over the lazy dog"},
		{ID: "doc2", Content: "A fox is a small mammal"},
		{ID: "doc3", Content: "Dogs are loyal pets"},
	}

	for _, doc := range docs {
		_ = idx.Index(doc)
	}

	results := idx.Search("fox", 10)
	if len(results) == 0 {
		t.Fatal("Expected results for 'fox' query")
	}

	// doc1 and doc2 should be returned
	foundDoc1 := false
	foundDoc2 := false
	for _, r := range results {
		if r.DocID == "doc1" {
			foundDoc1 = true
		}
		if r.DocID == "doc2" {
			foundDoc2 = true
		}
	}

	if !foundDoc1 {
		t.Error("Expected doc1 in results")
	}
	if !foundDoc2 {
		t.Error("Expected doc2 in results")
	}
}

func TestBM25SearchEmpty(t *testing.T) {
	t.Parallel()

	idx := NewBM25Index(nil)

	results := idx.Search("anything", 10)
	if len(results) != 0 {
		t.Errorf("Expected 0 results for empty index, got %d", len(results))
	}
}

func TestBM25SearchNoMatch(t *testing.T) {
	t.Parallel()

	idx := NewBM25Index(nil)

	doc := &Document{
		ID:      "doc1",
		Content: "Hello world",
	}
	_ = idx.Index(doc)

	results := idx.Search("xyz123", 10)
	if len(results) != 0 {
		t.Errorf("Expected 0 results for non-matching query, got %d", len(results))
	}
}

func TestBM25SearchLimit(t *testing.T) {
	t.Parallel()

	idx := NewBM25Index(nil)

	for i := 0; i < 20; i++ {
		doc := &Document{
			ID:      string(rune('a' + i)),
			Content: "test content with word",
		}
		_ = idx.Index(doc)
	}

	results := idx.Search("word", 5)
	if len(results) > 5 {
		t.Errorf("Expected max 5 results, got %d", len(results))
	}
}

func TestBM25SearchScoreOrder(t *testing.T) {
	t.Parallel()

	idx := NewBM25Index(nil)

	docs := []*Document{
		{ID: "doc1", Content: "fox fox fox fox fox"}, // Most occurrences
		{ID: "doc2", Content: "fox fox fox"},          // Medium
		{ID: "doc3", Content: "fox"},                  // Least
	}

	for _, doc := range docs {
		_ = idx.Index(doc)
	}

	results := idx.Search("fox", 10)
	if len(results) < 3 {
		t.Fatal("Expected 3 results")
	}

	// Results should be ordered by score (descending)
	for i := 0; i < len(results)-1; i++ {
		if results[i].Score < results[i+1].Score {
			t.Errorf("Results not properly sorted by score: %f < %f", results[i].Score, results[i+1].Score)
		}
	}
}

func TestBM25SearchWithBoost(t *testing.T) {
	t.Parallel()

	idx := NewBM25Index(nil)

	docs := []*Document{
		{
			ID:      "doc1",
			Content: "general content",
			Fields:  map[string]string{"title": "fox title"},
		},
		{
			ID:      "doc2",
			Content: "fox content",
			Fields:  map[string]string{"title": "general title"},
		},
	}

	for _, doc := range docs {
		_ = idx.Index(doc)
	}

	// Boost title field
	boosts := map[string]float64{"title": 2.0}
	results := idx.SearchWithBoost("fox", 10, boosts)

	if len(results) == 0 {
		t.Fatal("Expected results with boost")
	}
}

func TestBM25GetDocument(t *testing.T) {
	t.Parallel()

	idx := NewBM25Index(nil)

	doc := &Document{
		ID:      "doc1",
		Content: "Test content",
		Fields:  map[string]string{"field1": "value1"},
	}
	_ = idx.Index(doc)

	retrieved, exists := idx.GetDocument("doc1")
	if !exists {
		t.Fatal("Document should exist")
	}
	if retrieved.ID != "doc1" {
		t.Errorf("Expected ID=doc1, got %s", retrieved.ID)
	}
}

func TestBM25GetDocumentNotFound(t *testing.T) {
	t.Parallel()

	idx := NewBM25Index(nil)

	_, exists := idx.GetDocument("nonexistent")
	if exists {
		t.Error("Document should not exist")
	}
}

func TestBM25StopWords(t *testing.T) {
	t.Parallel()

	cfg := &BM25Config{
		K1:        1.2,
		B:         0.75,
		Tokenizer: NewDefaultTokenizer(),
		StopWords: []string{"the", "is", "a"},
	}
	idx := NewBM25Index(cfg)

	doc := &Document{
		ID:      "doc1",
		Content: "The fox is a animal",
	}
	_ = idx.Index(doc)

	// Search for stop word should not return results
	results := idx.Search("the", 10)
	if len(results) != 0 {
		t.Errorf("Expected 0 results for stop word, got %d", len(results))
	}

	// Search for non-stop word should return results
	results = idx.Search("fox", 10)
	if len(results) == 0 {
		t.Error("Expected results for 'fox'")
	}
}

func TestBM25MultipleQueryTerms(t *testing.T) {
	t.Parallel()

	idx := NewBM25Index(nil)

	docs := []*Document{
		{ID: "doc1", Content: "quick brown fox"},
		{ID: "doc2", Content: "lazy brown dog"},
		{ID: "doc3", Content: "quick lazy cat"},
	}

	for _, doc := range docs {
		_ = idx.Index(doc)
	}

	results := idx.Search("brown lazy", 10)
	if len(results) == 0 {
		t.Fatal("Expected results for multi-term query")
	}

	// doc2 should score highest (has both terms)
	// Note: this depends on BM25 scoring
}

func TestBM25EmptyQuery(t *testing.T) {
	t.Parallel()

	idx := NewBM25Index(nil)

	doc := &Document{
		ID:      "doc1",
		Content: "Some content",
	}
	_ = idx.Index(doc)

	results := idx.Search("", 10)
	if len(results) != 0 {
		t.Errorf("Expected 0 results for empty query, got %d", len(results))
	}
}

func TestBM25AverageDocLength(t *testing.T) {
	t.Parallel()

	idx := NewBM25Index(nil)

	docs := []*Document{
		{ID: "doc1", Content: "short"},
		{ID: "doc2", Content: "medium length document"},
		{ID: "doc3", Content: "this is a longer document with more words"},
	}

	for _, doc := range docs {
		_ = idx.Index(doc)
	}

	if idx.avgDocLength <= 0 {
		t.Error("Expected positive average document length")
	}

	// After removing all docs
	for _, doc := range docs {
		_ = idx.Remove(doc.ID)
	}

	if idx.avgDocLength != 0 {
		t.Errorf("Expected avgDocLength=0 after removing all docs, got %f", idx.avgDocLength)
	}
}
