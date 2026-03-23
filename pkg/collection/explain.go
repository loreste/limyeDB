package collection

import (
	"time"

	"github.com/limyedb/limyedb/pkg/config"
	"github.com/limyedb/limyedb/pkg/index/payload"
	"github.com/limyedb/limyedb/pkg/point"
)

// QueryPlan represents the execution plan for a query
type QueryPlan struct {
	// Query type
	QueryType string `json:"query_type"`

	// Index information
	IndexInfo *IndexPlanInfo `json:"index_info"`

	// Filter plan
	FilterPlan *FilterPlanInfo `json:"filter_plan,omitempty"`

	// Estimated cost
	EstimatedCost *CostEstimate `json:"estimated_cost"`

	// Actual execution stats (only if executed)
	ExecutionStats *ExecutionStats `json:"execution_stats,omitempty"`
}

// IndexPlanInfo describes the index usage in the query plan
type IndexPlanInfo struct {
	IndexType   string `json:"index_type"`    // "hnsw", "flat", etc.
	VectorName  string `json:"vector_name"`
	Dimension   int    `json:"dimension"`
	Metric      string `json:"metric"`
	TotalPoints int64  `json:"total_points"`

	// HNSW specific
	M              int `json:"m,omitempty"`
	EfConstruction int `json:"ef_construction,omitempty"`
	EfSearch       int `json:"ef_search,omitempty"`
}

// FilterPlanInfo describes filter execution plan
type FilterPlanInfo struct {
	FilterType     string              `json:"filter_type"`      // "none", "pre", "post"
	IndexesUsed    []string            `json:"indexes_used"`     // Payload indexes that can be used
	EstimatedMatch float64             `json:"estimated_match"`  // Estimated fraction of points matching
	Conditions     []*ConditionPlan    `json:"conditions"`       // Breakdown of conditions
}

// ConditionPlan describes a single filter condition
type ConditionPlan struct {
	Field          string  `json:"field"`
	Operator       string  `json:"operator"`
	IndexAvailable bool    `json:"index_available"`
	Selectivity    float64 `json:"selectivity"` // Estimated fraction of points matching
}

// CostEstimate provides estimated query cost
type CostEstimate struct {
	// Estimated number of distance calculations
	DistanceCalculations int64 `json:"distance_calculations"`

	// Estimated number of points to scan
	PointsToScan int64 `json:"points_to_scan"`

	// Estimated memory usage in bytes
	MemoryBytes int64 `json:"memory_bytes"`

	// Overall cost score (lower is better)
	CostScore float64 `json:"cost_score"`
}

// ExecutionStats holds actual execution statistics
type ExecutionStats struct {
	// Timing
	TotalTimeMs      int64 `json:"total_time_ms"`
	IndexTimeMs      int64 `json:"index_time_ms"`
	FilterTimeMs     int64 `json:"filter_time_ms"`
	PostProcessingMs int64 `json:"post_processing_ms"`

	// Counts
	DistanceCalculations int64 `json:"distance_calculations"`
	PointsScanned        int64 `json:"points_scanned"`
	PointsFiltered       int64 `json:"points_filtered"`
	ResultsReturned      int64 `json:"results_returned"`
}

// ExplainParams holds parameters for explain query
type ExplainParams struct {
	// Query parameters
	Query      point.Vector    `json:"vector,omitempty"`
	K          int             `json:"limit"`
	Ef         int             `json:"ef,omitempty"`
	Filter     *payload.Filter `json:"filter,omitempty"`
	VectorName string          `json:"vector_name,omitempty"`

	// If true, actually execute the query and include stats
	Analyze bool `json:"analyze,omitempty"`
}

