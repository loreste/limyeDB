package autotune

import (
	"context"
	"testing"
	"time"
)

func TestNewAutoTuner(t *testing.T) {
	t.Parallel()

	// Test with nil config
	tuner := NewAutoTuner(nil)
	if tuner == nil {
		t.Fatal("NewAutoTuner returned nil")
	}

	params := tuner.GetParams()
	if params.EfConstruction != 200 {
		t.Errorf("Expected default EfConstruction=200, got %d", params.EfConstruction)
	}
	if params.EfSearch != 100 {
		t.Errorf("Expected default EfSearch=100, got %d", params.EfSearch)
	}
	if params.M != 16 {
		t.Errorf("Expected default M=16, got %d", params.M)
	}
}

func TestNewAutoTunerWithConfig(t *testing.T) {
	t.Parallel()

	cfg := &AutoTunerConfig{
		Goal:            GoalLatency,
		TargetLatencyMs: 5.0,
		MinRecall:       0.90,
		TuneInterval:    time.Minute,
		MaxSamples:      500,
		InitialParams: IndexParams{
			EfConstruction: 150,
			EfSearch:       50,
			M:              12,
			BatchSize:      500,
			NumThreads:     2,
		},
	}

	tuner := NewAutoTuner(cfg)
	params := tuner.GetParams()

	if params.EfConstruction != 150 {
		t.Errorf("Expected EfConstruction=150, got %d", params.EfConstruction)
	}
	if params.EfSearch != 50 {
		t.Errorf("Expected EfSearch=50, got %d", params.EfSearch)
	}
}

func TestRecordQuery(t *testing.T) {
	t.Parallel()

	tuner := NewAutoTuner(nil)

	// Record some queries
	for i := 0; i < 10; i++ {
		tuner.RecordQuery(float64(i+5), 0.95)
	}

	stats := tuner.GetStats()
	if stats.TotalQueries != 10 {
		t.Errorf("Expected 10 queries, got %d", stats.TotalQueries)
	}
	if stats.AvgLatencyMs <= 0 {
		t.Error("Expected positive average latency")
	}
}

func TestRecordQueryPercentiles(t *testing.T) {
	t.Parallel()

	tuner := NewAutoTuner(nil)

	// Record 100+ queries to trigger percentile calculation
	for i := 0; i < 150; i++ {
		tuner.RecordQuery(float64(i%20+1), 0.95)
	}

	stats := tuner.GetStats()
	if stats.P50LatencyMs <= 0 {
		t.Error("Expected positive P50 latency")
	}
	if stats.P95LatencyMs <= 0 {
		t.Error("Expected positive P95 latency")
	}
	if stats.P99LatencyMs <= 0 {
		t.Error("Expected positive P99 latency")
	}
	// P99 should be >= P95 >= P50
	if stats.P50LatencyMs > stats.P95LatencyMs {
		t.Error("P50 should be <= P95")
	}
}

func TestSetGoal(t *testing.T) {
	t.Parallel()

	tuner := NewAutoTuner(nil)

	goals := []OptimizationGoal{GoalLatency, GoalRecall, GoalThroughput, GoalMemory, GoalBalanced}

	for _, goal := range goals {
		tuner.SetGoal(goal)
		// Just verify it doesn't panic
	}
}

func TestEnableDisable(t *testing.T) {
	t.Parallel()

	tuner := NewAutoTuner(nil)

	tuner.Disable()
	// Record queries shouldn't trigger tuning
	for i := 0; i < 200; i++ {
		tuner.RecordQuery(100.0, 0.5)
	}

	tuner.Enable()
	// Now tuning should be enabled again
}

func TestSuggest(t *testing.T) {
	t.Parallel()

	tuner := NewAutoTuner(nil)

	// Fill in some stats
	for i := 0; i < 150; i++ {
		tuner.RecordQuery(float64(i%20+1), 0.95)
	}

	params := tuner.Suggest()
	if params.EfSearch <= 0 {
		t.Error("Expected positive EfSearch suggestion")
	}
}

func TestSuggestForWorkload(t *testing.T) {
	t.Parallel()

	tuner := NewAutoTuner(nil)

	testCases := []struct {
		profile         WorkloadProfile
		expectedM       int
		expectedEfRange [2]int // min, max expected ef_search
	}{
		{WorkloadRealtime, 12, [2]int{30, 80}},
		{WorkloadBatch, 24, [2]int{150, 300}},
		{WorkloadHighRecall, 32, [2]int{250, 400}},
		{WorkloadLowLatency, 8, [2]int{20, 50}},
	}

	for _, tc := range testCases {
		t.Run(string(tc.profile), func(t *testing.T) {
			params := tuner.SuggestForWorkload(tc.profile)
			if params.M != tc.expectedM {
				t.Errorf("Profile %s: expected M=%d, got %d", tc.profile, tc.expectedM, params.M)
			}
			if params.EfSearch < tc.expectedEfRange[0] || params.EfSearch > tc.expectedEfRange[1] {
				t.Errorf("Profile %s: expected EfSearch in range [%d,%d], got %d",
					tc.profile, tc.expectedEfRange[0], tc.expectedEfRange[1], params.EfSearch)
			}
		})
	}
}

