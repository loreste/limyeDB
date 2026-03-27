package tuning

import (
	"runtime"

	"github.com/limyedb/limyedb/pkg/config"
)

// =============================================================================
// Presets for Common Use Cases
// =============================================================================

// Preset represents a named configuration preset
type Preset string

const (
	// PresetSmall optimized for <100K vectors, single node
	PresetSmall Preset = "small"

	// PresetMedium optimized for 100K-1M vectors
	PresetMedium Preset = "medium"

	// PresetLarge optimized for 1M-10M vectors
	PresetLarge Preset = "large"

	// PresetHuge optimized for >10M vectors, requires cluster
	PresetHuge Preset = "huge"

	// PresetAccuracy prioritizes search quality over speed
	PresetAccuracy Preset = "accuracy"

	// PresetSpeed prioritizes search speed over quality
	PresetSpeed Preset = "speed"

	// PresetBalanced balances speed and accuracy
	PresetBalanced Preset = "balanced"

	// PresetLowMemory optimizes for minimal memory usage
	PresetLowMemory Preset = "low_memory"

	// PresetRealtime optimizes for real-time applications
	PresetRealtime Preset = "realtime"
)

// PresetConfig returns HNSW configuration for a preset
func PresetConfig(preset Preset) *config.HNSWConfig {
	switch preset {
	case PresetSmall:
		return &config.HNSWConfig{
			M:              16,
			EfConstruction: 100,
			EfSearch:       50,
			MaxElements:    100000,
		}
	case PresetMedium:
		return &config.HNSWConfig{
			M:              16,
			EfConstruction: 200,
			EfSearch:       100,
			MaxElements:    1000000,
		}
	case PresetLarge:
		return &config.HNSWConfig{
			M:              24,
			EfConstruction: 400,
			EfSearch:       150,
			MaxElements:    10000000,
		}
	case PresetHuge:
		return &config.HNSWConfig{
			M:              32,
			EfConstruction: 500,
			EfSearch:       200,
			MaxElements:    100000000,
		}
	case PresetAccuracy:
		return &config.HNSWConfig{
			M:              32,
			EfConstruction: 500,
			EfSearch:       500,
			MaxElements:    1000000,
		}
	case PresetSpeed:
		return &config.HNSWConfig{
			M:              8,
			EfConstruction: 100,
			EfSearch:       20,
			MaxElements:    1000000,
		}
	case PresetBalanced:
		return &config.HNSWConfig{
			M:              16,
			EfConstruction: 200,
			EfSearch:       100,
			MaxElements:    1000000,
		}
	case PresetLowMemory:
		return &config.HNSWConfig{
			M:              8,
			EfConstruction: 100,
			EfSearch:       50,
			MaxElements:    1000000,
		}
	case PresetRealtime:
		return &config.HNSWConfig{
			M:              12,
			EfConstruction: 150,
			EfSearch:       30,
			MaxElements:    1000000,
		}
	default:
		return PresetConfig(PresetBalanced)
	}
}

// =============================================================================
// Auto-Tuning Engine
// =============================================================================

// AutoTuneParams holds parameters for auto-tuning
type AutoTuneParams struct {
	VectorCount       int     // Expected number of vectors
	Dimension         int     // Vector dimension
	TargetRecall      float64 // Target recall (0.0-1.0)
	TargetLatencyMs   int     // Target p99 latency in ms
	AvailableMemoryMB int     // Available memory in MB
	CPUCores          int     // Available CPU cores
}

// AutoTuneResult holds the result of auto-tuning
type AutoTuneResult struct {
	M                 int     `json:"m"`
	EfConstruction    int     `json:"ef_construction"`
	EfSearch          int     `json:"ef_search"`
	Quantization      string  `json:"quantization"` // "none", "scalar", "binary"
	OnDisk            bool    `json:"on_disk"`
	EstimatedMemoryMB int     `json:"estimated_memory_mb"`
	EstimatedRecall   float64 `json:"estimated_recall"`
	Explanation       string  `json:"explanation"`
}

