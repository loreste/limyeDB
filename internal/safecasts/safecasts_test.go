package safecasts

import (
	"errors"
	"math"
	"testing"
)

// is64Bit reports whether int is 64 bits wide. A few conversions here can only
// overflow on a 32-bit platform, so those assertions are guarded.
const is64Bit = math.MaxInt == math.MaxInt64

func TestIntToUint32(t *testing.T) {
	tests := []struct {
		name    string
		in      int
		want    uint32
		wantErr bool
	}{
		{"zero", 0, 0, false},
		{"one", 1, 1, false},
		{"max", math.MaxUint32, math.MaxUint32, false},
		{"negative", -1, 0, true},
		{"above max", math.MaxUint32 + 1, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IntToUint32(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("IntToUint32(%d) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrOverflow) {
				t.Errorf("IntToUint32(%d) error = %v, want ErrOverflow", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("IntToUint32(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestIntToUint32Safe(t *testing.T) {
	// Documented behavior: 0 on overflow, not a clamp to max.
	tests := []struct {
		name string
		in   int
		want uint32
	}{
		{"zero", 0, 0},
		{"max", math.MaxUint32, math.MaxUint32},
		{"negative yields zero", -1, 0},
		{"above max yields zero", math.MaxUint32 + 1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IntToUint32Safe(tt.in); got != tt.want {
				t.Errorf("IntToUint32Safe(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestIntToUint16(t *testing.T) {
	tests := []struct {
		name    string
		in      int
		want    uint16
		wantErr bool
	}{
		{"zero", 0, 0, false},
		{"max", math.MaxUint16, math.MaxUint16, false},
		{"negative", -1, 0, true},
		{"above max", math.MaxUint16 + 1, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IntToUint16(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("IntToUint16(%d) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrOverflow) {
				t.Errorf("IntToUint16(%d) error = %v, want ErrOverflow", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("IntToUint16(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestIntToUint16Safe(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want uint16
	}{
		{"in range", 42, 42},
		{"max", math.MaxUint16, math.MaxUint16},
		{"negative yields zero", -1, 0},
		{"above max yields zero", math.MaxUint16 + 1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IntToUint16Safe(tt.in); got != tt.want {
				t.Errorf("IntToUint16Safe(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestIntToInt32(t *testing.T) {
	tests := []struct {
		name    string
		in      int
		want    int32
		wantErr bool
	}{
		{"zero", 0, 0, false},
		{"negative in range", -5, -5, false},
		{"min", math.MinInt32, math.MinInt32, false},
		{"max", math.MaxInt32, math.MaxInt32, false},
		{"below min", math.MinInt32 - 1, 0, true},
		{"above max", math.MaxInt32 + 1, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IntToInt32(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("IntToInt32(%d) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrOverflow) {
				t.Errorf("IntToInt32(%d) error = %v, want ErrOverflow", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("IntToInt32(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestIntToInt32Safe(t *testing.T) {
	// Documented behavior: clamps to the int32 range rather than returning 0.
	tests := []struct {
		name string
		in   int
		want int32
	}{
		{"in range", 7, 7},
		{"min", math.MinInt32, math.MinInt32},
		{"max", math.MaxInt32, math.MaxInt32},
		{"below min clamps", math.MinInt32 - 1, math.MinInt32},
		{"above max clamps", math.MaxInt32 + 1, math.MaxInt32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IntToInt32Safe(tt.in); got != tt.want {
				t.Errorf("IntToInt32Safe(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestIntToUint64(t *testing.T) {
	tests := []struct {
		name    string
		in      int
		want    uint64
		wantErr bool
	}{
		{"zero", 0, 0, false},
		{"positive", 12345, 12345, false},
		{"max int", math.MaxInt, uint64(math.MaxInt), false},
		{"negative", -1, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IntToUint64(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("IntToUint64(%d) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrOverflow) {
				t.Errorf("IntToUint64(%d) error = %v, want ErrOverflow", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("IntToUint64(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestIntToUint64Safe(t *testing.T) {
	if got := IntToUint64Safe(99); got != 99 {
		t.Errorf("IntToUint64Safe(99) = %d, want 99", got)
	}
	if got := IntToUint64Safe(-1); got != 0 {
		t.Errorf("IntToUint64Safe(-1) = %d, want 0", got)
	}
}

func TestInt64ToUint64(t *testing.T) {
	tests := []struct {
		name    string
		in      int64
		want    uint64
		wantErr bool
	}{
		{"zero", 0, 0, false},
		{"max int64", math.MaxInt64, uint64(math.MaxInt64), false},
		{"negative", -1, 0, true},
		{"min int64", math.MinInt64, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Int64ToUint64(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Int64ToUint64(%d) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrOverflow) {
				t.Errorf("Int64ToUint64(%d) error = %v, want ErrOverflow", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("Int64ToUint64(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestInt64ToUint64Safe(t *testing.T) {
	if got := Int64ToUint64Safe(math.MaxInt64); got != uint64(math.MaxInt64) {
		t.Errorf("Int64ToUint64Safe(MaxInt64) = %d, want %d", got, uint64(math.MaxInt64))
	}
	if got := Int64ToUint64Safe(-42); got != 0 {
		t.Errorf("Int64ToUint64Safe(-42) = %d, want 0", got)
	}
}

func TestUint64ToInt64(t *testing.T) {
	tests := []struct {
		name    string
		in      uint64
		want    int64
		wantErr bool
	}{
		{"zero", 0, 0, false},
		{"max int64", uint64(math.MaxInt64), math.MaxInt64, false},
		{"above max int64", uint64(math.MaxInt64) + 1, 0, true},
		{"max uint64", math.MaxUint64, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Uint64ToInt64(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Uint64ToInt64(%d) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrOverflow) {
				t.Errorf("Uint64ToInt64(%d) error = %v, want ErrOverflow", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("Uint64ToInt64(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestUint64ToInt64Safe(t *testing.T) {
	// Documented behavior: clamps to MaxInt64.
	if got := Uint64ToInt64Safe(123); got != 123 {
		t.Errorf("Uint64ToInt64Safe(123) = %d, want 123", got)
	}
	if got := Uint64ToInt64Safe(math.MaxUint64); got != math.MaxInt64 {
		t.Errorf("Uint64ToInt64Safe(MaxUint64) = %d, want %d", got, int64(math.MaxInt64))
	}
}

func TestUint64ToInt(t *testing.T) {
	if got, err := Uint64ToInt(0); err != nil || got != 0 {
		t.Errorf("Uint64ToInt(0) = (%d, %v), want (0, nil)", got, err)
	}
	if got, err := Uint64ToInt(4096); err != nil || got != 4096 {
		t.Errorf("Uint64ToInt(4096) = (%d, %v), want (4096, nil)", got, err)
	}
	if got, err := Uint64ToInt(uint64(math.MaxInt)); err != nil || got != math.MaxInt {
		t.Errorf("Uint64ToInt(MaxInt) = (%d, %v), want (%d, nil)", got, err, math.MaxInt)
	}

	got, err := Uint64ToInt(math.MaxUint64)
	if !errors.Is(err, ErrOverflow) {
		t.Errorf("Uint64ToInt(MaxUint64) error = %v, want ErrOverflow", err)
	}
	if got != 0 {
		t.Errorf("Uint64ToInt(MaxUint64) = %d, want 0", got)
	}
}

func TestUint64ToIntSafe(t *testing.T) {
	if got := Uint64ToIntSafe(77); got != 77 {
		t.Errorf("Uint64ToIntSafe(77) = %d, want 77", got)
	}
	if got := Uint64ToIntSafe(math.MaxUint64); got != math.MaxInt {
		t.Errorf("Uint64ToIntSafe(MaxUint64) = %d, want %d", got, math.MaxInt)
	}
}

func TestInt64ToInt(t *testing.T) {
	// On a 64-bit platform int and int64 have the same range, so no input can
	// overflow; only the pass-through path is reachable.
	for _, in := range []int64{0, 1, -1, math.MaxInt64, math.MinInt64} {
		if !is64Bit && (in == math.MaxInt64 || in == math.MinInt64) {
			continue
		}
		got, err := Int64ToInt(in)
		if err != nil {
			t.Errorf("Int64ToInt(%d) unexpected error: %v", in, err)
		}
		if int64(got) != in {
			t.Errorf("Int64ToInt(%d) = %d, want %d", in, got, in)
		}
	}
}

func TestInt64ToIntSafe(t *testing.T) {
	for _, in := range []int64{0, 1, -1, 65535} {
		if got := Int64ToIntSafe(in); int64(got) != in {
			t.Errorf("Int64ToIntSafe(%d) = %d, want %d", in, got, in)
		}
	}

	if is64Bit {
		if got := Int64ToIntSafe(math.MaxInt64); int64(got) != math.MaxInt64 {
			t.Errorf("Int64ToIntSafe(MaxInt64) = %d, want %d", got, int64(math.MaxInt64))
		}
		if got := Int64ToIntSafe(math.MinInt64); int64(got) != math.MinInt64 {
			t.Errorf("Int64ToIntSafe(MinInt64) = %d, want %d", got, int64(math.MinInt64))
		}
	}
}

func TestErrOverflowIsComparable(t *testing.T) {
	_, err := IntToUint32(-1)
	if !errors.Is(err, ErrOverflow) {
		t.Fatalf("errors.Is(err, ErrOverflow) = false, want true")
	}
}
