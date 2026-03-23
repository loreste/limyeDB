package hybrid

import (
	"math"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// BM25Index implements the BM25 ranking algorithm for full-text search
type BM25Index struct {
	// BM25 parameters
	k1 float64 // Term frequency saturation parameter (default: 1.2)
	b  float64 // Length normalization parameter (default: 0.75)

	// Index data
	documents    map[string]*Document   // docID -> document
	invertedIdx  map[string]*PostingList // term -> posting list
	docLengths   map[string]int         // docID -> document length
	avgDocLength float64
	totalDocs    int

	// Tokenization
	tokenizer Tokenizer
	stopWords map[string]bool

	mu sync.RWMutex
}

// Document represents an indexed document
type Document struct {
	ID       string
	Content  string
	Fields   map[string]string // field name -> content
	Metadata map[string]interface{}
}

// PostingList contains all documents containing a term
type PostingList struct {
	Term      string
	DocFreq   int                    // Number of documents containing this term
	Postings  map[string]*Posting    // docID -> posting
}

// Posting represents a term occurrence in a document
type Posting struct {
	DocID         string
	TermFreq      int       // Number of times term appears in doc
	Positions     []int     // Positions where term appears
	FieldTermFreq map[string]int // Term frequency per field
}

// Tokenizer defines the interface for text tokenization
type Tokenizer interface {
	Tokenize(text string) []string
}

// DefaultTokenizer implements basic tokenization
type DefaultTokenizer struct {
	lowercase   bool
	stemming    bool
	minLength   int
}

// NewDefaultTokenizer creates a new default tokenizer
func NewDefaultTokenizer() *DefaultTokenizer {
	return &DefaultTokenizer{
		lowercase: true,
		stemming:  false,
		minLength: 2,
	}
}

// Tokenize splits text into tokens
func (t *DefaultTokenizer) Tokenize(text string) []string {
	// Convert to lowercase if enabled
	if t.lowercase {
		text = strings.ToLower(text)
	}

	// Split on non-alphanumeric characters
	var tokens []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else if current.Len() > 0 {
			token := current.String()
			if len(token) >= t.minLength {
				tokens = append(tokens, token)
			}
			current.Reset()
		}
	}

	// Don't forget the last token
	if current.Len() >= t.minLength {
		tokens = append(tokens, current.String())
	}

	return tokens
}

// BM25Config holds BM25 index configuration
type BM25Config struct {
	K1        float64
	B         float64
	Tokenizer Tokenizer
	StopWords []string
}

// DefaultBM25Config returns default BM25 configuration
func DefaultBM25Config() *BM25Config {
	return &BM25Config{
		K1:        1.2,
		B:         0.75,
		Tokenizer: NewDefaultTokenizer(),
		StopWords: defaultStopWords,
	}
}

// NewBM25Index creates a new BM25 index
func NewBM25Index(cfg *BM25Config) *BM25Index {
	if cfg == nil {
		cfg = DefaultBM25Config()
	}

	stopWords := make(map[string]bool)
	for _, w := range cfg.StopWords {
		stopWords[strings.ToLower(w)] = true
	}

	return &BM25Index{
		k1:          cfg.K1,
		b:           cfg.B,
		documents:   make(map[string]*Document),
		invertedIdx: make(map[string]*PostingList),
		docLengths:  make(map[string]int),
		tokenizer:   cfg.Tokenizer,
		stopWords:   stopWords,
	}
}

// Index adds a document to the index
func (idx *BM25Index) Index(doc *Document) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Remove existing document if present
	if _, exists := idx.documents[doc.ID]; exists {
		idx.removeDocLocked(doc.ID)
	}

	// Store document
	idx.documents[doc.ID] = doc

	// Tokenize and index
	allText := doc.Content
	for _, fieldContent := range doc.Fields {
		allText += " " + fieldContent
	}

	docLength := 0

	// Track frequencies globally and per-field
	termFreqs := make(map[string]int)
	fieldTermFreqs := make(map[string]map[string]int)
	termPositions := make(map[string][]int)

	processTokens := func(text string, fieldName string, offset int) int {
		tokens := idx.tokenizer.Tokenize(text)
		for pos, token := range tokens {
			if idx.stopWords[token] {
				continue
			}
			termFreqs[token]++
			termPositions[token] = append(termPositions[token], offset+pos)
			
			if fieldTermFreqs[token] == nil {
				fieldTermFreqs[token] = make(map[string]int)
			}
			fieldTermFreqs[token][fieldName]++
			docLength++
		}
		return len(tokens)
	}

	offset := 0
	offset += processTokens(doc.Content, "", offset)
	for fieldName, fieldContent := range doc.Fields {
		offset += processTokens(fieldContent, fieldName, offset)
	}

	// Update inverted index
	for term, freq := range termFreqs {
		postingList, exists := idx.invertedIdx[term]
		if !exists {
			postingList = &PostingList{
				Term:     term,
				Postings: make(map[string]*Posting),
			}
			idx.invertedIdx[term] = postingList
		}

		postingList.Postings[doc.ID] = &Posting{
			DocID:         doc.ID,
			TermFreq:      freq,
			Positions:     termPositions[term],
			FieldTermFreq: fieldTermFreqs[term],
		}
		postingList.DocFreq = len(postingList.Postings)
	}

	// Update document length
	idx.docLengths[doc.ID] = docLength
	idx.totalDocs++

	// Recalculate average document length
	totalLength := 0
	for _, length := range idx.docLengths {
		totalLength += length
	}
	idx.avgDocLength = float64(totalLength) / float64(idx.totalDocs)

	return nil
}

