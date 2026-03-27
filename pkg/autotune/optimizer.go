package autotune

import (
	"context"
	"math"
	"sync"
	"time"
)

// QueryStats tracks query performance statistics
type QueryStats struct {
	TotalQueries      int64
	AvgLatencyMs      float64
	P50LatencyMs      float64
	P95LatencyMs      float64
	P99LatencyMs      float64
	AvgRecall         float64
	QueriesPerSecond  float64
	CacheHitRate      float64
	IndexSize         int64
	LastQueryTime     time.Time
}

// IndexParams holds tunable index parameters
type IndexParams struct {
	EfConstruction int     `json:"ef_construction"`
	EfSearch       int     `json:"ef_search"`
	M              int     `json:"m"`
	BatchSize      int     `json:"batch_size"`
	NumThreads     int     `json:"num_threads"`
}

// OptimizationGoal represents what to optimize for
type OptimizationGoal string

const (
	GoalLatency     OptimizationGoal = "latency"
	GoalRecall      OptimizationGoal = "recall"
	GoalThroughput  OptimizationGoal = "throughput"
	GoalBalanced    OptimizationGoal = "balanced"
	GoalMemory      OptimizationGoal = "memory"
)

// AutoTuner automatically optimizes index parameters based on workload
type AutoTuner struct {
	// Current parameters
	params IndexParams

	// Statistics
	stats      *QueryStats
	latencies  []float64
	maxSamples int

	// Configuration
	goal            OptimizationGoal
	targetLatencyMs float64
	minRecall       float64

	// Tuning state
	lastTuneTime   time.Time
	tuneInterval   time.Duration
	enabled        bool

	// Callbacks
	onParamsChange func(params IndexParams)

	mu sync.RWMutex
}

// AutoTunerConfig configures the auto-tuner
type AutoTunerConfig struct {
	Goal            OptimizationGoal
	TargetLatencyMs float64
	MinRecall       float64
	TuneInterval    time.Duration
	MaxSamples      int
	InitialParams   IndexParams
}

// DefaultAutoTunerConfig returns default configuration
func DefaultAutoTunerConfig() *AutoTunerConfig {
	return &AutoTunerConfig{
		Goal:            GoalBalanced,
		TargetLatencyMs: 10.0,
		MinRecall:       0.95,
		TuneInterval:    5 * time.Minute,
		MaxSamples:      1000,
		InitialParams: IndexParams{
			EfConstruction: 200,
			EfSearch:       100,
			M:              16,
			BatchSize:      1000,
			NumThreads:     4,
		},
	}
}

// NewAutoTuner creates a new auto-tuner
func NewAutoTuner(cfg *AutoTunerConfig) *AutoTuner {
	if cfg == nil {
		cfg = DefaultAutoTunerConfig()
	}

	return &AutoTuner{
		params:          cfg.InitialParams,
		stats:           &QueryStats{},
		latencies:       make([]float64, 0, cfg.MaxSamples),
		maxSamples:      cfg.MaxSamples,
		goal:            cfg.Goal,
		targetLatencyMs: cfg.TargetLatencyMs,
		minRecall:       cfg.MinRecall,
		tuneInterval:    cfg.TuneInterval,
		enabled:         true,
	}
}

// RecordQuery records a query for statistics
func (t *AutoTuner) RecordQuery(latencyMs float64, recall float64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Update latencies
	if len(t.latencies) >= t.maxSamples {
		t.latencies = t.latencies[1:]
	}
	t.latencies = append(t.latencies, latencyMs)

	// Update stats
	t.stats.TotalQueries++
	t.stats.LastQueryTime = time.Now()

	// Update running averages
	alpha := 0.1 // Exponential moving average factor
	t.stats.AvgLatencyMs = (1-alpha)*t.stats.AvgLatencyMs + alpha*latencyMs
	if recall > 0 {
		t.stats.AvgRecall = (1-alpha)*t.stats.AvgRecall + alpha*recall
	}

	// Recalculate percentiles periodically
	if t.stats.TotalQueries%100 == 0 {
		t.calculatePercentiles()
	}

	// Check if tuning is needed
	if t.enabled && time.Since(t.lastTuneTime) > t.tuneInterval {
		go t.tune()
	}
}

func (t *AutoTuner) calculatePercentiles() {
	if len(t.latencies) == 0 {
		return
	}

	// Sort latencies for percentile calculation
	sorted := make([]float64, len(t.latencies))
	copy(sorted, t.latencies)
	quickSort(sorted)

	n := len(sorted)
	t.stats.P50LatencyMs = sorted[n*50/100]
	t.stats.P95LatencyMs = sorted[n*95/100]
	t.stats.P99LatencyMs = sorted[min(n*99/100, n-1)]
}

// GetParams returns current index parameters
func (t *AutoTuner) GetParams() IndexParams {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.params
}

// GetStats returns current statistics
func (t *AutoTuner) GetStats() QueryStats {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return *t.stats
}

// SetGoal changes the optimization goal
func (t *AutoTuner) SetGoal(goal OptimizationGoal) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.goal = goal
}

