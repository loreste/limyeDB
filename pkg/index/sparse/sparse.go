package sparse

import (
	"encoding/binary"
	"errors"
	"math"
	"sort"
	"sync"
)

// Vector represents a sparse vector with indices and values
type Vector struct {
	Indices []uint32  `json:"indices"`
	Values  []float32 `json:"values"`
}

// NewVector creates a new sparse vector
func NewVector(indices []uint32, values []float32) *Vector {
	return &Vector{
		Indices: indices,
		Values:  values,
	}
}

// NewVectorFromMap creates a sparse vector from a map
func NewVectorFromMap(m map[uint32]float32) *Vector {
	indices := make([]uint32, 0, len(m))
	values := make([]float32, 0, len(m))

	for idx := range m {
		indices = append(indices, idx)
	}
	sort.Slice(indices, func(i, j int) bool { return indices[i] < indices[j] })

	for _, idx := range indices {
		values = append(values, m[idx])
	}

	return &Vector{Indices: indices, Values: values}
}

// Len returns the number of non-zero elements
func (v *Vector) Len() int {
	return len(v.Indices)
}

// DotProduct computes dot product with another sparse vector
func (v *Vector) DotProduct(other *Vector) float32 {
	var result float32
	i, j := 0, 0

	for i < len(v.Indices) && j < len(other.Indices) {
		if v.Indices[i] == other.Indices[j] {
			result += v.Values[i] * other.Values[j]
			i++
			j++
		} else if v.Indices[i] < other.Indices[j] {
			i++
		} else {
			j++
		}
	}

	return result
}

// Norm computes the L2 norm
func (v *Vector) Norm() float32 {
	var sum float32
	for _, val := range v.Values {
		sum += val * val
	}
	return float32(math.Sqrt(float64(sum)))
}

// Normalize normalizes the vector in place
func (v *Vector) Normalize() {
	norm := v.Norm()
	if norm > 0 {
		for i := range v.Values {
			v.Values[i] /= norm
		}
	}
}

// Encode serializes the sparse vector
func (v *Vector) Encode() []byte {
	// Format: [num_elements:4][indices:num*4][values:num*4]
	numElements := len(v.Indices)
	data := make([]byte, 4+numElements*4+numElements*4)

	// #nosec G115 - sparse vectors are limited by practical memory constraints
	binary.LittleEndian.PutUint32(data[0:4], uint32(numElements))

	offset := 4
	for _, idx := range v.Indices {
		binary.LittleEndian.PutUint32(data[offset:], idx)
		offset += 4
	}
	for _, val := range v.Values {
		binary.LittleEndian.PutUint32(data[offset:], math.Float32bits(val))
		offset += 4
	}

	return data
}

// Decode deserializes a sparse vector
func Decode(data []byte) (*Vector, error) {
	if len(data) < 4 {
		return nil, errors.New("invalid sparse vector data")
	}

	numElements := int(binary.LittleEndian.Uint32(data[0:4]))
	expectedLen := 4 + numElements*4 + numElements*4
	if len(data) < expectedLen {
		return nil, errors.New("sparse vector data too short")
	}

	indices := make([]uint32, numElements)
	values := make([]float32, numElements)

	offset := 4
	for i := 0; i < numElements; i++ {
		indices[i] = binary.LittleEndian.Uint32(data[offset:])
		offset += 4
	}
	for i := 0; i < numElements; i++ {
		values[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
		offset += 4
	}

	return &Vector{Indices: indices, Values: values}, nil
}

// =============================================================================
// Inverted Index for Sparse Vectors
// =============================================================================

// InvertedIndex stores posting lists for sparse vector search
type InvertedIndex struct {
	// postingLists maps dimension index to list of (docID, value) pairs
	postingLists map[uint32][]posting
	docCount     int
	mu           sync.RWMutex
}

type posting struct {
	DocID uint32
	Value float32
}

// NewInvertedIndex creates a new inverted index
func NewInvertedIndex() *InvertedIndex {
	return &InvertedIndex{
		postingLists: make(map[uint32][]posting),
	}
}

// Add adds a sparse vector to the index
func (idx *InvertedIndex) Add(docID uint32, vec *Vector) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	for i, dimIdx := range vec.Indices {
		idx.postingLists[dimIdx] = append(idx.postingLists[dimIdx], posting{
			DocID: docID,
			Value: vec.Values[i],
		})
	}
	idx.docCount++
}

