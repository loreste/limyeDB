package collection

import (
	"sort"
	"time"

	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/distance"
	"github.com/limyedb/limyedb/pkg/index/hnsw"
	"github.com/limyedb/limyedb/pkg/index/payload"
	"github.com/limyedb/limyedb/pkg/point"
)

// DiscoveryParams holds parameters for discovery/context search
type DiscoveryParams struct {
	// Target is the main query vector
	Target point.Vector `json:"target,omitempty"`

	// Context provides additional context for the search
	Context *DiscoveryContext `json:"context,omitempty"`

	// Standard search parameters
	K          int             `json:"limit"`
	Ef         int             `json:"ef,omitempty"`
	Filter     *payload.Filter `json:"filter,omitempty"`
	VectorName string          `json:"vector_name,omitempty"`

	// Result options
	WithVector  bool `json:"with_vector,omitempty"`
	WithPayload bool `json:"with_payload,omitempty"`
}

// DiscoveryContext provides positive and negative examples for the search
type DiscoveryContext struct {
	// Positive examples - results should be similar to these
	Positive []ContextExample `json:"positive,omitempty"`

	// Negative examples - results should be dissimilar to these
	Negative []ContextExample `json:"negative,omitempty"`
}

// ContextExample is either a point ID or a vector
type ContextExample struct {
	ID     string       `json:"id,omitempty"`
	Vector point.Vector `json:"vector,omitempty"`
}

// DiscoveryResult holds discovery search results
type DiscoveryResult struct {
	Points []ScoredPointV2 `json:"points"`
	TookMs int64           `json:"took_ms"`
}

