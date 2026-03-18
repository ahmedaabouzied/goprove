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
	if s == other {
		return s
	}
	if s == NilBottom {
		return other
	}
	if other == NilBottom {
		return s
	}
	return MaybeNil // Nil+NonNil, or either is MaybeNil
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
	// Simplified:
	if s == other {
		return s
	}
	if s == NilBottom || other == NilBottom {
		return NilBottom
	}
	if s == MaybeNil {
		return other
	}
	if other == MaybeNil {
		return s
	}
	return NilBottom
}
