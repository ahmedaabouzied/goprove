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
	}

	// Default
	return NilBottom
}

func (s NilState) Meet(other NilState) NilState {
	/*
			Meet lookup table
		  ┌──────────┬──────────┬──────────┬──────────┬──────────┐
		  │   Meet   │  Bottom  │   Nil    │  NonNil  │ MaybeNil │
		  ├──────────┼──────────┼──────────┼──────────┼──────────┤
		  │ Bottom   │ Bottom   │ Bottom   │ Bottom   │ Bottom   │
		  ├──────────┼──────────┼──────────┼──────────┼──────────┤
		  │ Nil      │ Bottom   │ Nil      │ Bottom   │ Nil      │
		  ├──────────┼──────────┼──────────┼──────────┼──────────┤
		  │ NonNil   │ Bottom   │ Bottom   │ NonNil   │ NonNil   │
		  ├──────────┼──────────┼──────────┼──────────┼──────────┤
		  │ MaybeNil │ Bottom   │ Nil      │ NonNil   │ MaybeNil │
		  └──────────┴──────────┴──────────┴──────────┴──────────┘
	*/
	switch {
	case s == NilBottom:
		return NilBottom
	case other == NilBottom:
		return NilBottom

	case s == DefinitelyNil:
		switch other {
		case DefinitelyNil:
			return DefinitelyNil
		case DefinitelyNotNil:
			return NilBottom
		case MaybeNil:
			return DefinitelyNil
		}

	case s == DefinitelyNotNil:
		switch other {
		case DefinitelyNil:
			return NilBottom
		case DefinitelyNotNil:
			return DefinitelyNotNil
		case MaybeNil:
			return DefinitelyNotNil
		}

	case s == MaybeNil:
		switch other {
		case DefinitelyNil:
			return DefinitelyNil
		case DefinitelyNotNil:
			return DefinitelyNotNil
		case MaybeNil:
			return MaybeNil
		}
	}

	return NilBottom
}