// Discover performs context-aware search
// It finds points similar to positive examples and dissimilar to negative examples
func (c *Collection) Discover(params *DiscoveryParams) (*DiscoveryResult, error) {
	start := time.Now()

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Get the appropriate index and config
	var idx *hnsw.HNSW
	var distCalc distance.Calculator
	var vc *config.VectorConfig

	if c.config.HasNamedVectors() {
		vectorName := params.VectorName
		if vectorName == "" {
			vectorName = "default"
		}
		var ok bool
		idx, ok = c.indices[vectorName]
		if !ok {
			return nil, CollectionError("unknown vector name: " + vectorName)
		}
		distCalc = c.distCalcs[vectorName]
		vc = c.config.GetVectorConfig(vectorName)
	} else {
		idx = c.index
		distCalc = c.distCalc
		vc = c.config.GetVectorConfig("")
	}

	if vc == nil {
		return nil, CollectionError("vector config not found")
	}

	// Resolve context examples to vectors
	var positiveVectors []point.Vector
	var negativeVectors []point.Vector

	if params.Context != nil {
		for _, ex := range params.Context.Positive {
			vec, err := c.resolveContextExample(idx, ex)
			if err != nil {
				return nil, err
			}
			if vc.Metric == config.MetricCosine {
				vec = distance.Normalize(vec)
			}
			positiveVectors = append(positiveVectors, vec)
		}

		for _, ex := range params.Context.Negative {
			vec, err := c.resolveContextExample(idx, ex)
			if err != nil {
				return nil, err
			}
			if vc.Metric == config.MetricCosine {
				vec = distance.Normalize(vec)
			}
			negativeVectors = append(negativeVectors, vec)
		}
	}

	// Prepare target vector
	var target point.Vector
	if len(params.Target) > 0 {
		target = params.Target
		if vc.Metric == config.MetricCosine {
			target = distance.Normalize(target)
		}
	} else if len(positiveVectors) > 0 {
		// Use centroid of positive examples as target
		target = computeCentroid(positiveVectors, vc.Dimension)
	} else {
		return nil, CollectionError("no target vector or positive examples provided")
	}

	if len(target) != vc.Dimension {
		return nil, ErrDimensionMismatch
	}

	// Perform initial search
	ef := params.Ef
	if ef == 0 {
		ef = 100
	}

	// Search with higher ef to get more candidates for re-ranking
	searchK := params.K * 3
	if searchK < 100 {
		searchK = 100
	}

	var candidates []hnsw.Candidate
	var err error

	// Perform graph-native context discovery instead of searching target and post-processing
	hnswContext := make([]hnsw.ContextPair, len(positiveVectors))
	// Pad/truncate negatives to match positives length (or simply pair them up up to the max length)
	// Actually, context distance can evaluate un-paired pos/negs.
	// For simplicity, let's just make pairs.
	pairCount := len(positiveVectors)
	if len(negativeVectors) > pairCount {
		pairCount = len(negativeVectors)
	}

	for i := 0; i < pairCount; i++ {
		pair := hnsw.ContextPair{}
		if i < len(positiveVectors) {
			pair.Positive = positiveVectors[i]
		}
		if i < len(negativeVectors) {
			pair.Negative = negativeVectors[i]
		}
		hnswContext = append(hnswContext, pair)
	}

	searchEf := ef
	if searchEf < params.K*3 {
		searchEf = params.K * 3
	}

	discoverParams := &hnsw.DiscoverParams{
		Target:  target,
		Context: hnswContext,
		K:       searchK, // fetch extra just in case
		Ef:      searchEf,
	}

	if params.Filter != nil {
		evaluator := payload.NewEvaluator()
		discoverParams.Filter = func(id string, pl map[string]interface{}) bool {
			return evaluator.Evaluate(params.Filter, pl)
		}
	}

	candidates, err = idx.DiscoverWithFilter(discoverParams)

	if err != nil {
		return nil, err
	}

	// Re-rank using context
	type scoredCandidate struct {
		candidate hnsw.Candidate
		score     float64
	}

	scoredCandidates := make([]scoredCandidate, 0, len(candidates))

	for _, cand := range candidates {
		p, err := idx.Get(idx.GetPointID(cand.ID))
		if err != nil {
			continue
		}

		vec := p.Vector
		if vc.Metric == config.MetricCosine {
			vec = distance.Normalize(vec)
		}

		// Compute context-aware score
		score := c.computeDiscoveryScore(vec, target, positiveVectors, negativeVectors, distCalc)
		scoredCandidates = append(scoredCandidates, scoredCandidate{
			candidate: cand,
			score:     score,
		})
	}

	// Sort by score (higher is better)
	sort.Slice(scoredCandidates, func(i, j int) bool {
		return scoredCandidates[i].score > scoredCandidates[j].score
	})

	// Limit results
	if len(scoredCandidates) > params.K {
		scoredCandidates = scoredCandidates[:params.K]
	}

	// Build result
	result := &DiscoveryResult{
		Points: make([]ScoredPointV2, 0, len(scoredCandidates)),
		TookMs: time.Since(start).Milliseconds(),
	}

	for _, sc := range scoredCandidates {
		p, err := idx.Get(idx.GetPointID(sc.candidate.ID))
		if err != nil {
			continue
		}

		sp := ScoredPointV2{
			ID:         p.ID,
			Score:      float32(sc.score),
			VectorName: params.VectorName,
		}

		if params.WithVector {
			sp.Vector = p.Vector
		}
		if params.WithPayload {
			sp.Payload = p.Payload
		}

		result.Points = append(result.Points, sp)
	}

	return result, nil
}

// resolveContextExample converts a context example to a vector
func (c *Collection) resolveContextExample(idx *hnsw.HNSW, ex ContextExample) (point.Vector, error) {
	if len(ex.Vector) > 0 {
		return ex.Vector, nil
	}

	if ex.ID != "" {
		p, err := idx.Get(ex.ID)
		if err != nil {
			return nil, err
		}
		return p.Vector, nil
	}

	return nil, CollectionError("context example must have either id or vector")
}