// Remove removes a document from the index
func (idx *BM25Index) Remove(docID string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return idx.removeDocLocked(docID)
}

func (idx *BM25Index) removeDocLocked(docID string) error {
	doc, exists := idx.documents[docID]
	if !exists {
		return nil
	}

	// Remove from inverted index
	allText := doc.Content
	for _, fieldContent := range doc.Fields {
		allText += " " + fieldContent
	}

	tokens := idx.tokenizer.Tokenize(allText)
	seen := make(map[string]bool)

	for _, token := range tokens {
		if idx.stopWords[token] || seen[token] {
			continue
		}
		seen[token] = true

		if postingList, exists := idx.invertedIdx[token]; exists {
			delete(postingList.Postings, docID)
			postingList.DocFreq = len(postingList.Postings)

			// Remove empty posting lists
			if postingList.DocFreq == 0 {
				delete(idx.invertedIdx, token)
			}
		}
	}

	// Remove document
	delete(idx.documents, docID)
	delete(idx.docLengths, docID)
	idx.totalDocs--

	// Recalculate average document length
	if idx.totalDocs > 0 {
		totalLength := 0
		for _, length := range idx.docLengths {
			totalLength += length
		}
		idx.avgDocLength = float64(totalLength) / float64(idx.totalDocs)
	} else {
		idx.avgDocLength = 0
	}

	return nil
}

// SearchResult represents a BM25 search result
type SearchResult struct {
	DocID    string
	Score    float64
	Document *Document
}

// Search performs BM25 search
func (idx *BM25Index) Search(query string, limit int) []SearchResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.totalDocs == 0 {
		return nil
	}

	queryTokens := idx.tokenizer.Tokenize(query)
	if len(queryTokens) == 0 {
		return nil
	}

	// Calculate BM25 scores
	scores := make(map[string]float64)

	for _, token := range queryTokens {
		if idx.stopWords[token] {
			continue
		}

		postingList, exists := idx.invertedIdx[token]
		if !exists {
			continue
		}

		// Calculate IDF
		idf := math.Log((float64(idx.totalDocs)-float64(postingList.DocFreq)+0.5) /
			(float64(postingList.DocFreq) + 0.5) + 1)

		for docID, posting := range postingList.Postings {
			docLength := float64(idx.docLengths[docID])
			tf := float64(posting.TermFreq)

			// BM25 score formula
			numerator := tf * (idx.k1 + 1)
			denominator := tf + idx.k1*(1-idx.b+idx.b*(docLength/idx.avgDocLength))
			score := idf * numerator / denominator

			scores[docID] += score
		}
	}

	// Sort by score
	results := make([]SearchResult, 0, len(scores))
	for docID, score := range scores {
		results = append(results, SearchResult{
			DocID:    docID,
			Score:    score,
			Document: idx.documents[docID],
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results
}

// SearchWithBoost performs BM25 search with field boosting.
func (idx *BM25Index) SearchWithBoost(query string, limit int, fieldBoosts map[string]float64) []SearchResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.totalDocs == 0 {
		return nil
	}

	queryTokens := idx.tokenizer.Tokenize(query)
	if len(queryTokens) == 0 {
		return nil
	}

	scores := make(map[string]float64)

	for _, token := range queryTokens {
		if idx.stopWords[token] {
			continue
		}

		postingList, exists := idx.invertedIdx[token]
		if !exists {
			continue
		}

		idf := math.Log((float64(idx.totalDocs)-float64(postingList.DocFreq)+0.5) /
			(float64(postingList.DocFreq) + 0.5) + 1)

		for docID, posting := range postingList.Postings {
			docLength := float64(idx.docLengths[docID])
			
			tf := 0.0
			if len(fieldBoosts) > 0 {
				for fieldName, freq := range posting.FieldTermFreq {
					boost, hasBoost := fieldBoosts[fieldName]
					if !hasBoost {
						boost = 1.0
					}
					// Default empty string represents the main Content
					if fieldName == "" {
						boost = 1.0 // Content doesn't get a specific text field boost unless defined, but typically its 1.0
					}
					tf += float64(freq) * boost
				}
			} else {
				tf = float64(posting.TermFreq)
			}

			numerator := tf * (idx.k1 + 1)
			denominator := tf + idx.k1*(1-idx.b+idx.b*(docLength/idx.avgDocLength))
			score := idf * numerator / denominator

			scores[docID] += score
		}
	}

	results := make([]SearchResult, 0, len(scores))
	for docID, score := range scores {
		results = append(results, SearchResult{
			DocID:    docID,
			Score:    score,
			Document: idx.documents[docID],
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results
}


// Size returns the number of documents in the index
func (idx *BM25Index) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.totalDocs
}

// GetDocument retrieves a document by ID
func (idx *BM25Index) GetDocument(docID string) (*Document, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	doc, exists := idx.documents[docID]
	return doc, exists
}

// Common English stop words
var defaultStopWords = []string{
	"a", "an", "and", "are", "as", "at", "be", "by", "for", "from",
	"has", "he", "in", "is", "it", "its", "of", "on", "that", "the",
	"to", "was", "were", "will", "with", "the", "this", "but", "they",
	"have", "had", "what", "when", "where", "who", "which", "why", "how",
}
