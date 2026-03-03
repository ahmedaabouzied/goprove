package analysis

import (
	"go/types"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntervalForType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		kind     types.BasicKind
		wantIv   Interval
		wantOk   bool
		wantLo   int64
		wantHi   int64
	}{
		// Supported signed integer types
		{"int8", types.Int8, NewInterval(math.MinInt8, math.MaxInt8), true, math.MinInt8, math.MaxInt8},
		{"int16", types.Int16, NewInterval(math.MinInt16, math.MaxInt16), true, math.MinInt16, math.MaxInt16},
		{"int32", types.Int32, NewInterval(math.MinInt32, math.MaxInt32), true, math.MinInt32, math.MaxInt32},

		// Types we don't track (can't detect overflow with int64 internals)
		{"int64", types.Int64, NewInterval(math.MinInt64, math.MaxInt64), false, 0, 0},
		{"int", types.Int, Bottom(), false, 0, 0},

		// Unsigned integers — not tracked
		{"uint", types.Uint, Bottom(), false, 0, 0},
		{"uint8", types.Uint8, Bottom(), false, 0, 0},
		{"uint16", types.Uint16, Bottom(), false, 0, 0},
		{"uint32", types.Uint32, Bottom(), false, 0, 0},
		{"uint64", types.Uint64, Bottom(), false, 0, 0},
		{"uintptr", types.Uintptr, Bottom(), false, 0, 0},

		// Non-integer types — not tracked
		{"bool", types.Bool, Bottom(), false, 0, 0},
		{"string", types.String, Bottom(), false, 0, 0},
		{"float32", types.Float32, Bottom(), false, 0, 0},
		{"float64", types.Float64, Bottom(), false, 0, 0},
		{"complex64", types.Complex64, Bottom(), false, 0, 0},
		{"complex128", types.Complex128, Bottom(), false, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := IntervalForType(tt.kind)
			require.Equal(t, tt.wantOk, ok, "IntervalForType(%v) ok", tt.kind)
			if !tt.wantOk {
				return
			}
			require.True(t, got.Equals(tt.wantIv), "IntervalForType(%v) = %+v, want %+v", tt.kind, got, tt.wantIv)
			require.Equal(t, tt.wantLo, got.Lo, "Lo bound")
			require.Equal(t, tt.wantHi, got.Hi, "Hi bound")
			require.False(t, got.IsTop, "should not be Top")
			require.False(t, got.IsBottom, "should not be Bottom")
		})
	}
}

func TestIntervalForTypeBoundsAreExact(t *testing.T) {
	t.Parallel()

	// Verify the bounds match Go's actual type limits
	t.Run("int8 bounds match", func(t *testing.T) {
		t.Parallel()
		iv, ok := IntervalForType(types.Int8)
		require.True(t, ok)
		require.Equal(t, int64(int8(iv.Lo)), iv.Lo, "Lo should survive int8 round-trip")
		require.Equal(t, int64(int8(iv.Hi)), iv.Hi, "Hi should survive int8 round-trip")
		// One beyond the bounds should NOT survive round-trip
		require.NotEqual(t, int64(int8(iv.Lo-1)), iv.Lo-1, "Lo-1 should overflow int8")
		require.NotEqual(t, int64(int8(iv.Hi+1)), iv.Hi+1, "Hi+1 should overflow int8")
	})

	t.Run("int16 bounds match", func(t *testing.T) {
		t.Parallel()
		iv, ok := IntervalForType(types.Int16)
		require.True(t, ok)
		require.Equal(t, int64(int16(iv.Lo)), iv.Lo, "Lo should survive int16 round-trip")
		require.Equal(t, int64(int16(iv.Hi)), iv.Hi, "Hi should survive int16 round-trip")
		require.NotEqual(t, int64(int16(iv.Lo-1)), iv.Lo-1, "Lo-1 should overflow int16")
		require.NotEqual(t, int64(int16(iv.Hi+1)), iv.Hi+1, "Hi+1 should overflow int16")
	})

	t.Run("int32 bounds match", func(t *testing.T) {
		t.Parallel()
		iv, ok := IntervalForType(types.Int32)
		require.True(t, ok)
		require.Equal(t, int64(int32(iv.Lo)), iv.Lo, "Lo should survive int32 round-trip")
		require.Equal(t, int64(int32(iv.Hi)), iv.Hi, "Hi should survive int32 round-trip")
		require.NotEqual(t, int64(int32(iv.Lo-1)), iv.Lo-1, "Lo-1 should overflow int32")
		require.NotEqual(t, int64(int32(iv.Hi+1)), iv.Hi+1, "Hi+1 should overflow int32")
	})
}

func TestIntervalForTypeContainment(t *testing.T) {
	t.Parallel()

	// int8 range is fully contained within int16 range, etc.
	iv8, _ := IntervalForType(types.Int8)
	iv16, _ := IntervalForType(types.Int16)
	iv32, _ := IntervalForType(types.Int32)

	t.Run("int8 contained in int16", func(t *testing.T) {
		t.Parallel()
		require.GreaterOrEqual(t, iv8.Lo, iv16.Lo)
		require.LessOrEqual(t, iv8.Hi, iv16.Hi)
	})

	t.Run("int16 contained in int32", func(t *testing.T) {
		t.Parallel()
		require.GreaterOrEqual(t, iv16.Lo, iv32.Lo)
		require.LessOrEqual(t, iv16.Hi, iv32.Hi)
	})

	// Meet of a wider type with narrower should give narrower
	t.Run("meet int16 with int8 bounds gives int8", func(t *testing.T) {
		t.Parallel()
		met := iv16.Meet(iv8)
		require.True(t, met.Equals(iv8), "Meet(int16, int8) should give int8 bounds")
	})
}
