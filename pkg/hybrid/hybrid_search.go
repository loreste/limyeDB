package hybrid

import (
	"sort"
	"sync"
	"time"

	"github.com/limyedb/limyedb/pkg/index/hnsw"
	"github.com/limyedb/limyedb/pkg/point"
)

// FusionMethod defines how to combine vector and text search results
type FusionMethod string

const (
	// FusionRRF uses Reciprocal Rank Fusion
	FusionRRF FusionMethod = "rrf"
	// FusionWeighted uses weighted score combination
	FusionWeighted FusionMethod = "weighted"
	// FusionConvex uses convex combination of normalized scores
	FusionConvex FusionMethod = "convex"
)

// HybridSearcher combines vector search with full-text search
type HybridSearcher struct {
	vectorIndex *hnsw.HNSW
	textIndex   *BM25Index

	// Default fusion parameters
	fusionMethod FusionMethod
	vectorWeight float64 // Weight for vector search (0-1)
	textWeight   float64 // Weight for text search (0-1)
	rrfK         int     // RRF constant (default: 60)

	mu sync.RWMutex
}

// HybridConfig holds hybrid search configuration
type HybridConfig struct {
	FusionMethod FusionMethod
	VectorWeight float64
	TextWeight   float64
	RRF_K        int
}

// DefaultHybridConfig returns default hybrid search configuration
func DefaultHybridConfig() *HybridConfig {
	return &HybridConfig{
		FusionMethod: FusionRRF,
		VectorWeight: 0.7,
		TextWeight:   0.3,
		RRF_K:        60,
	}
}

// NewHybridSearcher creates a new hybrid searcher
func NewHybridSearcher(vectorIndex *hnsw.HNSW, textIndex *BM25Index, cfg *HybridConfig) *HybridSearcher {
	if cfg == nil {
		cfg = DefaultHybridConfig()
	}

	return &HybridSearcher{
		vectorIndex:  vectorIndex,
		textIndex:    textIndex,
		fusionMethod: cfg.FusionMethod,
		vectorWeight: cfg.VectorWeight,
		textWeight:   cfg.TextWeight,
		rrfK:         cfg.RRF_K,
	}
}

// HybridSearchParams holds parameters for hybrid search
type HybridSearchParams struct {
	// Vector search parameters
	Vector     point.Vector
	VectorName string

	// Text search parameters
	Query     string
	TextField string // Field to search in (default: all)

	// Fusion parameters
	FusionMethod FusionMethod
	VectorWeight float64
	TextWeight   float64

	// Common parameters
	Limit       int
	Offset      int
	WithPayload bool
	WithVector  bool

	// Prefetch multiplier (how many results to fetch before fusion)
	PrefetchMultiplier int
}

// HybridSearchResult holds hybrid search results
type HybridSearchResult struct {
	Points       []HybridScoredPoint `json:"points"`
	TookMs       int64               `json:"took_ms"`
	VectorTookMs int64               `json:"vector_took_ms"`
	TextTookMs   int64               `json:"text_took_ms"`
}

// HybridScoredPoint represents a result with combined score
type HybridScoredPoint struct {
	ID          string                 `json:"id"`
	Score       float64                `json:"score"`
	VectorScore float64                `json:"vector_score,omitempty"`
	TextScore   float64                `json:"text_score,omitempty"`
	VectorRank  int                    `json:"vector_rank,omitempty"`
	TextRank    int                    `json:"text_rank,omitempty"`
	Vector      point.Vector           `json:"vector,omitempty"`
	Payload     map[string]interface{} `json:"payload,omitempty"`
}

// Search performs hybrid search combining vector and text search
func (h *HybridSearcher) Search(params *HybridSearchParams) (*HybridSearchResult, error) {
	startTime := time.Now()

	if params.Limit <= 0 {
		params.Limit = 10
	}

	if params.PrefetchMultiplier <= 0 {
		params.PrefetchMultiplier = 3
	}

	prefetchLimit := params.Limit * params.PrefetchMultiplier

	// Determine fusion parameters
	fusionMethod := params.FusionMethod
	if fusionMethod == "" {
		fusionMethod = h.fusionMethod
	}

	vectorWeight := params.VectorWeight
	if vectorWeight == 0 {
		vectorWeight = h.vectorWeight
	}

	textWeight := params.TextWeight
	if textWeight == 0 {
		textWeight = h.textWeight
	}

	// Run searches in parallel
	var vectorResults []hnsw.Candidate
	var textResults []SearchResult
	var vectorErr error
	var vectorDuration, textDuration time.Duration

	var wg sync.WaitGroup

	// Vector search
	if len(params.Vector) > 0 && h.vectorIndex != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vectorStart := time.Now()
			vectorResults, vectorErr = h.vectorIndex.SearchWithEf(params.Vector, prefetchLimit, 100)
			vectorDuration = time.Since(vectorStart)
		}()
	}

	// Text search
	if params.Query != "" && h.textIndex != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			textStart := time.Now()
			textResults = h.textIndex.Search(params.Query, prefetchLimit)
			textDuration = time.Since(textStart)
		}()
	}

	wg.Wait()

	if vectorErr != nil {
		return nil, vectorErr
	}

	// Fuse results
	var fusedResults []HybridScoredPoint

	switch fusionMethod {
	case FusionRRF:
		fusedResults = h.fuseRRF(vectorResults, textResults, params.Limit)
	case FusionWeighted:
		fusedResults = h.fuseWeighted(vectorResults, textResults, vectorWeight, textWeight, params.Limit)
	case FusionConvex:
		fusedResults = h.fuseConvex(vectorResults, textResults, vectorWeight, params.Limit)
	default:
		fusedResults = h.fuseRRF(vectorResults, textResults, params.Limit)
	}

	// Apply offset
	if params.Offset > 0 && params.Offset < len(fusedResults) {
		fusedResults = fusedResults[params.Offset:]
	}

	// Enrich results with payload and vector if requested
	if params.WithPayload || params.WithVector {
		h.enrichResults(fusedResults, params.WithVector, params.WithPayload)
	}

	return &HybridSearchResult{
		Points:       fusedResults,
		TookMs:       time.Since(startTime).Milliseconds(),
		VectorTookMs: vectorDuration.Milliseconds(),
		TextTookMs:   textDuration.Milliseconds(),
	}, nil
}