// Explain returns the query plan without executing
func (c *Collection) Explain(params *ExplainParams) (*QueryPlan, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	plan := &QueryPlan{
		QueryType: "vector_search",
	}

	// Get index info
	var vc *config.VectorConfig
	var totalPoints int64

	if c.config.HasNamedVectors() {
		vectorName := params.VectorName
		if vectorName == "" {
			vectorName = "default"
		}
		vc = c.config.GetVectorConfig(vectorName)
		if idx, ok := c.indices[vectorName]; ok {
			totalPoints = idx.Size()
		}
	} else {
		vc = c.config.GetVectorConfig("")
		totalPoints = c.index.Size()
	}

	if vc == nil {
		return nil, CollectionError("vector config not found")
	}

	plan.IndexInfo = &IndexPlanInfo{
		IndexType:      "hnsw",
		VectorName:     params.VectorName,
		Dimension:      vc.Dimension,
		Metric:         string(vc.Metric),
		TotalPoints:    totalPoints,
		M:              vc.HNSW.M,
		EfConstruction: vc.HNSW.EfConstruction,
		EfSearch:       vc.HNSW.EfSearch,
	}

	// Analyze filter
	if params.Filter != nil {
		plan.FilterPlan = c.analyzeFilter(params.Filter, totalPoints)
	}

	// Estimate cost
	plan.EstimatedCost = c.estimateCost(params, totalPoints, plan.FilterPlan)

	// If analyze is requested, execute the query
	if params.Analyze && len(params.Query) > 0 {
		stats, err := c.executeWithStats(params)
		if err != nil {
			return nil, err
		}
		plan.ExecutionStats = stats
	}

	return plan, nil
}

// analyzeFilter analyzes a filter and returns execution plan
func (c *Collection) analyzeFilter(filter *payload.Filter, totalPoints int64) *FilterPlanInfo {
	plan := &FilterPlanInfo{
		FilterType:     "post",  // Default to post-filtering
		EstimatedMatch: 1.0,     // Default: matches everything
		Conditions:     []*ConditionPlan{},
	}

	// Check if any payload indexes can be used
	indexedFields := c.payloadIndex.IndexedFields()
	indexSet := make(map[string]bool)
	for _, f := range indexedFields {
		indexSet[f] = true
	}

	// Analyze each condition
	conditions := extractFilterConditions(filter)
	for _, cond := range conditions {
		cp := &ConditionPlan{
			Field:          cond.field,
			Operator:       cond.op,
			IndexAvailable: indexSet[cond.field],
			Selectivity:    cond.selectivity,
		}
		plan.Conditions = append(plan.Conditions, cp)
		plan.EstimatedMatch *= cond.selectivity

		if cp.IndexAvailable {
			plan.IndexesUsed = append(plan.IndexesUsed, cond.field)
			plan.FilterType = "pre" // Can use pre-filtering
		}
	}

	return plan
}

type filterCondition struct {
	field       string
	op          string
	selectivity float64
}

// extractFilterConditions extracts conditions from a filter
func extractFilterConditions(filter *payload.Filter) []filterCondition {
	if filter == nil {
		return nil
	}

	var conditions []filterCondition

	// Handle different filter types
	switch filter.Type {
	case payload.FilterMatch:
		conditions = append(conditions, filterCondition{
			field:       filter.Field,
			op:          "match",
			selectivity: 0.1, // Estimate: 10% selectivity
		})
	case payload.FilterRange:
		conditions = append(conditions, filterCondition{
			field:       filter.Field,
			op:          "range",
			selectivity: 0.3, // Estimate: 30% selectivity
		})
	case payload.FilterIsNull:
		conditions = append(conditions, filterCondition{
			field:       filter.Field,
			op:          "is_null",
			selectivity: 0.05, // Estimate: 5% null values
		})
	case payload.FilterIsNotNull:
		conditions = append(conditions, filterCondition{
			field:       filter.Field,
			op:          "is_not_null",
			selectivity: 0.95, // Estimate: 95% non-null
		})
	case payload.FilterAnd:
		for _, sub := range filter.Conditions {
			conditions = append(conditions, extractFilterConditions(sub)...)
		}
	case payload.FilterOr:
		for _, sub := range filter.Conditions {
			conditions = append(conditions, extractFilterConditions(sub)...)
		}
	}

	return conditions
}

