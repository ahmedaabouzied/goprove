package analysis

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewInterval(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		lo   int64
		hi   int64
		want Interval
	}{
		{"normal range", 1, 10, Interval{Lo: 1, Hi: 10}},
		{"single point", 5, 5, Interval{Lo: 5, Hi: 5}},
		{"negative range", -10, -1, Interval{Lo: -10, Hi: -1}},
		{"spans zero", -5, 5, Interval{Lo: -5, Hi: 5}},
		{"zero to zero", 0, 0, Interval{Lo: 0, Hi: 0}},
		{"inverted returns bottom", 10, 5, Bottom()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := NewInterval(tt.lo, tt.hi)
			require.True(t, got.Equals(tt.want), "NewInterval(%d, %d) = %+v, want %+v", tt.lo, tt.hi, got, tt.want)
		})
	}
}

func TestContainsZero(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		interval Interval
		want     bool
	}{
		{"positive range", NewInterval(1, 10), false},
		{"negative range", NewInterval(-10, -1), false},
		{"spans zero", NewInterval(-5, 5), true},
		{"exactly zero", NewInterval(0, 0), true},
		{"lo is zero", NewInterval(0, 10), true},
		{"hi is zero", NewInterval(-10, 0), true},
		{"single positive", NewInterval(1, 1), false},
		{"single negative", NewInterval(-1, -1), false},
		{"top", Top(), true},
		{"bottom", Bottom(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, tt.interval.ContainsZero())
		})
	}
}

func TestJoin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		a    Interval
		b    Interval
		want Interval
	}{
		// Normal cases
		{"overlapping", NewInterval(1, 5), NewInterval(3, 8), NewInterval(1, 8)},
		{"disjoint", NewInterval(1, 3), NewInterval(7, 10), NewInterval(1, 10)},
		{"adjacent", NewInterval(1, 5), NewInterval(6, 10), NewInterval(1, 10)},
		{"identical", NewInterval(3, 7), NewInterval(3, 7), NewInterval(3, 7)},
		{"one contains other", NewInterval(1, 10), NewInterval(3, 7), NewInterval(1, 10)},
		{"negative ranges", NewInterval(-10, -5), NewInterval(-3, -1), NewInterval(-10, -1)},
		{"spans zero", NewInterval(-5, 0), NewInterval(0, 5), NewInterval(-5, 5)},
		{"single points", NewInterval(3, 3), NewInterval(7, 7), NewInterval(3, 7)},

		// Bottom identity
		{"bottom join normal", Bottom(), NewInterval(1, 5), NewInterval(1, 5)},
		{"normal join bottom", NewInterval(1, 5), Bottom(), NewInterval(1, 5)},
		{"bottom join bottom", Bottom(), Bottom(), Bottom()},

		// Top absorbs
		{"top join normal", Top(), NewInterval(1, 5), Top()},
		{"normal join top", NewInterval(1, 5), Top(), Top()},
		{"top join top", Top(), Top(), Top()},
		{"top join bottom", Top(), Bottom(), Top()},
		{"bottom join top", Bottom(), Top(), Top()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.a.Join(tt.b)
			require.True(t, got.Equals(tt.want), "%s: %+v.Join(%+v) = %+v, want %+v", tt.name, tt.a, tt.b, got, tt.want)
		})
	}
}

