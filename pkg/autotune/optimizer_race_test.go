package autotune

import (
	"sync"
	"testing"
	"time"
)

// TestRaceRecordQuery tests concurrent RecordQuery calls
func TestRaceRecordQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	tuner := NewAutoTuner(nil)

	const goroutines = 20
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				tuner.RecordQuery(float64(i+1), 0.95)
			}
		}(g)
	}

	wg.Wait()

	stats := tuner.GetStats()
	expectedQueries := int64(goroutines * opsPerGoroutine)
	if stats.TotalQueries != expectedQueries {
		t.Errorf("Expected %d queries, got %d", expectedQueries, stats.TotalQueries)
	}
}

// TestRaceGetParams tests concurrent GetParams calls with RecordQuery
func TestRaceGetParams(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	tuner := NewAutoTuner(nil)

	const goroutines = 10
	const ops = 200

	var wg sync.WaitGroup

	// Writers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				tuner.RecordQuery(float64(i+1), 0.95)
			}
		}()
	}

	// Readers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				_ = tuner.GetParams()
				_ = tuner.GetStats()
			}
		}()
	}

	wg.Wait()
}

// TestRaceSetGoal tests concurrent SetGoal calls
func TestRaceSetGoal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	tuner := NewAutoTuner(nil)

	const goroutines = 10
	const ops = 100

	goals := []OptimizationGoal{GoalLatency, GoalRecall, GoalThroughput, GoalMemory, GoalBalanced}

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				tuner.SetGoal(goals[i%len(goals)])
				tuner.RecordQuery(float64(i+1), 0.95)
			}
		}(g)
	}

	wg.Wait()
}

// TestRaceEnableDisable tests concurrent Enable/Disable calls
func TestRaceEnableDisable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	tuner := NewAutoTuner(nil)

	const goroutines = 10
	const ops = 100

	var wg sync.WaitGroup

	// Toggle enable/disable
	wg.Add(goroutines / 2)
	for g := 0; g < goroutines/2; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				if i%2 == 0 {
					tuner.Enable()
				} else {
					tuner.Disable()
				}
			}
		}()
	}

	// Record queries
	wg.Add(goroutines / 2)
	for g := 0; g < goroutines/2; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				tuner.RecordQuery(float64(i+1), 0.95)
			}
		}()
	}

	wg.Wait()
}

// TestRaceAdaptiveSearch tests concurrent AdaptiveSearch operations
func TestRaceAdaptiveSearch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	tuner := NewAutoTuner(nil)
	as := NewAdaptiveSearch(tuner)

	const goroutines = 10
	const ops = 100

	var wg sync.WaitGroup

	// Writers
	wg.Add(goroutines / 2)
	for g := 0; g < goroutines/2; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				as.RecordDimensionPerformance(128*(id%4+1), 50+i, float64(i+1), 0.96)
			}
		}(g)
	}

	// Readers
	wg.Add(goroutines / 2)
	for g := 0; g < goroutines/2; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				_ = as.GetEfSearch(128*(id%4+1), 10, i%2 == 0)
			}
		}(g)
	}

	wg.Wait()
}

// TestRaceWorkloadAnalyzer tests concurrent WorkloadAnalyzer operations
func TestRaceWorkloadAnalyzer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	wa := NewWorkloadAnalyzer()

	const goroutines = 10
	const ops = 100

	patterns := []string{"search", "recommend", "insert", "delete", "update"}

	var wg sync.WaitGroup

	// Writers
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				wa.RecordQuery(patterns[i%len(patterns)])
			}
		}(g)
	}

	// Readers
	wg.Add(goroutines / 2)
	for g := 0; g < goroutines/2; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				_ = wa.GetRecommendations()
			}
		}()
	}

	wg.Wait()
}

// TestRaceSuggest tests concurrent Suggest calls
func TestRaceSuggest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	tuner := NewAutoTuner(nil)

	// Populate some data
	for i := 0; i < 150; i++ {
		tuner.RecordQuery(float64(i+1), 0.95)
	}

	const goroutines = 10
	const ops = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				_ = tuner.Suggest()
			}
		}()
	}

	wg.Wait()
}

// TestRaceSuggestForWorkload tests concurrent SuggestForWorkload calls
func TestRaceSuggestForWorkload(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	tuner := NewAutoTuner(nil)

	profiles := []WorkloadProfile{WorkloadRealtime, WorkloadBatch, WorkloadHighRecall, WorkloadLowLatency}

	const goroutines = 10
	const ops = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				_ = tuner.SuggestForWorkload(profiles[i%len(profiles)])
			}
		}()
	}

	wg.Wait()
}

// TestRaceOnParamsChange tests concurrent SetOnParamsChange and tuning
func TestRaceOnParamsChange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	t.Parallel()

	cfg := &AutoTunerConfig{
		TuneInterval: time.Millisecond,
	}
	tuner := NewAutoTuner(cfg)

	const goroutines = 5
	const ops = 50

	var wg sync.WaitGroup

	// Set callbacks
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				tuner.SetOnParamsChange(func(params IndexParams) {
					_ = params.EfSearch
				})
			}
		}()
	}

	// Record queries to trigger tuning
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				tuner.RecordQuery(float64(i+1), 0.95)
			}
		}()
	}

	wg.Wait()
}
