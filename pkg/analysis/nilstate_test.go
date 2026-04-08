package analysis

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNilState_Absorption verifies the absorption law:
// a.Join(a.Meet(b)) == a and a.Meet(a.Join(b)) == a
func TestNilState_Absorption(t *testing.T) {
	t.Parallel()

	states := [4]NilState{NilBottom, DefinitelyNil, DefinitelyNotNil, MaybeNil}
	names := [4]string{"Bottom", "Nil", "NonNil", "MaybeNil"}

	for i, a := range states {
		for j, b := range states {
			t.Run(names[i]+"_"+names[j], func(t *testing.T) {
				t.Parallel()
				// a ∨ (a ∧ b) = a
				require.Equal(t, a, a.Join(a.Meet(b)),
					"absorption law 1 failed for %s, %s", names[i], names[j])
				// a ∧ (a ∨ b) = a
				require.Equal(t, a, a.Meet(a.Join(b)),
					"absorption law 2 failed for %s, %s", names[i], names[j])
			})
		}
	}
}

func TestNilState_Equals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a, b NilState
		want bool
	}{
		{"Bottom == Bottom", NilBottom, NilBottom, true},
		{"Nil == Nil", DefinitelyNil, DefinitelyNil, true},
		{"NonNil == NonNil", DefinitelyNotNil, DefinitelyNotNil, true},
		{"MaybeNil == MaybeNil", MaybeNil, MaybeNil, true},

		{"Bottom != Nil", NilBottom, DefinitelyNil, false},
		{"Bottom != NonNil", NilBottom, DefinitelyNotNil, false},
		{"Bottom != MaybeNil", NilBottom, MaybeNil, false},
		{"Nil != NonNil", DefinitelyNil, DefinitelyNotNil, false},
		{"Nil != MaybeNil", DefinitelyNil, MaybeNil, false},
		{"NonNil != MaybeNil", DefinitelyNotNil, MaybeNil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, tt.a.Equals(tt.b))
		})
	}
}

// TestNilState_Idempotent verifies a.Join(a) == a and a.Meet(a) == a.
func TestNilState_Idempotent(t *testing.T) {
	t.Parallel()

	states := [4]NilState{NilBottom, DefinitelyNil, DefinitelyNotNil, MaybeNil}
	names := [4]string{"Bottom", "Nil", "NonNil", "MaybeNil"}

	for i, s := range states {
		t.Run(names[i], func(t *testing.T) {
			t.Parallel()
			require.Equal(t, s, s.Join(s), "Join not idempotent for %s", names[i])
			require.Equal(t, s, s.Meet(s), "Meet not idempotent for %s", names[i])
		})
	}
}

// TestNilState_Join_BottomIsIdentity verifies Bottom is the identity element for Join.
func TestNilState_Join_BottomIsIdentity(t *testing.T) {
	t.Parallel()

	states := [4]NilState{NilBottom, DefinitelyNil, DefinitelyNotNil, MaybeNil}
	names := [4]string{"Bottom", "Nil", "NonNil", "MaybeNil"}

	for i, s := range states {
		t.Run(names[i], func(t *testing.T) {
			t.Parallel()
			require.Equal(t, s, s.Join(NilBottom))
			require.Equal(t, s, NilBottom.Join(s))
		})
	}
}

// TestNilState_Join_Commutative verifies a.Join(b) == b.Join(a) for all pairs.
func TestNilState_Join_Commutative(t *testing.T) {
	t.Parallel()

	states := [4]NilState{NilBottom, DefinitelyNil, DefinitelyNotNil, MaybeNil}
	names := [4]string{"Bottom", "Nil", "NonNil", "MaybeNil"}

	for i, a := range states {
		for j, b := range states {
			t.Run(names[i]+"_"+names[j], func(t *testing.T) {
				t.Parallel()
				require.Equal(t, a.Join(b), b.Join(a),
					"Join is not commutative for %s and %s", names[i], names[j])
			})
		}
	}
}

// TestNilState_Join_Exhaustive tests all 16 combinations of Join.
func TestNilState_Join_Exhaustive(t *testing.T) {
	t.Parallel()

	// Indexed by [receiver][argument] = expected result.
	// Order: NilBottom(0), DefinitelyNil(1), DefinitelyNotNil(2), MaybeNil(3)
	expected := [4][4]NilState{
		//             Bottom           Nil              NonNil           MaybeNil
		/* Bottom  */ {NilBottom, DefinitelyNil, DefinitelyNotNil, MaybeNil},
		/* Nil     */ {DefinitelyNil, DefinitelyNil, MaybeNil, MaybeNil},
		/* NonNil  */ {DefinitelyNotNil, MaybeNil, DefinitelyNotNil, MaybeNil},
		/* MaybeNil*/ {MaybeNil, MaybeNil, MaybeNil, MaybeNil},
	}

	names := [4]string{"Bottom", "Nil", "NonNil", "MaybeNil"}
	states := [4]NilState{NilBottom, DefinitelyNil, DefinitelyNotNil, MaybeNil}

	for i, s := range states {
		for j, other := range states {
			t.Run(names[i]+"_join_"+names[j], func(t *testing.T) {
				t.Parallel()
				got := s.Join(other)
				require.Equal(t, expected[i][j], got,
					"%s.Join(%s): want %s, got %s",
					names[i], names[j], names[expected[i][j]], names[got])
			})
		}
	}
}