// AutoTune automatically determines optimal parameters
func AutoTune(params AutoTuneParams) *AutoTuneResult {
	if params.CPUCores == 0 {
		params.CPUCores = runtime.NumCPU()
	}

	result := &AutoTuneResult{}

	// Calculate base memory requirements
	// Memory per vector: dim * 4 bytes (float32) + HNSW overhead
	vectorMemory := params.Dimension * 4
	hnswOverhead := 200 // Approximate bytes per node for links

	// Determine M based on target recall
	if params.TargetRecall >= 0.99 {
		result.M = 32
	} else if params.TargetRecall >= 0.95 {
		result.M = 24
	} else if params.TargetRecall >= 0.90 {
		result.M = 16
	} else {
		result.M = 12
	}

	// Adjust M based on dimension (higher dimension benefits from higher M)
	if params.Dimension > 512 {
		result.M += 4
	}
	if params.Dimension > 1024 {
		result.M += 4
	}

	// Calculate efConstruction based on M
	result.EfConstruction = result.M * 10
	if result.EfConstruction > 500 {
		result.EfConstruction = 500
	}

	// Calculate efSearch based on latency target
	if params.TargetLatencyMs <= 5 {
		result.EfSearch = 30
	} else if params.TargetLatencyMs <= 20 {
		result.EfSearch = 100
	} else if params.TargetLatencyMs <= 50 {
		result.EfSearch = 200
	} else {
		result.EfSearch = 500
	}

	// Ensure efSearch is at least 10
	if result.EfSearch < 10 {
		result.EfSearch = 10
	}

	// Calculate estimated memory
	memoryPerVector := vectorMemory + hnswOverhead + (result.M * 2 * 4) // links
	totalMemoryBytes := int64(memoryPerVector) * int64(params.VectorCount)
	totalMemoryMB := int(totalMemoryBytes / (1024 * 1024))

	// Determine if quantization is needed
	result.Quantization = "none"
	if params.AvailableMemoryMB > 0 && totalMemoryMB > params.AvailableMemoryMB {
		// Try scalar quantization first (4x reduction)
		if totalMemoryMB/4 <= params.AvailableMemoryMB {
			result.Quantization = "scalar"
			totalMemoryMB = totalMemoryMB / 4
		} else if totalMemoryMB/32 <= params.AvailableMemoryMB {
			// Binary quantization (32x reduction)
			result.Quantization = "binary"
			totalMemoryMB = totalMemoryMB / 32
		} else {
			// Still too big, enable on-disk
			result.OnDisk = true
			totalMemoryMB = totalMemoryMB / 10 // Rough estimate of memory-mapped usage
		}
	}

	result.EstimatedMemoryMB = totalMemoryMB

	// Estimate recall based on parameters
	result.EstimatedRecall = estimateRecall(result.M, result.EfSearch, params.VectorCount)

	// Build explanation
	result.Explanation = buildExplanation(result, params)

	return result
}

// estimateRecall estimates recall based on HNSW parameters
func estimateRecall(M, efSearch, vectorCount int) float64 {
	// Empirical formula based on HNSW benchmarks
	// Higher M and efSearch generally lead to higher recall
	base := 0.85

	// M contribution
	mFactor := float64(M) / 16.0 * 0.05
	if mFactor > 0.1 {
		mFactor = 0.1
	}

	// efSearch contribution
	efFactor := float64(efSearch) / 100.0 * 0.03
	if efFactor > 0.05 {
		efFactor = 0.05
	}

	// Size penalty (larger datasets are harder)
	sizePenalty := 0.0
	if vectorCount > 1000000 {
		sizePenalty = 0.02
	}
	if vectorCount > 10000000 {
		sizePenalty = 0.05
	}

	recall := base + mFactor + efFactor - sizePenalty
	if recall > 0.999 {
		recall = 0.999
	}
	return recall
}

// buildExplanation builds a human-readable explanation
func buildExplanation(result *AutoTuneResult, params AutoTuneParams) string {
	exp := "Auto-tuned configuration:\n"

	exp += "- M=" + itoa(result.M) + ": "
	if result.M >= 24 {
		exp += "High connectivity for better recall\n"
	} else if result.M >= 16 {
		exp += "Balanced connectivity\n"
	} else {
		exp += "Lower connectivity for faster search\n"
	}

	exp += "- efConstruction=" + itoa(result.EfConstruction) + ": "
	if result.EfConstruction >= 400 {
		exp += "Thorough index building\n"
	} else {
		exp += "Fast index building\n"
	}

	exp += "- efSearch=" + itoa(result.EfSearch) + ": "
	if result.EfSearch >= 200 {
		exp += "High quality search\n"
	} else if result.EfSearch >= 50 {
		exp += "Balanced search quality/speed\n"
	} else {
		exp += "Fast search\n"
	}

	if result.Quantization != "none" {
		exp += "- Quantization: " + result.Quantization + " (reduces memory)\n"
	}

	if result.OnDisk {
		exp += "- On-disk storage enabled (dataset exceeds memory)\n"
	}

	exp += "\nEstimated memory: " + itoa(result.EstimatedMemoryMB) + " MB"
	exp += "\nEstimated recall: " + ftoa(result.EstimatedRecall)

	return exp
}

// =============================================================================
// Runtime Optimization
// =============================================================================

// RuntimeConfig holds runtime optimization settings
type RuntimeConfig struct {
	WorkerPoolSize   int  `json:"worker_pool_size"`
	BatchSearchSize  int  `json:"batch_search_size"`
	PrefetchDistance int  `json:"prefetch_distance"`
	UseAVX           bool `json:"use_avx"`
	UseNEON          bool `json:"use_neon"`
	ParallelInsert   bool `json:"parallel_insert"`
	AsyncCommit      bool `json:"async_commit"`
}

