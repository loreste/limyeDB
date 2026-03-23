package tuning

import (
	"testing"

	"github.com/limyedb/limyedb/pkg/config"
)

func TestPresetConfig(t *testing.T) {
	presets := []Preset{
		PresetSmall,
		PresetMedium,
		PresetLarge,
		PresetHuge,
		PresetAccuracy,
		PresetSpeed,
		PresetBalanced,
		PresetLowMemory,
		PresetRealtime,
	}

	for _, preset := range presets {
		cfg := PresetConfig(preset)
		if cfg == nil {
			t.Errorf("PresetConfig(%s) returned nil", preset)
			continue
		}

		if cfg.M < 4 || cfg.M > 64 {
			t.Errorf("Preset %s: M=%d out of range", preset, cfg.M)
		}
		if cfg.EfConstruction < cfg.M {
			t.Errorf("Preset %s: EfConstruction=%d < M=%d", preset, cfg.EfConstruction, cfg.M)
		}
		if cfg.EfSearch < 10 {
			t.Errorf("Preset %s: EfSearch=%d too low", preset, cfg.EfSearch)
		}
	}
}

func TestAutoTune(t *testing.T) {
	tests := []struct {
		name   string
		params AutoTuneParams
	}{
		{
			name: "small dataset",
			params: AutoTuneParams{
				VectorCount:     10000,
				Dimension:       128,
				TargetRecall:    0.95,
				TargetLatencyMs: 10,
			},
		},
		{
			name: "medium dataset",
			params: AutoTuneParams{
				VectorCount:     500000,
				Dimension:       256,
				TargetRecall:    0.99,
				TargetLatencyMs: 20,
			},
		},
		{
			name: "large dataset with memory constraint",
			params: AutoTuneParams{
				VectorCount:       5000000,
				Dimension:         512,
				TargetRecall:      0.95,
				TargetLatencyMs:   50,
				AvailableMemoryMB: 8000,
			},
		},
		{
			name: "high dimension",
			params: AutoTuneParams{
				VectorCount:     100000,
				Dimension:       1536,
				TargetRecall:    0.98,
				TargetLatencyMs: 30,
			},
		},
		{
			name: "low latency requirement",
			params: AutoTuneParams{
				VectorCount:     100000,
				Dimension:       128,
				TargetRecall:    0.90,
				TargetLatencyMs: 5,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AutoTune(tt.params)

			if result == nil {
				t.Fatal("AutoTune returned nil")
			}

			// Check reasonable M
			if result.M < 4 || result.M > 64 {
				t.Errorf("M=%d out of reasonable range", result.M)
			}

			// Check reasonable efConstruction
			if result.EfConstruction < result.M {
				t.Errorf("EfConstruction=%d < M=%d", result.EfConstruction, result.M)
			}

			// Check reasonable efSearch
			if result.EfSearch < 10 {
				t.Errorf("EfSearch=%d too low", result.EfSearch)
			}

			// Check memory estimate is positive
			if result.EstimatedMemoryMB <= 0 {
				t.Errorf("EstimatedMemoryMB=%d should be positive", result.EstimatedMemoryMB)
			}

			// Check recall is in valid range
			if result.EstimatedRecall < 0.5 || result.EstimatedRecall > 1.0 {
				t.Errorf("EstimatedRecall=%f out of range", result.EstimatedRecall)
			}

			// Check explanation is not empty
			if result.Explanation == "" {
				t.Error("Explanation should not be empty")
			}

			t.Logf("%s: M=%d, efConstruction=%d, efSearch=%d, quantization=%s, memory=%dMB, recall=%.2f%%",
				tt.name, result.M, result.EfConstruction, result.EfSearch,
				result.Quantization, result.EstimatedMemoryMB, result.EstimatedRecall*100)
		})
	}
}

func TestAutoTuneQuantization(t *testing.T) {
	// Test that quantization is enabled when memory is constrained
	params := AutoTuneParams{
		VectorCount:       1000000,
		Dimension:         512,
		TargetRecall:      0.95,
		AvailableMemoryMB: 500, // Very constrained
	}

	result := AutoTune(params)

	if result.Quantization == "none" && !result.OnDisk {
		t.Error("Expected quantization or on-disk storage for constrained memory")
	}

	t.Logf("Quantization: %s, OnDisk: %v", result.Quantization, result.OnDisk)
}