// computeDiscoveryScore computes the discovery score for a candidate
func (c *Collection) computeDiscoveryScore(
	vec, target point.Vector,
	positive, negative []point.Vector,
	distCalc distance.Calculator,
) float64 {
	// Base score from target similarity
	targetDist := distCalc.Distance(vec, target)
	score := 1.0 - float64(targetDist)

	// Boost score based on similarity to positive examples
	if len(positive) > 0 {
		var posScore float64
		for _, pv := range positive {
			dist := distCalc.Distance(vec, pv)
			posScore += 1.0 - float64(dist)
		}
		score += posScore / float64(len(positive)) * 0.5 // Weight positive context
	}

	// Reduce score based on similarity to negative examples
	if len(negative) > 0 {
		var negScore float64
		for _, nv := range negative {
			dist := distCalc.Distance(vec, nv)
			negScore += 1.0 - float64(dist)
		}
		score -= negScore / float64(len(negative)) * 0.5 // Weight negative context
	}

	return score
}

// computeCentroid computes the centroid of a set of vectors
func computeCentroid(vectors []point.Vector, dimension int) point.Vector {
	if len(vectors) == 0 {
		return nil
	}

	centroid := make(point.Vector, dimension)
	for _, v := range vectors {
		for i := 0; i < len(v) && i < dimension; i++ {
			centroid[i] += v[i]
		}
	}

	n := float32(len(vectors))
	for i := range centroid {
		centroid[i] /= n
	}

	return centroid
}

// RecommendParams holds parameters for recommendation queries
type RecommendParams struct {
	// Positive point IDs - find similar points
	Positive []string `json:"positive"`

	// Negative point IDs - find dissimilar points
	Negative []string `json:"negative,omitempty"`

	// Strategy for combining vectors
	Strategy RecommendStrategy `json:"strategy,omitempty"`

	// Standard search parameters
	K          int             `json:"limit"`
	Ef         int             `json:"ef,omitempty"`
	Filter     *payload.Filter `json:"filter,omitempty"`
	VectorName string          `json:"vector_name,omitempty"`

	// Result options
	WithVector  bool `json:"with_vector,omitempty"`
	WithPayload bool `json:"with_payload,omitempty"`
}

// RecommendStrategy defines how to combine positive/negative examples
type RecommendStrategy string

const (
	// StrategyAverageVector averages positive vectors and subtracts average of negative
	StrategyAverageVector RecommendStrategy = "average_vector"

	// StrategyBestScore finds candidates scoring well against all positive examples
	StrategyBestScore RecommendStrategy = "best_score"
)

// RecommendV2 performs recommendation based on positive and negative point IDs
func (c *Collection) RecommendV2(params *RecommendParams) (*DiscoveryResult, error) {
	if len(params.Positive) == 0 {
		return nil, CollectionError("at least one positive example required")
	}

	// Convert to discovery params
	discoveryParams := &DiscoveryParams{
		K:           params.K,
		Ef:          params.Ef,
		Filter:      params.Filter,
		VectorName:  params.VectorName,
		WithVector:  params.WithVector,
		WithPayload: params.WithPayload,
		Context: &DiscoveryContext{
			Positive: make([]ContextExample, len(params.Positive)),
			Negative: make([]ContextExample, len(params.Negative)),
		},
	}

	for i, id := range params.Positive {
		discoveryParams.Context.Positive[i] = ContextExample{ID: id}
	}

	for i, id := range params.Negative {
		discoveryParams.Context.Negative[i] = ContextExample{ID: id}
	}

	return c.Discover(discoveryParams)
}

// GroupSearchParams holds parameters for grouped search
type GroupSearchParams struct {
	Query      point.Vector    `json:"vector"`
	GroupBy    string          `json:"group_by"`   // Payload field to group by
	GroupSize  int             `json:"group_size"` // Results per group
	Limit      int             `json:"limit"`      // Total groups to return
	Filter     *payload.Filter `json:"filter,omitempty"`
	VectorName string          `json:"vector_name,omitempty"`
	WithVector bool            `json:"with_vector,omitempty"`
}