// Remove removes a document from the index
func (idx *InvertedIndex) Remove(docID uint32) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	for dimIdx, postings := range idx.postingLists {
		newPostings := postings[:0]
		for _, p := range postings {
			if p.DocID != docID {
				newPostings = append(newPostings, p)
			}
		}
		if len(newPostings) == 0 {
			delete(idx.postingLists, dimIdx)
		} else {
			idx.postingLists[dimIdx] = newPostings
		}
	}
}

// Search finds the top-k documents by dot product with query
func (idx *InvertedIndex) Search(query *Vector, k int) []ScoredDoc {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// Accumulate scores per document
	scores := make(map[uint32]float32)

	for i, dimIdx := range query.Indices {
		queryVal := query.Values[i]
		postings := idx.postingLists[dimIdx]

		for _, p := range postings {
			scores[p.DocID] += queryVal * p.Value
		}
	}

	// Convert to sorted slice
	results := make([]ScoredDoc, 0, len(scores))
	for docID, score := range scores {
		results = append(results, ScoredDoc{DocID: docID, Score: score})
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > k {
		results = results[:k]
	}

	return results
}

// ScoredDoc represents a document with its score
type ScoredDoc struct {
	DocID uint32
	Score float32
}

// Size returns the number of documents in the index
func (idx *InvertedIndex) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.docCount
}

// =============================================================================
// BM25 Scorer for text-based sparse vectors
// =============================================================================

// BM25Config holds BM25 parameters
type BM25Config struct {
	K1 float32 // Term frequency saturation (default: 1.2)
	B  float32 // Length normalization (default: 0.75)
}

// DefaultBM25Config returns default BM25 configuration
func DefaultBM25Config() *BM25Config {
	return &BM25Config{
		K1: 1.2,
		B:  0.75,
	}
}

// BM25Index implements BM25 scoring for sparse vectors
type BM25Index struct {
	config       *BM25Config
	invertedIdx  *InvertedIndex
	docLengths   map[uint32]int
	avgDocLength float32
	totalDocs    int
	docFreq      map[uint32]int // Number of docs containing each term
	mu           sync.RWMutex
}

// NewBM25Index creates a new BM25 index
func NewBM25Index(config *BM25Config) *BM25Index {
	if config == nil {
		config = DefaultBM25Config()
	}
	return &BM25Index{
		config:      config,
		invertedIdx: NewInvertedIndex(),
		docLengths:  make(map[uint32]int),
		docFreq:     make(map[uint32]int),
	}
}

// Add adds a document with term frequencies
func (idx *BM25Index) Add(docID uint32, termFreqs map[uint32]float32) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Calculate document length
	var docLen int
	for _, freq := range termFreqs {
		docLen += int(freq)
	}
	idx.docLengths[docID] = docLen

	// Update average document length
	idx.totalDocs++
	totalLen := float32(0)
	for _, l := range idx.docLengths {
		totalLen += float32(l)
	}
	idx.avgDocLength = totalLen / float32(idx.totalDocs)

	// Update document frequency
	for termID := range termFreqs {
		idx.docFreq[termID]++
	}

	// Add to inverted index (store raw term frequencies)
	vec := NewVectorFromMap(termFreqs)
	idx.invertedIdx.Add(docID, vec)
}