// OptimizeRuntime returns optimized runtime configuration
func OptimizeRuntime() *RuntimeConfig {
	cpus := runtime.NumCPU()

	return &RuntimeConfig{
		WorkerPoolSize:   cpus,
		BatchSearchSize:  100,
		PrefetchDistance: 4,
		UseAVX:           runtime.GOARCH == "amd64",
		UseNEON:          runtime.GOARCH == "arm64",
		ParallelInsert:   cpus > 4,
		AsyncCommit:      true,
	}
}

// =============================================================================
// Parameter Validation and Recommendation
// =============================================================================

// ValidateParams validates HNSW parameters and returns recommendations
func ValidateParams(cfg *config.HNSWConfig, dimension, vectorCount int) []Recommendation {
	var recs []Recommendation

	// Check M
	if cfg.M < 4 {
		recs = append(recs, Recommendation{
			Severity:  SeverityWarning,
			Field:     "M",
			Message:   "M is very low, recall may suffer",
			Suggested: 16,
		})
	} else if cfg.M > 64 {
		recs = append(recs, Recommendation{
			Severity:  SeverityWarning,
			Field:     "M",
			Message:   "M is very high, memory usage may be excessive",
			Suggested: 32,
		})
	}

	// Check efConstruction
	if cfg.EfConstruction < cfg.M {
		recs = append(recs, Recommendation{
			Severity:  SeverityError,
			Field:     "EfConstruction",
			Message:   "efConstruction should be >= M",
			Suggested: cfg.M * 10,
		})
	}

	// Check efSearch
	if cfg.EfSearch < 10 {
		recs = append(recs, Recommendation{
			Severity:  SeverityWarning,
			Field:     "EfSearch",
			Message:   "efSearch is very low, recall may suffer",
			Suggested: 50,
		})
	}

	// Check dimension-specific recommendations
	if dimension > 1024 && cfg.M < 24 {
		recs = append(recs, Recommendation{
			Severity:  SeverityInfo,
			Field:     "M",
			Message:   "High-dimensional vectors benefit from higher M",
			Suggested: 24,
		})
	}

	// Check size-specific recommendations
	if vectorCount > 10000000 && cfg.EfConstruction < 400 {
		recs = append(recs, Recommendation{
			Severity:  SeverityInfo,
			Field:     "EfConstruction",
			Message:   "Large collections benefit from higher efConstruction",
			Suggested: 400,
		})
	}

	return recs
}

// Recommendation represents a tuning recommendation
type Recommendation struct {
	Severity  Severity `json:"severity"`
	Field     string   `json:"field"`
	Message   string   `json:"message"`
	Suggested int      `json:"suggested,omitempty"`
}

// Severity represents recommendation severity
type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// =============================================================================
// Memory Estimation
// =============================================================================

// EstimateMemory estimates memory usage for a configuration
func EstimateMemory(dimension, vectorCount, M int, quantization string) MemoryEstimate {
	// Base vector memory
	vectorSize := dimension * 4 // float32

	// Apply quantization
	switch quantization {
	case "scalar":
		vectorSize = dimension // int8
	case "binary":
		vectorSize = (dimension + 7) / 8 // bits to bytes
	case "pq":
		vectorSize = 8 // typical PQ encoding
	}

	// HNSW overhead per node
	hnswOverhead := 8 + (M * 4 * 2) // ID + bidirectional links

	// Total per vector
	perVector := vectorSize + hnswOverhead

	// Add payload estimate (rough)
	payloadEstimate := 100 // bytes per point

	totalBytes := int64(perVector+payloadEstimate) * int64(vectorCount)

	return MemoryEstimate{
		VectorMemoryMB:  int(int64(vectorSize*vectorCount) / (1024 * 1024)),
		IndexMemoryMB:   int(int64(hnswOverhead*vectorCount) / (1024 * 1024)),
		PayloadMemoryMB: int(int64(payloadEstimate*vectorCount) / (1024 * 1024)),
		TotalMemoryMB:   int(totalBytes / (1024 * 1024)),
		PerVectorBytes:  perVector + payloadEstimate,
	}
}

// MemoryEstimate holds memory usage estimates
type MemoryEstimate struct {
	VectorMemoryMB  int `json:"vector_memory_mb"`
	IndexMemoryMB   int `json:"index_memory_mb"`
	PayloadMemoryMB int `json:"payload_memory_mb"`
	TotalMemoryMB   int `json:"total_memory_mb"`
	PerVectorBytes  int `json:"per_vector_bytes"`
}

// =============================================================================
// Helper functions
// =============================================================================

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}

func ftoa(f float64) string {
	// Simple float to string for percentages
	percent := int(f * 1000)
	return itoa(percent/10) + "." + itoa(percent%10) + "%"
}
