package safecasts

import (
	"errors"
	"math"
)

// ErrOverflow indicates an integer overflow would occur
var ErrOverflow = errors.New("integer overflow")

// IntToUint32 safely converts int to uint32
func IntToUint32(v int) (uint32, error) {
	if v < 0 || v > math.MaxUint32 {
		return 0, ErrOverflow
	}
	return uint32(v), nil
}

// IntToUint32Safe converts int to uint32, returning 0 on overflow
func IntToUint32Safe(v int) uint32 {
	if v < 0 || v > math.MaxUint32 {
		return 0
	}
	return uint32(v)
}

// IntToUint16 safely converts int to uint16
func IntToUint16(v int) (uint16, error) {
	if v < 0 || v > math.MaxUint16 {
		return 0, ErrOverflow
	}
	return uint16(v), nil
}

// IntToUint16Safe converts int to uint16, returning 0 on overflow
func IntToUint16Safe(v int) uint16 {
	if v < 0 || v > math.MaxUint16 {
		return 0
	}
	return uint16(v)
}

// IntToInt32 safely converts int to int32
func IntToInt32(v int) (int32, error) {
	if v < math.MinInt32 || v > math.MaxInt32 {
		return 0, ErrOverflow
	}
	return int32(v), nil
}

// IntToInt32Safe converts int to int32, clamping on overflow
func IntToInt32Safe(v int) int32 {
	if v < math.MinInt32 {
		return math.MinInt32
	}
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(v)
}

// IntToUint64 safely converts int to uint64
func IntToUint64(v int) (uint64, error) {
	if v < 0 {
		return 0, ErrOverflow
	}
	return uint64(v), nil
}

// IntToUint64Safe converts int to uint64, returning 0 on negative
func IntToUint64Safe(v int) uint64 {
	if v < 0 {
		return 0
	}
	return uint64(v)
}

// Int64ToUint64 safely converts int64 to uint64
func Int64ToUint64(v int64) (uint64, error) {
	if v < 0 {
		return 0, ErrOverflow
	}
	return uint64(v), nil
}

// Int64ToUint64Safe converts int64 to uint64, returning 0 on negative
func Int64ToUint64Safe(v int64) uint64 {
	if v < 0 {
		return 0
	}
	return uint64(v)
}

// Uint64ToInt64 safely converts uint64 to int64
func Uint64ToInt64(v uint64) (int64, error) {
	if v > math.MaxInt64 {
		return 0, ErrOverflow
	}
	return int64(v), nil
}

// Uint64ToInt64Safe converts uint64 to int64, clamping on overflow
func Uint64ToInt64Safe(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}

// Uint64ToInt safely converts uint64 to int
func Uint64ToInt(v uint64) (int, error) {
	if v > math.MaxInt {
		return 0, ErrOverflow
	}
	return int(v), nil
}

// Uint64ToIntSafe converts uint64 to int, clamping on overflow
func Uint64ToIntSafe(v uint64) int {
	if v > math.MaxInt {
		return math.MaxInt
	}
	return int(v)
}

// Int64ToInt safely converts int64 to int (for 32-bit systems)
func Int64ToInt(v int64) (int, error) {
	if v < math.MinInt || v > math.MaxInt {
		return 0, ErrOverflow
	}
	return int(v), nil
}

// Int64ToIntSafe converts int64 to int, clamping on overflow
func Int64ToIntSafe(v int64) int {
	if v < math.MinInt {
		return math.MinInt
	}
	if v > math.MaxInt {
		return math.MaxInt
	}
	return int(v)
}