func TestOptimizeRuntime(t *testing.T) {
	cfg := OptimizeRuntime()

	if cfg == nil {
		t.Fatal("OptimizeRuntime returned nil")
	}

	if cfg.WorkerPoolSize < 1 {
		t.Errorf("WorkerPoolSize=%d should be >= 1", cfg.WorkerPoolSize)
	}

	if cfg.BatchSearchSize < 1 {
		t.Errorf("BatchSearchSize=%d should be >= 1", cfg.BatchSearchSize)
	}

	t.Logf("Runtime config: workers=%d, batch=%d, AVX=%v, NEON=%v",
		cfg.WorkerPoolSize, cfg.BatchSearchSize, cfg.UseAVX, cfg.UseNEON)
}

func TestValidateParams(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config.HNSWConfig
		dimension   int
		vectorCount int
		wantRecs    int // Minimum expected recommendations
	}{
		{
			name: "valid config",
			cfg: &config.HNSWConfig{
				M:              16,
				EfConstruction: 200,
				EfSearch:       100,
			},
			dimension:   128,
			vectorCount: 100000,
			wantRecs:    0,
		},
		{
			name: "M too low",
			cfg: &config.HNSWConfig{
				M:              2,
				EfConstruction: 50,
				EfSearch:       50,
			},
			dimension:   128,
			vectorCount: 100000,
			wantRecs:    1,
		},
		{
			name: "efConstruction too low",
			cfg: &config.HNSWConfig{
				M:              16,
				EfConstruction: 8, // Less than M
				EfSearch:       50,
			},
			dimension:   128,
			vectorCount: 100000,
			wantRecs:    1,
		},
		{
			name: "high dimension needs higher M",
			cfg: &config.HNSWConfig{
				M:              12,
				EfConstruction: 100,
				EfSearch:       50,
			},
			dimension:   1536,
			vectorCount: 100000,
			wantRecs:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recs := ValidateParams(tt.cfg, tt.dimension, tt.vectorCount)

			if len(recs) < tt.wantRecs {
				t.Errorf("Expected at least %d recommendations, got %d", tt.wantRecs, len(recs))
			}

			for _, rec := range recs {
				t.Logf("Recommendation: [%s] %s: %s", rec.Severity, rec.Field, rec.Message)
			}
		})
	}
}

func TestEstimateMemory(t *testing.T) {
	tests := []struct {
		name         string
		dimension    int
		vectorCount  int
		M            int
		quantization string
	}{
		{
			name:         "no quantization",
			dimension:    128,
			vectorCount:  100000,
			M:            16,
			quantization: "none",
		},
		{
			name:         "scalar quantization",
			dimension:    128,
			vectorCount:  100000,
			M:            16,
			quantization: "scalar",
		},
		{
			name:         "binary quantization",
			dimension:    128,
			vectorCount:  100000,
			M:            16,
			quantization: "binary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			estimate := EstimateMemory(tt.dimension, tt.vectorCount, tt.M, tt.quantization)

			if estimate.TotalMemoryMB <= 0 {
				t.Error("TotalMemoryMB should be positive")
			}

			if estimate.VectorMemoryMB < 0 {
				t.Error("VectorMemoryMB should not be negative")
			}

			t.Logf("%s: vectors=%dMB, index=%dMB, payload=%dMB, total=%dMB, per_vector=%d bytes",
				tt.name, estimate.VectorMemoryMB, estimate.IndexMemoryMB,
				estimate.PayloadMemoryMB, estimate.TotalMemoryMB, estimate.PerVectorBytes)
		})
	}

	// Test that quantization reduces memory
	noQuant := EstimateMemory(128, 100000, 16, "none")
	scalar := EstimateMemory(128, 100000, 16, "scalar")
	binary := EstimateMemory(128, 100000, 16, "binary")

	if scalar.VectorMemoryMB >= noQuant.VectorMemoryMB {
		t.Error("Scalar quantization should use less memory")
	}

	if binary.VectorMemoryMB >= scalar.VectorMemoryMB {
		t.Error("Binary quantization should use less memory than scalar")
	}
}

func BenchmarkAutoTune(b *testing.B) {
	params := AutoTuneParams{
		VectorCount:       1000000,
		Dimension:         512,
		TargetRecall:      0.95,
		TargetLatencyMs:   20,
		AvailableMemoryMB: 8000,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		AutoTune(params)
	}
}