// GroupSearchResult holds grouped search results
type GroupSearchResult struct {
	Groups []SearchGroup `json:"groups"`
	TookMs int64         `json:"took_ms"`
}

// SearchGroup represents a group of search results
type SearchGroup struct {
	GroupValue interface{}     `json:"group_value"`
	Points     []ScoredPointV2 `json:"points"`
}

// GroupSearch performs search with results grouped by a payload field
func (c *Collection) GroupSearch(params *GroupSearchParams) (*GroupSearchResult, error) {
	start := time.Now()

	c.mu.RLock()
	defer c.mu.RUnlock()

	if params.GroupSize <= 0 {
		params.GroupSize = 1
	}
	if params.Limit <= 0 {
		params.Limit = 10
	}

	// Get the appropriate index and config
	var idx *hnsw.HNSW
	var vc *config.VectorConfig

	if c.config.HasNamedVectors() {
		vectorName := params.VectorName
		if vectorName == "" {
			vectorName = "default"
		}
		var ok bool
		idx, ok = c.indices[vectorName]
		if !ok {
			return nil, CollectionError("unknown vector name: " + vectorName)
		}
		vc = c.config.GetVectorConfig(vectorName)
	} else {
		idx = c.index
		vc = c.config.GetVectorConfig("")
	}

	if vc == nil {
		return nil, CollectionError("vector config not found")
	}

	query := params.Query
	if len(query) != vc.Dimension {
		return nil, ErrDimensionMismatch
	}

	if vc.Metric == config.MetricCosine {
		query = distance.Normalize(query)
	}

	// Search with enough candidates to fill groups
	searchK := params.Limit * params.GroupSize * 3
	if searchK < 100 {
		searchK = 100
	}

	var candidates []hnsw.Candidate
	var err error

	if params.Filter != nil {
		hnswParams := &hnsw.SearchParams{
			K:  searchK,
			Ef: 100,
		}
		evaluator := payload.NewEvaluator()
		hnswParams.Filter = func(id string, pl map[string]interface{}) bool {
			return evaluator.Evaluate(params.Filter, pl)
		}
		candidates, err = idx.SearchWithFilter(query, hnswParams)
	} else {
		candidates, err = idx.SearchWithEf(query, searchK, 100)
	}

	if err != nil {
		return nil, err
	}

	// Group results
	groups := make(map[interface{}][]ScoredPointV2)
	groupOrder := make([]interface{}, 0)

	for _, cand := range candidates {
		p, err := idx.Get(idx.GetPointID(cand.ID))
		if err != nil {
			continue
		}

		// Get group value
		var groupVal interface{}
		if p.Payload != nil {
			groupVal = p.Payload[params.GroupBy]
		}
		if groupVal == nil {
			groupVal = "__none__"
		}

		// Check if group is full
		if len(groups[groupVal]) >= params.GroupSize {
			continue
		}

		// Track group order
		if _, exists := groups[groupVal]; !exists {
			if len(groupOrder) >= params.Limit {
				continue // Already have enough groups
			}
			groupOrder = append(groupOrder, groupVal)
		}

		sp := ScoredPointV2{
			ID:         p.ID,
			Score:      1.0 - cand.Distance,
			VectorName: params.VectorName,
		}
		if params.WithVector {
			sp.Vector = p.Vector
		}

		groups[groupVal] = append(groups[groupVal], sp)
	}

	// Build result
	result := &GroupSearchResult{
		Groups: make([]SearchGroup, 0, len(groupOrder)),
		TookMs: time.Since(start).Milliseconds(),
	}

	for _, gv := range groupOrder {
		result.Groups = append(result.Groups, SearchGroup{
			GroupValue: gv,
			Points:     groups[gv],
		})
	}

	return result, nil
}