// estimateCost estimates the cost of executing a query
func (c *Collection) estimateCost(params *ExplainParams, totalPoints int64, filterPlan *FilterPlanInfo) *CostEstimate {
	cost := &CostEstimate{}

	// EfSearch determines how many nodes we visit
	ef := params.Ef
	if ef == 0 {
		ef = 100
	}

	// Estimated distance calculations based on HNSW algorithm
	// Approximate: ef * log(totalPoints) * M
	if totalPoints > 0 {
		logN := 1.0
		for n := totalPoints; n > 1; n /= 2 {
			logN++
		}
		cost.DistanceCalculations = int64(float64(ef) * logN * 16) // M=16 default
	}

	// Points to scan depends on filtering
	if filterPlan != nil {
		cost.PointsToScan = int64(float64(totalPoints) * filterPlan.EstimatedMatch)
	} else {
		cost.PointsToScan = int64(ef) // Only ef points in HNSW search
	}

	// Memory: visited set + candidate heap + result heap
	cost.MemoryBytes = cost.DistanceCalculations*8 + // visited set (8 bytes per node)
		int64(ef)*32 + // candidate heap
		int64(params.K)*32 // result heap

	// Overall cost score
	cost.CostScore = float64(cost.DistanceCalculations) + float64(cost.PointsToScan)*0.1

	return cost
}

// executeWithStats executes a query and returns execution statistics
func (c *Collection) executeWithStats(params *ExplainParams) (*ExecutionStats, error) {
	stats := &ExecutionStats{}
	totalStart := time.Now()

	// Execute the search
	searchStart := time.Now()
	result, err := c.SearchV2WithParams(params.Query, params.VectorName, &SearchParams{
		K:      params.K,
		Ef:     params.Ef,
		Filter: params.Filter,
	})
	stats.IndexTimeMs = time.Since(searchStart).Milliseconds()

	if err != nil {
		return nil, err
	}

	stats.ResultsReturned = int64(len(result.Points))
	stats.TotalTimeMs = time.Since(totalStart).Milliseconds()

	// Estimate other stats (actual values would require instrumentation)
	stats.PointsScanned = stats.ResultsReturned * 10 // Rough estimate
	stats.DistanceCalculations = stats.PointsScanned * 2

	return stats, nil
}

// QueryStats returns statistics about queries
type QueryStats struct {
	TotalQueries     int64   `json:"total_queries"`
	AvgLatencyMs     float64 `json:"avg_latency_ms"`
	P50LatencyMs     float64 `json:"p50_latency_ms"`
	P95LatencyMs     float64 `json:"p95_latency_ms"`
	P99LatencyMs     float64 `json:"p99_latency_ms"`
	QueriesPerSecond float64 `json:"queries_per_second"`
}

// Optimizer provides query optimization hints
type Optimizer struct {
	collection *Collection
}

// NewOptimizer creates a new query optimizer
func NewOptimizer(c *Collection) *Optimizer {
	return &Optimizer{collection: c}
}

// SuggestIndexes suggests payload indexes to create
func (o *Optimizer) SuggestIndexes(queryLog []*ExplainParams) []string {
	fieldCounts := make(map[string]int)

	for _, q := range queryLog {
		if q.Filter != nil {
			conditions := extractFilterConditions(q.Filter)
			for _, c := range conditions {
				fieldCounts[c.field]++
			}
		}
	}

	// Find fields that are frequently filtered but not indexed
	indexedFields := o.collection.payloadIndex.IndexedFields()
	indexSet := make(map[string]bool)
	for _, f := range indexedFields {
		indexSet[f] = true
	}

	var suggestions []string
	for field, count := range fieldCounts {
		if !indexSet[field] && count > len(queryLog)/10 {
			suggestions = append(suggestions, field)
		}
	}

	return suggestions
}

// OptimalEfSearch suggests optimal efSearch parameter
func (o *Optimizer) OptimalEfSearch(targetRecall float64) int {
	// Based on HNSW research, ef should be proportional to M and desired recall
	// Higher ef = better recall but slower search

	var m int
	if o.collection.config.HasNamedVectors() {
		for _, vc := range o.collection.config.Vectors {
			m = vc.HNSW.M
			break
		}
	} else {
		m = o.collection.config.HNSW.M
	}

	if m == 0 {
		m = 16
	}

	// Rule of thumb: ef >= k for recall > 0.9
	// For 0.99 recall, ef should be ~4-8x k
	if targetRecall >= 0.99 {
		return m * 8
	} else if targetRecall >= 0.95 {
		return m * 4
	} else if targetRecall >= 0.90 {
		return m * 2
	}
	return m
}