func TestMeet(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		a    Interval
		b    Interval
		want Interval
	}{
		// Normal cases
		{"overlapping", NewInterval(1, 10), NewInterval(5, 20), NewInterval(5, 10)},
		{"one contains other", NewInterval(1, 10), NewInterval(3, 7), NewInterval(3, 7)},
		{"identical", NewInterval(3, 7), NewInterval(3, 7), NewInterval(3, 7)},
		{"single overlap point", NewInterval(1, 5), NewInterval(5, 10), NewInterval(5, 5)},
		{"negative overlap", NewInterval(-10, -3), NewInterval(-7, -1), NewInterval(-7, -3)},

		// Empty intersection returns bottom
		{"disjoint", NewInterval(1, 5), NewInterval(10, 20), Bottom()},
		{"disjoint adjacent", NewInterval(1, 5), NewInterval(6, 10), Bottom()},
		{"disjoint negative", NewInterval(-10, -5), NewInterval(1, 5), Bottom()},

		// Top identity
		{"top meet normal", Top(), NewInterval(1, 5), NewInterval(1, 5)},
		{"normal meet top", NewInterval(1, 5), Top(), NewInterval(1, 5)},
		{"top meet top", Top(), Top(), Top()},

		// Bottom absorbs
		{"bottom meet normal", Bottom(), NewInterval(1, 5), Bottom()},
		{"normal meet bottom", NewInterval(1, 5), Bottom(), Bottom()},
		{"bottom meet bottom", Bottom(), Bottom(), Bottom()},
		{"top meet bottom", Top(), Bottom(), Bottom()},
		{"bottom meet top", Bottom(), Top(), Bottom()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.a.Meet(tt.b)
			require.True(t, got.Equals(tt.want), "%s: %+v.Meet(%+v) = %+v, want %+v", tt.name, tt.a, tt.b, got, tt.want)
		})
	}
}

func TestEquals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		a    Interval
		b    Interval
		want bool
	}{
		// Equal cases
		{"same range", NewInterval(1, 10), NewInterval(1, 10), true},
		{"same single point", NewInterval(5, 5), NewInterval(5, 5), true},
		{"both bottom", Bottom(), Bottom(), true},
		{"both top", Top(), Top(), true},
		{"both zero", NewInterval(0, 0), NewInterval(0, 0), true},

		// Not equal cases
		{"different lo", NewInterval(1, 10), NewInterval(2, 10), false},
		{"different hi", NewInterval(1, 10), NewInterval(1, 9), false},
		{"different both", NewInterval(1, 5), NewInterval(6, 10), false},
		{"bottom vs normal", Bottom(), NewInterval(1, 5), false},
		{"normal vs bottom", NewInterval(1, 5), Bottom(), false},
		{"top vs normal", Top(), NewInterval(1, 5), false},
		{"normal vs top", NewInterval(1, 5), Top(), false},
		{"top vs bottom", Top(), Bottom(), false},
		{"bottom vs top", Bottom(), Top(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, tt.a.Equals(tt.b))
		})
	}
}

func TestAdd(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		a    Interval
		b    Interval
		want Interval
	}{
		{"positive ranges", NewInterval(1, 5), NewInterval(3, 7), NewInterval(4, 12)},
		{"negative ranges", NewInterval(-5, -1), NewInterval(-3, -2), NewInterval(-8, -3)},
		{"mixed signs", NewInterval(-5, 5), NewInterval(1, 3), NewInterval(-4, 8)},
		{"single points", NewInterval(3, 3), NewInterval(7, 7), NewInterval(10, 10)},
		{"add zero interval", NewInterval(1, 5), NewInterval(0, 0), NewInterval(1, 5)},
		{"spans zero", NewInterval(-10, -1), NewInterval(1, 10), NewInterval(-9, 9)},
		{"bottom propagates", Bottom(), NewInterval(1, 5), Bottom()},
		{"bottom propagates rhs", NewInterval(1, 5), Bottom(), Bottom()},
		{"top propagates", Top(), NewInterval(1, 5), Top()},
		{"top propagates rhs", NewInterval(1, 5), Top(), Top()},
		{"bottom and top", Bottom(), Top(), Bottom()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.a.Add(tt.b)
			require.True(t, got.Equals(tt.want), "%s: %+v.Add(%+v) = %+v, want %+v", tt.name, tt.a, tt.b, got, tt.want)
		})
	}
}