// TestNilState_Join_TopIsAbsorbing verifies MaybeNil is the absorbing element for Join.
func TestNilState_Join_TopIsAbsorbing(t *testing.T) {
	t.Parallel()

	states := [4]NilState{NilBottom, DefinitelyNil, DefinitelyNotNil, MaybeNil}
	names := [4]string{"Bottom", "Nil", "NonNil", "MaybeNil"}

	for i, s := range states {
		t.Run(names[i], func(t *testing.T) {
			t.Parallel()
			require.Equal(t, MaybeNil, s.Join(MaybeNil))
			require.Equal(t, MaybeNil, MaybeNil.Join(s))
		})
	}
}

// TestNilState_Meet_BottomIsAbsorbing verifies Bottom is the absorbing element for Meet.
func TestNilState_Meet_BottomIsAbsorbing(t *testing.T) {
	t.Parallel()

	states := [4]NilState{NilBottom, DefinitelyNil, DefinitelyNotNil, MaybeNil}
	names := [4]string{"Bottom", "Nil", "NonNil", "MaybeNil"}

	for i, s := range states {
		t.Run(names[i], func(t *testing.T) {
			t.Parallel()
			require.Equal(t, NilBottom, s.Meet(NilBottom))
			require.Equal(t, NilBottom, NilBottom.Meet(s))
		})
	}
}

// TestNilState_Meet_Commutative verifies a.Meet(b) == b.Meet(a) for all pairs.
func TestNilState_Meet_Commutative(t *testing.T) {
	t.Parallel()

	states := [4]NilState{NilBottom, DefinitelyNil, DefinitelyNotNil, MaybeNil}
	names := [4]string{"Bottom", "Nil", "NonNil", "MaybeNil"}

	for i, a := range states {
		for j, b := range states {
			t.Run(names[i]+"_"+names[j], func(t *testing.T) {
				t.Parallel()
				require.Equal(t, a.Meet(b), b.Meet(a),
					"Meet is not commutative for %s and %s", names[i], names[j])
			})
		}
	}
}

// TestNilState_Meet_Exhaustive tests all 16 combinations of Meet.
func TestNilState_Meet_Exhaustive(t *testing.T) {
	t.Parallel()

	// Indexed by [receiver][argument] = expected result.
	expected := [4][4]NilState{
		//             Bottom     Nil        NonNil     MaybeNil
		/* Bottom  */ {NilBottom, NilBottom, NilBottom, NilBottom},
		/* Nil     */ {NilBottom, DefinitelyNil, NilBottom, DefinitelyNil},
		/* NonNil  */ {NilBottom, NilBottom, DefinitelyNotNil, DefinitelyNotNil},
		/* MaybeNil*/ {NilBottom, DefinitelyNil, DefinitelyNotNil, MaybeNil},
	}

	names := [4]string{"Bottom", "Nil", "NonNil", "MaybeNil"}
	states := [4]NilState{NilBottom, DefinitelyNil, DefinitelyNotNil, MaybeNil}

	for i, s := range states {
		for j, other := range states {
			t.Run(names[i]+"_meet_"+names[j], func(t *testing.T) {
				t.Parallel()
				got := s.Meet(other)
				require.Equal(t, expected[i][j], got,
					"%s.Meet(%s): want %s, got %s",
					names[i], names[j], names[expected[i][j]], names[got])
			})
		}
	}
}

// TestNilState_Meet_TopIsIdentity verifies MaybeNil is the identity element for Meet.
func TestNilState_Meet_TopIsIdentity(t *testing.T) {
	t.Parallel()

	states := [4]NilState{NilBottom, DefinitelyNil, DefinitelyNotNil, MaybeNil}
	names := [4]string{"Bottom", "Nil", "NonNil", "MaybeNil"}

	for i, s := range states {
		t.Run(names[i], func(t *testing.T) {
			t.Parallel()
			require.Equal(t, s, s.Meet(MaybeNil))
			require.Equal(t, s, MaybeNil.Meet(s))
		})
	}
}