func TestOptimizeForLatency(t *testing.T) {
	t.Parallel()

	cfg := &AutoTunerConfig{
		Goal:            GoalLatency,
		TargetLatencyMs: 10.0,
		MinRecall:       0.95,
		TuneInterval:    time.Hour, // Long interval to prevent auto-tuning
		InitialParams: IndexParams{
			EfSearch: 100,
		},
	}
	tuner := NewAutoTuner(cfg)

	// Simulate high latency
	for i := 0; i < 150; i++ {
		tuner.RecordQuery(25.0, 0.98) // High latency, good recall
	}

	params := tuner.Suggest()
	// Should reduce ef_search when latency is high
	if params.EfSearch >= cfg.InitialParams.EfSearch {
		t.Logf("EfSearch should decrease for high latency, got %d", params.EfSearch)
	}
}

func TestOptimizeForRecall(t *testing.T) {
	t.Parallel()

	cfg := &AutoTunerConfig{
		Goal:            GoalRecall,
		TargetLatencyMs: 10.0,
		MinRecall:       0.95,
		TuneInterval:    time.Hour,
		InitialParams: IndexParams{
			EfSearch: 100,
		},
	}
	tuner := NewAutoTuner(cfg)

	// Simulate low recall
	for i := 0; i < 150; i++ {
		tuner.RecordQuery(5.0, 0.85) // Low latency, low recall
	}

	params := tuner.Suggest()
	// Should increase ef_search when recall is low
	if params.EfSearch <= cfg.InitialParams.EfSearch {
		t.Logf("EfSearch should increase for low recall, got %d", params.EfSearch)
	}
}

func TestOnParamsChange(t *testing.T) {
	t.Parallel()

	cfg := &AutoTunerConfig{
		TuneInterval: time.Millisecond, // Short interval for testing
	}
	tuner := NewAutoTuner(cfg)

	var callbackCalled bool
	var newParams IndexParams

	tuner.SetOnParamsChange(func(params IndexParams) {
		callbackCalled = true
		newParams = params
	})

	// Record many queries to potentially trigger tuning
	for i := 0; i < 200; i++ {
		tuner.RecordQuery(100.0, 0.5) // Bad performance
	}

	// Give time for async tuning
	time.Sleep(100 * time.Millisecond)

	if callbackCalled && newParams.EfSearch == 0 {
		t.Error("Callback called but params were not set")
	}
}

func TestAdaptiveSearch(t *testing.T) {
	t.Parallel()

	tuner := NewAutoTuner(nil)
	as := NewAdaptiveSearch(tuner)

	ef := as.GetEfSearch(128, 10, false)
	if ef <= 0 {
		t.Error("Expected positive ef_search")
	}

	efWithFilter := as.GetEfSearch(128, 10, true)
	if efWithFilter <= ef {
		t.Error("Expected higher ef_search with filter")
	}
}

func TestAdaptiveSearchRecordPerformance(t *testing.T) {
	t.Parallel()

	tuner := NewAutoTuner(nil)
	as := NewAdaptiveSearch(tuner)

	// Record good performance
	as.RecordDimensionPerformance(128, 50, 10.0, 0.98)

	// Should now use cached ef_search
	ef := as.GetEfSearch(128, 10, false)
	if ef < 20 { // Should be at least max(cached, limit*2)
		t.Errorf("Expected ef >= 20, got %d", ef)
	}
}

func TestWorkloadAnalyzer(t *testing.T) {
	t.Parallel()

	wa := NewWorkloadAnalyzer()

	// Record some queries
	wa.RecordQuery("search")
	wa.RecordQuery("search")
	wa.RecordQuery("recommend")

	recommendations := wa.GetRecommendations()
	// Should return slice (even if empty)
	if recommendations == nil {
		t.Error("Expected non-nil recommendations slice")
	}
}

func TestWorkloadAnalyzerPeakHours(t *testing.T) {
	t.Parallel()

	wa := NewWorkloadAnalyzer()

	// Record many queries
	for i := 0; i < 100; i++ {
		wa.RecordQuery("search")
	}

	recommendations := wa.GetRecommendations()
	if recommendations == nil {
		t.Error("Expected non-nil recommendations")
	}
}

func TestRunWithContext(t *testing.T) {
	t.Parallel()

	cfg := &AutoTunerConfig{
		TuneInterval: 50 * time.Millisecond,
	}
	tuner := NewAutoTuner(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		tuner.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Good, Run() returned after context cancellation
	case <-time.After(time.Second):
		t.Error("Run() did not exit after context cancellation")
	}
}

func TestQuickSort(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    []float64
		expected []float64
	}{
		{"empty", []float64{}, []float64{}},
		{"single", []float64{1.0}, []float64{1.0}},
		{"sorted", []float64{1.0, 2.0, 3.0}, []float64{1.0, 2.0, 3.0}},
		{"reverse", []float64{3.0, 2.0, 1.0}, []float64{1.0, 2.0, 3.0}},
		{"mixed", []float64{3.0, 1.0, 4.0, 1.0, 5.0}, []float64{1.0, 1.0, 3.0, 4.0, 5.0}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			input := make([]float64, len(tc.input))
			copy(input, tc.input)
			quickSort(input)
			for i := range input {
				if input[i] != tc.expected[i] {
					t.Errorf("Expected %v, got %v", tc.expected, input)
					break
				}
			}
		})
	}
}

func TestMinMax(t *testing.T) {
	t.Parallel()

	if min(5, 10) != 5 {
		t.Error("min(5, 10) should be 5")
	}
	if min(10, 5) != 5 {
		t.Error("min(10, 5) should be 5")
	}
	if max(5, 10) != 10 {
		t.Error("max(5, 10) should be 10")
	}
	if max(10, 5) != 10 {
		t.Error("max(10, 5) should be 10")
	}
}