func TestSub(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		a    Interval
		b    Interval
		want Interval
	}{
		{"positive ranges", NewInterval(5, 10), NewInterval(1, 3), NewInterval(2, 9)},
		{"same interval", NewInterval(3, 7), NewInterval(3, 7), NewInterval(-4, 4)},
		{"single points", NewInterval(10, 10), NewInterval(3, 3), NewInterval(7, 7)},
		{"negative ranges", NewInterval(-5, -1), NewInterval(-3, -2), NewInterval(-3, 2)},
		{"mixed signs", NewInterval(-5, 5), NewInterval(1, 3), NewInterval(-8, 4)},
		{"sub zero", NewInterval(1, 5), NewInterval(0, 0), NewInterval(1, 5)},
		{"result spans zero", NewInterval(1, 5), NewInterval(2, 8), NewInterval(-7, 3)},
		{"bottom propagates", Bottom(), NewInterval(1, 5), Bottom()},
		{"bottom propagates rhs", NewInterval(1, 5), Bottom(), Bottom()},
		{"top propagates", Top(), NewInterval(1, 5), Top()},
		{"top propagates rhs", NewInterval(1, 5), Top(), Top()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.a.Sub(tt.b)
			require.True(t, got.Equals(tt.want), "%s: %+v.Sub(%+v) = %+v, want %+v", tt.name, tt.a, tt.b, got, tt.want)
		})
	}
}

func TestMul(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		a    Interval
		b    Interval
		want Interval
	}{
		{"positive ranges", NewInterval(2, 4), NewInterval(3, 5), NewInterval(6, 20)},
		{"negative times positive", NewInterval(-3, -1), NewInterval(2, 4), NewInterval(-12, -2)},
		{"negative times negative", NewInterval(-4, -2), NewInterval(-3, -1), NewInterval(2, 12)},
		{"spans zero lhs", NewInterval(-2, 3), NewInterval(1, 4), NewInterval(-8, 12)},
		{"spans zero both", NewInterval(-2, 3), NewInterval(-1, 4), NewInterval(-8, 12)},
		{"single points", NewInterval(3, 3), NewInterval(7, 7), NewInterval(21, 21)},
		{"mul by zero interval", NewInterval(1, 5), NewInterval(0, 0), NewInterval(0, 0)},
		{"mul by one", NewInterval(3, 7), NewInterval(1, 1), NewInterval(3, 7)},
		{"negative spans zero", NewInterval(-3, 2), NewInterval(-4, 1), NewInterval(-8, 12)},
		{"bottom propagates", Bottom(), NewInterval(1, 5), Bottom()},
		{"top propagates", Top(), NewInterval(1, 5), Top()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.a.Mul(tt.b)
			require.True(t, got.Equals(tt.want), "%s: %+v.Mul(%+v) = %+v, want %+v", tt.name, tt.a, tt.b, got, tt.want)
		})
	}
}

func TestExcludeZero_ContainsZero(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		interval Interval
		want     bool
	}{
		{"top with excludeZero", Top().ExcludeZero(), false},
		{"spans zero with excludeZero", NewInterval(-5, 5).ExcludeZero(), false},
		{"exactly zero with excludeZero", NewInterval(0, 0).ExcludeZero(), false},
		{"lo is zero with excludeZero", NewInterval(0, 10).ExcludeZero(), false},
		{"hi is zero with excludeZero", NewInterval(-10, 0).ExcludeZero(), false},
		{"positive with excludeZero", NewInterval(1, 10).ExcludeZero(), false},
		{"negative with excludeZero", NewInterval(-10, -1).ExcludeZero(), false},
		{"bottom with excludeZero", Bottom().ExcludeZero(), false},
		// Without excludeZero for contrast
		{"top without excludeZero", Top(), true},
		{"spans zero without excludeZero", NewInterval(-5, 5), true},
		{"exactly zero without excludeZero", NewInterval(0, 0), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, tt.interval.ContainsZero())
		})
	}
}

