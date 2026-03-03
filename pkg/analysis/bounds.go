package analysis

import (
	"go/types"
	"math"
)

func IntervalForType(kind types.BasicKind) (Interval, bool) {
	switch kind {
	case types.Int8:
		return NewInterval(math.MinInt8, math.MaxInt8), true
	case types.Int16:
		return NewInterval(math.MinInt16, math.MaxInt16), true
	case types.Int32:
		return NewInterval(math.MinInt32, math.MaxInt32), true
	case types.Int64:
		return NewInterval(math.MinInt64, math.MaxInt64), false
	default:
		return Bottom(), false
	}
}
