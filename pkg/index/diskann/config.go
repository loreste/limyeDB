package diskann

import (
	"github.com/limyedb/limyedb/pkg/config"
)

// Config holds DiskANN configuration parameters
type Config struct {
	// R is the maximum degree (number of neighbors) for each node
	R int `json:"r"`

	// L is the search list size during index construction
	L int `json:"l"`

	// Alpha is the diversity factor for RobustPrune (typically 1.0-1.5)
	Alpha float32 `json:"alpha"`

	// Dimension is the vector dimension
	Dimension int `json:"dimension"`

	// MaxElements is the maximum number of elements in the index
	MaxElements int `json:"max_elements"`

	// Metric specifies the distance metric to use
	Metric config.MetricType `json:"metric"`

	// DataPath is the path for storing graph data on disk
	DataPath string `json:"data_path"`

	// UseMemoryMap enables memory-mapped file storage
	UseMemoryMap bool `json:"use_memory_map"`
}

// DefaultConfig returns a default DiskANN configuration
func DefaultConfig() *Config {
	return &Config{
		R:            64,  // Default max neighbors per node
		L:            100, // Default search list size during construction
		Alpha:        1.2, // Default diversity factor
		Dimension:    0,   // Must be set
		MaxElements:  1000000,
		Metric:       config.MetricCosine,
		UseMemoryMap: false,
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Dimension <= 0 {
		return ErrInvalidDimension
	}
	if c.R < 1 {
		return ErrInvalidR
	}
	if c.L < 1 {
		return ErrInvalidL
	}
	if c.Alpha < 1.0 {
		return ErrInvalidAlpha
	}
	return nil
}