// Enable enables auto-tuning
func (t *AutoTuner) Enable() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.enabled = true
}

// Disable disables auto-tuning
func (t *AutoTuner) Disable() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.enabled = false
}

// SetOnParamsChange sets the callback for parameter changes
func (t *AutoTuner) SetOnParamsChange(fn func(params IndexParams)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onParamsChange = fn
}

// tune performs auto-tuning based on current statistics
func (t *AutoTuner) tune() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.lastTuneTime = time.Now()

	if len(t.latencies) < 100 {
		return // Not enough data
	}

	newParams := t.params

	switch t.goal {
	case GoalLatency:
		newParams = t.optimizeForLatency()
	case GoalRecall:
		newParams = t.optimizeForRecall()
	case GoalThroughput:
		newParams = t.optimizeForThroughput()
	case GoalMemory:
		newParams = t.optimizeForMemory()
	case GoalBalanced:
		newParams = t.optimizeBalanced()
	}

	// Apply changes if different
	if newParams != t.params {
		t.params = newParams
		if t.onParamsChange != nil {
			go t.onParamsChange(newParams)
		}
	}
}

func (t *AutoTuner) optimizeForLatency() IndexParams {
	params := t.params

	// If latency is too high, reduce ef_search
	if t.stats.P95LatencyMs > t.targetLatencyMs*2 {
		params.EfSearch = max(20, params.EfSearch-20)
	} else if t.stats.P95LatencyMs > t.targetLatencyMs {
		params.EfSearch = max(20, params.EfSearch-10)
	}

	// If latency is very low and recall might be suffering, increase slightly
	if t.stats.P95LatencyMs < t.targetLatencyMs/2 && t.stats.AvgRecall < t.minRecall {
		params.EfSearch = min(500, params.EfSearch+10)
	}

	return params
}

func (t *AutoTuner) optimizeForRecall() IndexParams {
	params := t.params

	// If recall is too low, increase ef_search
	if t.stats.AvgRecall < t.minRecall {
		params.EfSearch = min(500, params.EfSearch+20)
	} else if t.stats.AvgRecall < t.minRecall+0.02 {
		params.EfSearch = min(500, params.EfSearch+10)
	}

	// If recall is high and latency is acceptable, no change needed
	// If recall is very high, we might reduce ef_search slightly
	if t.stats.AvgRecall > 0.99 && t.stats.P95LatencyMs > t.targetLatencyMs {
		params.EfSearch = max(50, params.EfSearch-5)
	}

	return params
}

func (t *AutoTuner) optimizeForThroughput() IndexParams {
	params := t.params

	// Reduce ef_search for higher throughput
	if t.stats.P95LatencyMs > t.targetLatencyMs {
		params.EfSearch = max(20, params.EfSearch-15)
	}

	// Increase batch size for better throughput
	params.BatchSize = min(10000, params.BatchSize+500)

	return params
}

func (t *AutoTuner) optimizeForMemory() IndexParams {
	params := t.params

	// Reduce M for lower memory usage (trades off search quality)
	if params.M > 8 && t.stats.AvgRecall > t.minRecall+0.05 {
		params.M = max(8, params.M-2)
	}

	// Reduce ef_construction
	if params.EfConstruction > 100 {
		params.EfConstruction = max(100, params.EfConstruction-20)
	}

	return params
}

func (t *AutoTuner) optimizeBalanced() IndexParams {
	params := t.params

	// Balance between latency and recall
	recallWeight := 0.6
	latencyWeight := 0.4

	needsMoreRecall := t.stats.AvgRecall < t.minRecall
	needsLessLatency := t.stats.P95LatencyMs > t.targetLatencyMs

	if needsMoreRecall && !needsLessLatency {
		// Can afford to increase ef_search
		adjustment := int(math.Round(10 * recallWeight))
		params.EfSearch = min(400, params.EfSearch+adjustment)
	} else if needsLessLatency && !needsMoreRecall {
		// Need to reduce latency
		adjustment := int(math.Round(10 * latencyWeight))
		params.EfSearch = max(30, params.EfSearch-adjustment)
	} else if needsMoreRecall && needsLessLatency {
		// Both need adjustment - prioritize based on weights
		if (t.minRecall-t.stats.AvgRecall)*recallWeight > (t.stats.P95LatencyMs/t.targetLatencyMs-1)*latencyWeight {
			params.EfSearch = min(400, params.EfSearch+5)
		} else {
			params.EfSearch = max(30, params.EfSearch-5)
		}
	}

	return params
}

// Suggest returns recommended parameters without applying them
func (t *AutoTuner) Suggest() IndexParams {
	t.mu.RLock()
	defer t.mu.RUnlock()

	switch t.goal {
	case GoalLatency:
		return t.optimizeForLatency()
	case GoalRecall:
		return t.optimizeForRecall()
	case GoalThroughput:
		return t.optimizeForThroughput()
	case GoalMemory:
		return t.optimizeForMemory()
	default:
		return t.optimizeBalanced()
	}
}