func TestExcludeZero_Join(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		a              Interval
		b              Interval
		wantContains   bool
		wantExcludeSet bool
	}{
		// Both exclude zero → result excludes zero
		{"both exclude", NewInterval(-5, 5).ExcludeZero(), NewInterval(-3, 3).ExcludeZero(), false, true},
		// Only one excludes zero → result does NOT exclude zero
		{"only lhs excludes", NewInterval(-5, 5).ExcludeZero(), NewInterval(-3, 3), true, false},
		{"only rhs excludes", NewInterval(-5, 5), NewInterval(-3, 3).ExcludeZero(), true, false},
		// Neither excludes zero → result does NOT exclude zero
		{"neither excludes", NewInterval(-5, 5), NewInterval(-3, 3), true, false},
		// Bottom identity preserves excludeZero
		{"bottom join excluded", Bottom(), NewInterval(-5, 5).ExcludeZero(), false, true},
		{"excluded join bottom", NewInterval(-5, 5).ExcludeZero(), Bottom(), false, true},
		// Top absorbs (Top.Join returns Top, loses excludeZero)
		{"top join excluded", Top(), NewInterval(-5, 5).ExcludeZero(), true, false},
		{"excluded join top", NewInterval(-5, 5).ExcludeZero(), Top(), true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.a.Join(tt.b)
			require.Equal(t, tt.wantContains, got.ContainsZero(), "ContainsZero mismatch")
			require.Equal(t, tt.wantExcludeSet, got.excludeZero, "excludeZero flag mismatch")
		})
	}
}

func TestExcludeZero_Meet(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		a              Interval
		b              Interval
		wantContains   bool
		wantExcludeSet bool
	}{
		// Either excludes zero → result excludes zero
		{"both exclude", NewInterval(-5, 5).ExcludeZero(), NewInterval(-3, 3).ExcludeZero(), false, true},
		{"only lhs excludes", NewInterval(-5, 5).ExcludeZero(), NewInterval(-3, 3), false, true},
		{"only rhs excludes", NewInterval(-5, 5), NewInterval(-3, 3).ExcludeZero(), false, true},
		// Neither excludes → result does NOT exclude zero
		{"neither excludes", NewInterval(-5, 5), NewInterval(-3, 3), true, false},
		// Top identity preserves excludeZero
		{"top meet excluded", Top(), NewInterval(-5, 5).ExcludeZero(), false, true},
		{"excluded meet top", NewInterval(-5, 5).ExcludeZero(), Top(), false, true},
		// Bottom absorbs
		{"bottom meet excluded", Bottom(), NewInterval(-5, 5).ExcludeZero(), false, false},
		{"excluded meet bottom", NewInterval(-5, 5).ExcludeZero(), Bottom(), false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.a.Meet(tt.b)
			require.Equal(t, tt.wantContains, got.ContainsZero(), "ContainsZero mismatch")
			require.Equal(t, tt.wantExcludeSet, got.excludeZero, "excludeZero flag mismatch")
		})
	}
}

func TestDiv(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		a    Interval
		b    Interval
		want Interval
	}{
		{"positive ranges", NewInterval(10, 20), NewInterval(2, 5), NewInterval(2, 10)},
		{"negative dividend", NewInterval(-20, -10), NewInterval(2, 5), NewInterval(-10, -2)},
		{"negative divisor", NewInterval(10, 20), NewInterval(-5, -2), NewInterval(-10, -2)},
		{"both negative", NewInterval(-20, -10), NewInterval(-5, -2), NewInterval(2, 10)},
		{"single points", NewInterval(12, 12), NewInterval(4, 4), NewInterval(3, 3)},
		{"integer truncation", NewInterval(7, 7), NewInterval(2, 2), NewInterval(3, 3)},
		{"div by one", NewInterval(3, 7), NewInterval(1, 1), NewInterval(3, 7)},

		// Division by zero cases
		{"divisor contains zero", NewInterval(1, 10), NewInterval(-1, 1), Top()},
		{"divisor is zero", NewInterval(1, 10), NewInterval(0, 0), Top()},
		{"divisor spans zero", NewInterval(1, 10), NewInterval(-5, 5), Top()},

		// Special cases
		{"bottom propagates", Bottom(), NewInterval(1, 5), Bottom()},
		{"bottom propagates rhs", NewInterval(1, 5), Bottom(), Bottom()},
		{"top propagates", Top(), NewInterval(1, 5), Top()},
		{"top propagates rhs", NewInterval(1, 5), Top(), Top()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.a.Div(tt.b)
			require.True(t, got.Equals(tt.want), "%s: %+v.Div(%+v) = %+v, want %+v", tt.name, tt.a, tt.b, got, tt.want)
		})
	}
}