// Search performs BM25 search
func (idx *BM25Index) Search(queryTerms map[uint32]float32, k int) []ScoredDoc {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	scores := make(map[uint32]float32)

	for termID, queryWeight := range queryTerms {
		// Get IDF
		df := idx.docFreq[termID]
		if df == 0 {
			continue
		}
		idf := float32(math.Log(1 + (float64(idx.totalDocs)-float64(df)+0.5)/(float64(df)+0.5)))

		// Get postings for this term
		postings := idx.invertedIdx.postingLists[termID]
		for _, p := range postings {
			tf := p.Value
			docLen := float32(idx.docLengths[p.DocID])

			// BM25 formula
			tfNorm := (tf * (idx.config.K1 + 1)) /
				(tf + idx.config.K1*(1-idx.config.B+idx.config.B*(docLen/idx.avgDocLength)))

			scores[p.DocID] += queryWeight * idf * tfNorm
		}
	}

	// Sort results
	results := make([]ScoredDoc, 0, len(scores))
	for docID, score := range scores {
		results = append(results, ScoredDoc{DocID: docID, Score: score})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > k {
		results = results[:k]
	}

	return results
}

// =============================================================================
// Hybrid Search (Dense + Sparse)
// =============================================================================

// HybridResult combines dense and sparse search results
type HybridResult struct {
	DocID       uint32
	DenseScore  float32
	SparseScore float32
	FinalScore  float32
}

// FusionMethod specifies how to combine dense and sparse scores
type FusionMethod string

const (
	FusionRRF      FusionMethod = "rrf"      // Reciprocal Rank Fusion
	FusionWeighted FusionMethod = "weighted" // Weighted linear combination
)

// HybridSearchConfig holds hybrid search configuration
type HybridSearchConfig struct {
	Method       FusionMethod
	DenseWeight  float32 // For weighted fusion
	SparseWeight float32 // For weighted fusion
	RRFConstant  int     // For RRF (default: 60)
}

// DefaultHybridConfig returns default hybrid search config
func DefaultHybridConfig() *HybridSearchConfig {
	return &HybridSearchConfig{
		Method:       FusionRRF,
		DenseWeight:  0.5,
		SparseWeight: 0.5,
		RRFConstant:  60,
	}
}

// FuseResults combines dense and sparse search results
func FuseResults(denseResults, sparseResults []ScoredDoc, config *HybridSearchConfig, k int) []HybridResult {
	if config == nil {
		config = DefaultHybridConfig()
	}

	switch config.Method {
	case FusionRRF:
		return fuseRRF(denseResults, sparseResults, config.RRFConstant, k)
	case FusionWeighted:
		return fuseWeighted(denseResults, sparseResults, config.DenseWeight, config.SparseWeight, k)
	default:
		return fuseRRF(denseResults, sparseResults, config.RRFConstant, k)
	}
}

// fuseRRF implements Reciprocal Rank Fusion
func fuseRRF(denseResults, sparseResults []ScoredDoc, rrf_k int, k int) []HybridResult {
	scores := make(map[uint32]*HybridResult)

	// Add dense results
	for rank, doc := range denseResults {
		result, ok := scores[doc.DocID]
		if !ok {
			result = &HybridResult{DocID: doc.DocID}
			scores[doc.DocID] = result
		}
		result.DenseScore = doc.Score
		result.FinalScore += 1.0 / float32(rrf_k+rank+1)
	}

	// Add sparse results
	for rank, doc := range sparseResults {
		result, ok := scores[doc.DocID]
		if !ok {
			result = &HybridResult{DocID: doc.DocID}
			scores[doc.DocID] = result
		}
		result.SparseScore = doc.Score
		result.FinalScore += 1.0 / float32(rrf_k+rank+1)
	}

	// Convert to sorted slice
	results := make([]HybridResult, 0, len(scores))
	for _, result := range scores {
		results = append(results, *result)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].FinalScore > results[j].FinalScore
	})

	if len(results) > k {
		results = results[:k]
	}

	return results
}

// fuseWeighted implements weighted linear combination
func fuseWeighted(denseResults, sparseResults []ScoredDoc, denseWeight, sparseWeight float32, k int) []HybridResult {
	// Normalize scores first
	denseNorm := normalizeScores(denseResults)
	sparseNorm := normalizeScores(sparseResults)

	scores := make(map[uint32]*HybridResult)

	// Add dense results
	for _, doc := range denseNorm {
		result, ok := scores[doc.DocID]
		if !ok {
			result = &HybridResult{DocID: doc.DocID}
			scores[doc.DocID] = result
		}
		result.DenseScore = doc.Score
		result.FinalScore += denseWeight * doc.Score
	}

	// Add sparse results
	for _, doc := range sparseNorm {
		result, ok := scores[doc.DocID]
		if !ok {
			result = &HybridResult{DocID: doc.DocID}
			scores[doc.DocID] = result
		}
		result.SparseScore = doc.Score
		result.FinalScore += sparseWeight * doc.Score
	}

	// Convert to sorted slice
	results := make([]HybridResult, 0, len(scores))
	for _, result := range scores {
		results = append(results, *result)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].FinalScore > results[j].FinalScore
	})

	if len(results) > k {
		results = results[:k]
	}

	return results
}

// normalizeScores normalizes scores to 0-1 range
func normalizeScores(docs []ScoredDoc) []ScoredDoc {
	if len(docs) == 0 {
		return docs
	}

	var maxScore, minScore = docs[0].Score, docs[0].Score
	for _, doc := range docs {
		if doc.Score > maxScore {
			maxScore = doc.Score
		}
		if doc.Score < minScore {
			minScore = doc.Score
		}
	}

	rang := maxScore - minScore
	if rang < 1e-8 {
		rang = 1.0
	}

	normalized := make([]ScoredDoc, len(docs))
	for i, doc := range docs {
		normalized[i] = ScoredDoc{
			DocID: doc.DocID,
			Score: (doc.Score - minScore) / rang,
		}
	}

	return normalized
}
