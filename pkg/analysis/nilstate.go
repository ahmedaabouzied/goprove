package analysis

type NilState uint8

const (
	NilBottom        NilState = iota // Unreachable
	DefinitelyNil                    // Proven nil
	DefinitelyNotNil                 // Proven non-nil
	MaybeNil                         // Unknown. Could be either (Top)
)

func (s NilState) Equals(other NilState) bool {
	return s == other
}

func (s NilState) Join(other NilState) NilState {
	/*
			Join lookup table
		  ┌──────────┬──────────┬──────────┬──────────┬──────────┐
		  │   Join   │  Bottom  │   Nil    │  NonNil  │ MaybeNil │
		  ├──────────┼──────────┼──────────┼──────────┼──────────┤
		  │ Bottom   │ Bottom   │ Nil      │ NonNil   │ MaybeNil │
		  ├──────────┼──────────┼──────────┼──────────┼──────────┤
		  │ Nil      │ Nil      │ Nil      │ MaybeNil │ MaybeNil │
		  ├──────────┼──────────┼──────────┼──────────┼──────────┤
		  │ NonNil   │ NonNil   │ MaybeNil │ NonNil   │ MaybeNil │
		  ├──────────┼──────────┼──────────┼──────────┼──────────┤
		  │ MaybeNil │ MaybeNil │ MaybeNil │ MaybeNil │ MaybeNil │
		  └──────────┴──────────┴──────────┴──────────┴──────────┘
	*/
	switch {
	case s == NilBottom:
		return other
	case other == NilBottom:
		return s

	case s == MaybeNil:
		return MaybeNil

	case s == DefinitelyNil:
		switch other {
		// NilBottom has been handled earlier
		case DefinitelyNil:
			return DefinitelyNil
		case DefinitelyNotNil:
			return MaybeNil
		case MaybeNil:
			return MaybeNil
		default:
			return NilBottom
		}
	case s == DefinitelyNotNil:
		switch other {
		case DefinitelyNotNil:
			return DefinitelyNotNil
		case DefinitelyNil:
			return MaybeNil
		case MaybeNil:
			return MaybeNil
		default:
			return NilBottom
		}
	default:
		return NilBottom
	}
}
