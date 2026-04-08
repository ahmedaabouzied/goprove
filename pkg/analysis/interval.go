package analysis

import "math"

// Bottom returns an impossible interval. It happens when two branches are contradictory.
// For example, after x > 5 and x < 3 on the same path. No x satisfies both.
// Bottom is the identity for "Join()". Joining anything with Bottom gives the other thing.
func Bottom() Interval {
	return Interval{IsBottom: true}
}

// Top returns an interval which we have not learned anything about its bounds yet.
// This is the starting state of the parameters.
// Top is the identity for Meet(). Meeting anything with Top gives the other thing.
func Top() Interval {
	return Interval{IsTop: true}
}

// NewInterval returns the interval with the lo and hi bounds.
func NewInterval(lo, hi int64) Interval {
	if lo > hi {
		return Bottom()
	}
	return Interval{Lo: lo, Hi: hi}
}

type Interval struct {
	Lo          int64
	Hi          int64
	IsBottom    bool // unreachable - no possible value
	IsTop       bool // unknown -- any value possible
	excludeZero bool
}

func (i Interval) Add(other Interval) Interval {
	if res, ok := checkSpecial(i, other); ok {
		return res
	}

	lo := i.Lo + other.Lo
	hi := i.Hi + other.Hi

	if i.Hi > 0 && other.Hi > 0 && i.Hi > math.MaxInt64-other.Hi {
		hi = math.MaxInt64
	}
	if i.Lo < 0 && other.Lo < 0 && i.Lo < math.MinInt64-other.Lo {
		lo = math.MinInt64
	}

	return NewInterval(lo, hi)
}

func (i Interval) Contains(other Interval) bool {
	switch {
	case i.IsTop:
		return true // Top contains everything.
	case other.IsTop:
		return i.IsTop // Nothing contains top unless it's top itself.
	case i.IsBottom:
		return false // Bottom contains nothing.
	case other.IsBottom:
		return true // Everything contains bottom
	default:
		return i.Lo <= other.Lo && i.Hi >= other.Hi
	}
}

// ContainsZero is useful for checking for a division by zero is possible.
func (i Interval) ContainsZero() bool {
	if i.excludeZero || i.IsBottom {
		return false
	}

	if i.IsTop {
		return true
	}

	return i.Lo <= 0 && i.Hi >= 0
}

func (i Interval) Div(other Interval) Interval {
	if res, ok := checkSpecial(i, other); ok {
		return res
	}
	if other.ContainsZero() {
		return Top()
	}
	lo := min(i.Lo/other.Lo, i.Lo/other.Hi, i.Hi/other.Lo, i.Hi/other.Hi)
	hi := max(i.Lo/other.Lo, i.Lo/other.Hi, i.Hi/other.Lo, i.Hi/other.Hi)
	return NewInterval(lo, hi)
}

func (i Interval) Equals(other Interval) bool {
	if (i.IsBottom && other.IsBottom) || (i.IsTop && other.IsTop) {
		return true
	}
	if i.IsBottom || other.IsBottom || i.IsTop || other.IsTop {
		return false
	}

	return i.Lo == other.Lo && i.Hi == other.Hi
}

func (i Interval) ExcludeZero() Interval {
	if i.IsBottom {
		return Bottom()
	}

	if i.IsTop {
		i.excludeZero = true
		return i
	}

	if i.Lo == 0 && i.Hi == 0 { // Excluding 0 out of [0,0]
		return Bottom()
	}
	if i.Lo == 0 {
		return NewInterval(1, i.Hi)
	}

	if i.Hi == 0 {
		return NewInterval(i.Lo, -1)
	}

	i.excludeZero = true
	return i
}

func (i Interval) Join(other Interval) Interval {
	if i.IsTop || other.IsTop {
		return Top()
	}

	if i.IsBottom {
		return other
	}

	if other.IsBottom {
		return i
	}

	lo := least(i.Lo, other.Lo)
	hi := greatest(i.Hi, other.Hi)

	res := NewInterval(lo, hi)
	res.excludeZero = i.excludeZero && other.excludeZero

	return res
}

func (i Interval) Meet(other Interval) Interval {
	if i.IsBottom || other.IsBottom {
		return Bottom()
	}

	if i.IsTop {
		return other
	}

	if other.IsTop {
		return i
	}

	lo := greatest(i.Lo, other.Lo)
	hi := least(i.Hi, other.Hi)

	res := NewInterval(lo, hi)
	res.excludeZero = i.excludeZero || other.excludeZero

	return res
}

func (i Interval) Mul(other Interval) Interval {
	if res, ok := checkSpecial(i, other); ok {
		return res
	}
	lo := min(i.Lo*other.Lo, i.Lo*other.Hi, i.Hi*other.Lo, i.Hi*other.Hi)
	hi := max(i.Lo*other.Lo, i.Lo*other.Hi, i.Hi*other.Lo, i.Hi*other.Hi)

	return NewInterval(lo, hi)
}

func (i Interval) Neg() Interval {
	if i.IsBottom {
		return Bottom()
	}

	if i.IsTop {
		return Top()
	}

	return NewInterval(-i.Hi, -i.Lo)
}

func (i Interval) Rem(other Interval) Interval {
	if i.IsBottom || other.IsBottom {
		return Bottom()
	}

	if other.IsTop {
		return Top()
	}

	// M = max(|y.Lo|, |y.Hi|)
	M := max(abs(other.Lo), abs(other.Hi))
	lo := -(M - 1)
	hi := M - 1
	return NewInterval(lo, hi)
}

func (i Interval) Sub(other Interval) Interval {
	if res, ok := checkSpecial(i, other); ok {
		return res
	}
	lo := i.Lo - other.Hi
	hi := i.Hi - other.Lo

	return NewInterval(lo, hi)
}

func (i Interval) Widen(new Interval) Interval {
	if i.IsBottom {
		return new
	}
	if new.IsBottom {
		return i
	}

	if i.IsTop || new.IsTop {
		return Top()
	}

	lo := i.Lo
	if new.Lo < i.Lo {
		lo = math.MinInt64
	}

	hi := i.Hi
	if new.Hi > i.Hi {
		hi = math.MaxInt64
	}

	return NewInterval(lo, hi)
}

func abs(i int64) int64 {
	if i < 0 {
		return -i
	}
	return i
}

func checkSpecial(a, b Interval) (Interval, bool) {
	if a.IsBottom || b.IsBottom {
		return Bottom(), true
	}

	if a.IsTop || b.IsTop {
		return Top(), true
	}
	return Interval{}, false
}

func greatest(x, y int64) int64 {
	if x < y {
		return y
	}
	return x
}

func least(x, y int64) int64 {
	if x < y {
		return x
	}
	return y
}
