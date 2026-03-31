package diskann

import "errors"

// Errors for DiskANN operations
var (
	// ErrDimensionMismatch is returned when the vector dimension doesn't match the index dimension
	ErrDimensionMismatch = errors.New("vector dimension mismatch")

	// ErrPointNotFound is returned when a point is not found in the index
	ErrPointNotFound = errors.New("point not found")

	// ErrPointExists is returned when trying to insert a point that already exists
	ErrPointExists = errors.New("point already exists")

	// ErrEmptyIndex is returned when searching an empty index
	ErrEmptyIndex = errors.New("index is empty")

	// ErrInvalidDimension is returned when dimension is invalid
	ErrInvalidDimension = errors.New("dimension must be positive")

	// ErrInvalidR is returned when R parameter is invalid
	ErrInvalidR = errors.New("R (max degree) must be at least 1")

	// ErrInvalidL is returned when L parameter is invalid
	ErrInvalidL = errors.New("L (search list size) must be at least 1")

	// ErrInvalidAlpha is returned when alpha parameter is invalid
	ErrInvalidAlpha = errors.New("alpha must be at least 1.0")

	// ErrInvalidK is returned when k parameter is invalid
	ErrInvalidK = errors.New("k must be positive")

	// ErrMaxNodesExceeded is returned when the index exceeds max capacity
	ErrMaxNodesExceeded = errors.New("maximum number of nodes exceeded")
)