// SuggestForWorkload returns parameters optimized for a specific workload profile
func (t *AutoTuner) SuggestForWorkload(profile WorkloadProfile) IndexParams {
	switch profile {
	case WorkloadRealtime:
		return IndexParams{
			EfConstruction: 100,
			EfSearch:       50,
			M:              12,
			BatchSize:      100,
			NumThreads:     8,
		}
	case WorkloadBatch:
		return IndexParams{
			EfConstruction: 400,
			EfSearch:       200,
			M:              24,
			BatchSize:      10000,
			NumThreads:     4,
		}
	case WorkloadHighRecall:
		return IndexParams{
			EfConstruction: 500,
			EfSearch:       300,
			M:              32,
			BatchSize:      1000,
			NumThreads:     4,
		}
	case WorkloadLowLatency:
		return IndexParams{
			EfConstruction: 100,
			EfSearch:       30,
			M:              8,
			BatchSize:      500,
			NumThreads:     8,
		}
	default:
		return t.params
	}
}

// WorkloadProfile represents a common workload pattern
type WorkloadProfile string

const (
	WorkloadRealtime   WorkloadProfile = "realtime"
	WorkloadBatch      WorkloadProfile = "batch"
	WorkloadHighRecall WorkloadProfile = "high_recall"
	WorkloadLowLatency WorkloadProfile = "low_latency"
)

// AdaptiveSearch provides adaptive search parameters based on query characteristics
type AdaptiveSearch struct {
	tuner          *AutoTuner
	vectorDimCache map[int]int // dimension -> optimal ef_search
	mu             sync.RWMutex
}

// NewAdaptiveSearch creates a new adaptive search helper
func NewAdaptiveSearch(tuner *AutoTuner) *AdaptiveSearch {
	return &AdaptiveSearch{
		tuner:          tuner,
		vectorDimCache: make(map[int]int),
	}
}

// GetEfSearch returns the optimal ef_search for given parameters
func (as *AdaptiveSearch) GetEfSearch(dimension int, limit int, hasFilter bool) int {
	as.mu.RLock()
	baseEf, ok := as.vectorDimCache[dimension]
	as.mu.RUnlock()

	if !ok {
		baseEf = as.tuner.GetParams().EfSearch
	}

	// Adjust based on limit
	ef := max(baseEf, limit*2)

	// Increase ef when filtering (filters reduce candidates)
	if hasFilter {
		ef = int(float64(ef) * 1.5)
	}

	// Apply bounds
	ef = max(20, min(500, ef))

	return ef
}

// RecordDimensionPerformance records performance for a specific dimension
func (as *AdaptiveSearch) RecordDimensionPerformance(dimension int, efSearch int, latencyMs float64, recall float64) {
	as.mu.Lock()
	defer as.mu.Unlock()

	// Simple heuristic: if good performance, cache this ef_search
	if recall > 0.95 && latencyMs < 20 {
		as.vectorDimCache[dimension] = efSearch
	}
}

// WorkloadAnalyzer analyzes query patterns to suggest optimizations
type WorkloadAnalyzer struct {
	queryPatterns map[string]int // pattern -> count
	peakHours     map[int]int    // hour -> query count
	mu            sync.RWMutex
}

// NewWorkloadAnalyzer creates a new workload analyzer
func NewWorkloadAnalyzer() *WorkloadAnalyzer {
	return &WorkloadAnalyzer{
		queryPatterns: make(map[string]int),
		peakHours:     make(map[int]int),
	}
}

// RecordQuery records a query pattern
func (wa *WorkloadAnalyzer) RecordQuery(pattern string) {
	wa.mu.Lock()
	defer wa.mu.Unlock()

	wa.queryPatterns[pattern]++
	wa.peakHours[time.Now().Hour()]++
}

// GetRecommendations returns workload-based recommendations
func (wa *WorkloadAnalyzer) GetRecommendations() []string {
	wa.mu.RLock()
	defer wa.mu.RUnlock()

	var recommendations []string

	// Find peak hours
	var peakHour int
	var maxQueries int
	for hour, count := range wa.peakHours {
		if count > maxQueries {
			maxQueries = count
			peakHour = hour
		}
	}

	if maxQueries > 0 {
		recommendations = append(recommendations,
			"Peak traffic at hour "+string(rune('0'+peakHour))+". Consider scaling resources.")
	}

	// Analyze patterns
	for pattern, count := range wa.queryPatterns {
		if count > 1000 {
			recommendations = append(recommendations,
				"Frequent query pattern '"+pattern+"'. Consider caching.")
		}
	}

	return recommendations
}

// Run starts continuous workload monitoring
func (t *AutoTuner) Run(ctx context.Context) {
	ticker := time.NewTicker(t.tuneInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if t.enabled {
				t.tune()
			}
		}
	}
}

// Helper functions
func quickSort(arr []float64) {
	if len(arr) <= 1 {
		return
	}
	// Simple insertion sort for small arrays
	for i := 1; i < len(arr); i++ {
		for j := i; j > 0 && arr[j-1] > arr[j]; j-- {
			arr[j], arr[j-1] = arr[j-1], arr[j]
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