// fuseRRF performs Reciprocal Rank Fusion
func (h *HybridSearcher) fuseRRF(vectorResults []hnsw.Candidate, textResults []SearchResult, limit int) []HybridScoredPoint {
	scores := make(map[string]*HybridScoredPoint)
	k := float64(h.rrfK)

	// Process vector results
	for rank, result := range vectorResults {
		id := h.vectorIndex.GetPointID(result.ID)
		if _, exists := scores[id]; !exists {
			scores[id] = &HybridScoredPoint{ID: id}
		}
		scores[id].VectorRank = rank + 1
		scores[id].VectorScore = float64(1.0 - result.Distance)
		scores[id].Score += 1.0 / (k + float64(rank+1))
	}

	// Process text results
	for rank, result := range textResults {
		if _, exists := scores[result.DocID]; !exists {
			scores[result.DocID] = &HybridScoredPoint{ID: result.DocID}
		}
		scores[result.DocID].TextRank = rank + 1
		scores[result.DocID].TextScore = result.Score
		scores[result.DocID].Score += 1.0 / (k + float64(rank+1))
	}

	// Sort by fused score
	results := make([]HybridScoredPoint, 0, len(scores))
	for _, sp := range scores {
		results = append(results, *sp)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results
}

// fuseWeighted performs weighted score combination
func (h *HybridSearcher) fuseWeighted(vectorResults []hnsw.Candidate, textResults []SearchResult, vectorWeight, textWeight float64, limit int) []HybridScoredPoint {
	scores := make(map[string]*HybridScoredPoint)

	// Normalize weights
	totalWeight := vectorWeight + textWeight
	vectorWeight /= totalWeight
	textWeight /= totalWeight

	// Process vector results (convert distance to similarity)
	for rank, result := range vectorResults {
		id := h.vectorIndex.GetPointID(result.ID)
		similarity := 1.0 - float64(result.Distance)
		if _, exists := scores[id]; !exists {
			scores[id] = &HybridScoredPoint{ID: id}
		}
		scores[id].VectorRank = rank + 1
		scores[id].VectorScore = similarity
		scores[id].Score += similarity * vectorWeight
	}

	// Normalize text scores and add
	var maxTextScore float64
	for _, result := range textResults {
		if result.Score > maxTextScore {
			maxTextScore = result.Score
		}
	}

	for rank, result := range textResults {
		normalizedScore := result.Score
		if maxTextScore > 0 {
			normalizedScore = result.Score / maxTextScore
		}

		if _, exists := scores[result.DocID]; !exists {
			scores[result.DocID] = &HybridScoredPoint{ID: result.DocID}
		}
		scores[result.DocID].TextRank = rank + 1
		scores[result.DocID].TextScore = result.Score
		scores[result.DocID].Score += normalizedScore * textWeight
	}

	// Sort by fused score
	results := make([]HybridScoredPoint, 0, len(scores))
	for _, sp := range scores {
		results = append(results, *sp)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results
}

// fuseConvex performs convex combination of normalized scores
func (h *HybridSearcher) fuseConvex(vectorResults []hnsw.Candidate, textResults []SearchResult, alpha float64, limit int) []HybridScoredPoint {
	// alpha controls the weight: 1.0 = all vector, 0.0 = all text
	return h.fuseWeighted(vectorResults, textResults, alpha, 1-alpha, limit)
}

// enrichResults adds payload and vector data to results
func (h *HybridSearcher) enrichResults(results []HybridScoredPoint, withVector, withPayload bool) {
	for i := range results {
		if h.vectorIndex != nil {
			p, err := h.vectorIndex.Get(results[i].ID)
			if err == nil {
				if withVector {
					results[i].Vector = p.Vector
				}
				if withPayload {
					results[i].Payload = p.Payload
				}
			}
		}
	}
}

// IndexText indexes text content for a document
func (h *HybridSearcher) IndexText(docID string, content string, fields map[string]string) error {
	if h.textIndex == nil {
		return nil
	}

	doc := &Document{
		ID:      docID,
		Content: content,
		Fields:  fields,
	}

	return h.textIndex.Index(doc)
}

// RemoveText removes a document from the text index
func (h *HybridSearcher) RemoveText(docID string) error {
	if h.textIndex == nil {
		return nil
	}
	return h.textIndex.Remove(docID)
}

// UpdateWeights updates the default fusion weights
func (h *HybridSearcher) UpdateWeights(vectorWeight, textWeight float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.vectorWeight = vectorWeight
	h.textWeight = textWeight
}

// SetFusionMethod sets the default fusion method
func (h *HybridSearcher) SetFusionMethod(method FusionMethod) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.fusionMethod = method
}
