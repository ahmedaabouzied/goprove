package analysis

import (
	"go/constant"
	"go/types"
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/loader"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
)

// ---------------------------------------------------------------------------
// isNillable tests
// ---------------------------------------------------------------------------

// TestIsNillable_Constants tests isNillable using ssa.NewConst with
// various types. Constants are ssa.Values, so isNillable inspects their type.
func TestIsNillable_Constants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		typ  types.Type
		want bool
	}{
		// --- Non-nillable basic types ---
		{"int", types.Typ[types.Int], false},
		{"int8", types.Typ[types.Int8], false},
		{"int16", types.Typ[types.Int16], false},
		{"int32", types.Typ[types.Int32], false},
		{"int64", types.Typ[types.Int64], false},
		{"uint", types.Typ[types.Uint], false},
		{"uint8", types.Typ[types.Uint8], false},
		{"uint16", types.Typ[types.Uint16], false},
		{"uint32", types.Typ[types.Uint32], false},
		{"uint64", types.Typ[types.Uint64], false},
		{"float32", types.Typ[types.Float32], false},
		{"float64", types.Typ[types.Float64], false},
		{"complex64", types.Typ[types.Complex64], false},
		{"complex128", types.Typ[types.Complex128], false},
		{"bool", types.Typ[types.Bool], false},
		{"string", types.Typ[types.String], false},
		{"byte", types.Typ[types.Byte], false},
		{"rune", types.Typ[types.Rune], false},
		{"uintptr", types.Typ[types.Uintptr], false},

		// unsafe.Pointer is technically nillable in Go, but we deliberately
		// exclude it from nil tracking. NASA P10 Rule 8 restricts unsafe
		// usage, so it should rarely appear. If unsafe.Pointer nil tracking
		// is ever needed, this test documents the exception to update.
		{"unsafe.Pointer", types.Typ[types.UnsafePointer], false},

		// --- Non-nillable composite types ---
		{"struct", types.NewStruct(nil, nil), false},
		{"array", types.NewArray(types.Typ[types.Int], 3), false},

		// --- Nillable types ---
		{"*int", types.NewPointer(types.Typ[types.Int]), true},
		{"*string", types.NewPointer(types.Typ[types.String]), true},
		{"**int", types.NewPointer(types.NewPointer(types.Typ[types.Int])), true},
		{"empty interface", types.NewInterfaceType(nil, nil), true},
		{"[]int", types.NewSlice(types.Typ[types.Int]), true},
		{"[]byte", types.NewSlice(types.Typ[types.Byte]), true},
		{"map[string]int", types.NewMap(types.Typ[types.String], types.Typ[types.Int]), true},
		{"chan int", types.NewChan(types.SendRecv, types.Typ[types.Int]), true},
		{"chan<- int", types.NewChan(types.SendOnly, types.Typ[types.Int]), true},
		{"<-chan int", types.NewChan(types.RecvOnly, types.Typ[types.Int]), true},
		{"func()", types.NewSignatureType(nil, nil, nil, nil, nil, false), true},
		{"func(int) string", types.NewSignatureType(
			nil, nil, nil,
			types.NewTuple(types.NewVar(0, nil, "x", types.Typ[types.Int])),
			types.NewTuple(types.NewVar(0, nil, "", types.Typ[types.String])),
			false,
		), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ssa.NewConst creates a valid ssa.Value with the given type.
			// For nillable types, use nil Value; for non-nillable, use a zero constant.
			var v ssa.Value
			if tt.want {
				v = ssa.NewConst(nil, tt.typ) // nil constant of this type
			} else {
				// Create a zero-value constant for non-nillable types.
				switch tt.typ.Underlying().(type) {
				case *types.Basic:
					info := tt.typ.Underlying().(*types.Basic).Info()
					switch {
					case info&types.IsBoolean != 0:
						v = ssa.NewConst(constant.MakeBool(false), tt.typ)
					case info&types.IsString != 0:
						v = ssa.NewConst(constant.MakeString(""), tt.typ)
					case info&types.IsInteger != 0:
						v = ssa.NewConst(constant.MakeInt64(0), tt.typ)
					case info&types.IsFloat != 0:
						v = ssa.NewConst(constant.MakeFloat64(0), tt.typ)
					case info&types.IsComplex != 0:
						v = ssa.NewConst(constant.MakeImag(constant.MakeFloat64(0)), tt.typ)
					default:
						v = ssa.NewConst(nil, tt.typ)
					}
				default:
					// struct, array — nil Value represents zero value
					v = ssa.NewConst(nil, tt.typ)
				}
			}
			require.Equal(t, tt.want, isNillable(v))
		})
	}
}

// TestIsNillable_NamedTypes verifies that named types wrapping nillable/non-nillable
// types are handled correctly. isNillable uses Underlying(), so a named type
// like `type MyPtr *int` should still be nillable.
func TestIsNillable_NamedTypes(t *testing.T) {
	t.Parallel()

	pkg := types.NewPackage("test/pkg", "pkg")

	tests := []struct {
		name       string
		underlying types.Type
		want       bool
	}{
		{"named *int", types.NewPointer(types.Typ[types.Int]), true},
		{"named interface{}", types.NewInterfaceType(nil, nil), true},
		{"named []byte", types.NewSlice(types.Typ[types.Byte]), true},
		{"named map[string]int", types.NewMap(types.Typ[types.String], types.Typ[types.Int]), true},
		{"named chan int", types.NewChan(types.SendRecv, types.Typ[types.Int]), true},
		{"named func()", types.NewSignatureType(nil, nil, nil, nil, nil, false), true},
		{"named int", types.Typ[types.Int], false},
		{"named bool", types.Typ[types.Bool], false},
		{"named string", types.Typ[types.String], false},
		{"named struct{}", types.NewStruct(nil, nil), false},
		{"named [3]int", types.NewArray(types.Typ[types.Int], 3), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			named := types.NewNamed(
				types.NewTypeName(0, pkg, "T_"+tt.name, nil),
				tt.underlying,
				nil,
			)
			v := ssa.NewConst(nil, named)
			require.Equal(t, tt.want, isNillable(v))
		})
	}
}

// TestIsNillable_FromSSA loads real SSA code and verifies isNillable on
// actual function parameters of various types.
func TestIsNillable_FromSSA(t *testing.T) {
	t.Parallel()

	prog, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)
	_ = prog

	tests := []struct {
		fnName    string
		paramIdx  int
		wantNil   bool
		paramDesc string
	}{
		// DerefParam(p *int) — pointer param, nillable
		{"DerefParam", 0, true, "*int"},
		// DivByDecrement() — no params, skip
		// Double(x int) — int param, not nillable
		{"Double", 0, false, "int"},
		// MethodCallOnParam(s fmt_Stringer) — interface param, nillable
		{"MethodCallOnParam", 0, true, "interface"},
		// MapLookupOk(m map[string]*int, key string) — map param, nillable; string param, not nillable
		{"MapLookupOk", 0, true, "map"},
		{"MapLookupOk", 1, false, "string"},
	}

	for _, tt := range tests {
		t.Run(tt.fnName+"_param"+tt.paramDesc, func(t *testing.T) {
			t.Parallel()
			var fn *ssa.Function
			for _, member := range pkgs[0].Members {
				f, ok := member.(*ssa.Function)
				if ok && f.Name() == tt.fnName {
					fn = f
					break
				}
			}
			require.NotNil(t, fn, "function %s not found", tt.fnName)
			require.True(t, tt.paramIdx < len(fn.Params),
				"param index %d out of range for %s (has %d params)",
				tt.paramIdx, tt.fnName, len(fn.Params))

			param := fn.Params[tt.paramIdx]
			require.Equal(t, tt.wantNil, isNillable(param),
				"isNillable(%s param %d [%s]): want %v",
				tt.fnName, tt.paramIdx, param.Type(), tt.wantNil)
		})
	}
}

// ---------------------------------------------------------------------------
// lookupNilState tests
// ---------------------------------------------------------------------------

// TestLookupNilState_NilConst tests that a nil-valued *ssa.Const returns DefinitelyNil.
func TestLookupNilState_NilConst(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}
	nilConst := ssa.NewConst(nil, types.NewPointer(types.Typ[types.Int])) // nil:*int

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {},
		},
	}

	got := a.lookupNilState(block, nilConst)
	require.Equal(t, DefinitelyNil, got)
}

// TestLookupNilState_NonNilConst tests that a non-nil *ssa.Const returns DefinitelyNotNil.
func TestLookupNilState_NonNilConst(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}

	tests := []struct {
		name string
		val  *ssa.Const
	}{
		{"int constant", ssa.NewConst(constant.MakeInt64(42), types.Typ[types.Int])},
		{"string constant", ssa.NewConst(constant.MakeString("hello"), types.Typ[types.String])},
		{"bool constant", ssa.NewConst(constant.MakeBool(true), types.Typ[types.Bool])},
		{"float constant", ssa.NewConst(constant.MakeFloat64(3.14), types.Typ[types.Float64])},
		{"zero int", ssa.NewConst(constant.MakeInt64(0), types.Typ[types.Int])},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := &NilAnalyzer{
				state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
					block: {},
				},
			}
			got := a.lookupNilState(block, tt.val)
			require.Equal(t, DefinitelyNotNil, got)
		})
	}
}

// TestLookupNilState_NilConstVariousTypes tests nil constants of every nillable type.
func TestLookupNilState_NilConstVariousTypes(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}

	nillableTypes := []struct {
		name string
		typ  types.Type
	}{
		{"nil *int", types.NewPointer(types.Typ[types.Int])},
		{"nil interface{}", types.NewInterfaceType(nil, nil)},
		{"nil []int", types.NewSlice(types.Typ[types.Int])},
		{"nil map[string]int", types.NewMap(types.Typ[types.String], types.Typ[types.Int])},
		{"nil chan int", types.NewChan(types.SendRecv, types.Typ[types.Int])},
		{"nil func()", types.NewSignatureType(nil, nil, nil, nil, nil, false)},
	}

	for _, tt := range nillableTypes {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			nilConst := ssa.NewConst(nil, tt.typ)
			a := &NilAnalyzer{
				state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
					block: {},
				},
			}
			got := a.lookupNilState(block, nilConst)
			require.Equal(t, DefinitelyNil, got, "nil const of type %s", tt.name)
		})
	}
}

// TestLookupNilState_ZeroValueConst_NonNillable tests that zero-value constants
// of non-nillable types (e.g., 0:int, "":string) return DefinitelyNotNil.
// These have c.Value == nil in SSA but c.IsNil() returns false for non-nillable types.
func TestLookupNilState_ZeroValueConst_NonNillable(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}

	// Zero-value constants: SSA represents struct{}{} and [3]int{} as
	// Const with Value == nil but IsNil() == false (they're not pointer-like).
	tests := []struct {
		name string
		c    *ssa.Const
	}{
		{"zero struct", ssa.NewConst(nil, types.NewStruct(nil, nil))},
		{"zero array", ssa.NewConst(nil, types.NewArray(types.Typ[types.Int], 3))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := &NilAnalyzer{
				state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
					block: {},
				},
			}
			got := a.lookupNilState(block, tt.c)
			require.Equal(t, DefinitelyNotNil, got,
				"zero-value const of non-nillable type should be DefinitelyNotNil")
		})
	}
}

// TestLookupNilState_NonNillableValue tests that a non-nillable ssa.Value
// (not a Const) returns DefinitelyNotNil regardless of block state.
func TestLookupNilState_NonNillableValue(t *testing.T) {
	t.Parallel()

	prog, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)
	_ = prog

	// Double(x int) — x is an int parameter (non-nillable)
	var fn *ssa.Function
	for _, member := range pkgs[0].Members {
		f, ok := member.(*ssa.Function)
		if ok && f.Name() == "Double" {
			fn = f
			break
		}
	}
	require.NotNil(t, fn)
	require.NotEmpty(t, fn.Params)

	intParam := fn.Params[0] // x int
	block := fn.Blocks[0]

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {}, // block exists but has no state for this param
		},
	}

	got := a.lookupNilState(block, intParam)
	require.Equal(t, DefinitelyNotNil, got,
		"non-nillable param should always be DefinitelyNotNil")
}

// TestLookupNilState_NillableValue_InState tests that a nillable value
// with existing state in the block returns that state.
func TestLookupNilState_NillableValue_InState(t *testing.T) {
	t.Parallel()

	prog, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)
	_ = prog

	// DerefParam(p *int) — p is a pointer parameter (nillable)
	var fn *ssa.Function
	for _, member := range pkgs[0].Members {
		f, ok := member.(*ssa.Function)
		if ok && f.Name() == "DerefParam" {
			fn = f
			break
		}
	}
	require.NotNil(t, fn)
	require.NotEmpty(t, fn.Params)

	ptrParam := fn.Params[0] // p *int
	block := fn.Blocks[0]

	// Test each possible stored state.
	allStates := []NilState{NilBottom, DefinitelyNil, DefinitelyNotNil, MaybeNil}
	stateNames := []string{"NilBottom", "DefinitelyNil", "DefinitelyNotNil", "MaybeNil"}

	for i, state := range allStates {
		t.Run("state_"+stateNames[i], func(t *testing.T) {
			t.Parallel()
			a := &NilAnalyzer{
				state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
					block: {ptrParam: state},
				},
			}
			got := a.lookupNilState(block, ptrParam)
			require.Equal(t, state, got,
				"should return stored state %s", stateNames[i])
		})
	}
}

// TestLookupNilState_NillableValue_NotInState tests that a nillable value
// with no state in the block returns MaybeNil (Top).
func TestLookupNilState_NillableValue_NotInState(t *testing.T) {
	t.Parallel()

	prog, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)
	_ = prog

	var fn *ssa.Function
	for _, member := range pkgs[0].Members {
		f, ok := member.(*ssa.Function)
		if ok && f.Name() == "DerefParam" {
			fn = f
			break
		}
	}
	require.NotNil(t, fn)

	ptrParam := fn.Params[0]
	block := fn.Blocks[0]

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {}, // block visited, but no state for ptrParam
		},
	}

	got := a.lookupNilState(block, ptrParam)
	require.Equal(t, MaybeNil, got,
		"nillable value with no state should default to MaybeNil (Top)")
}

// TestLookupNilState_NillableValue_UnvisitedBlock tests that a nillable value
// in an unvisited block (no block entry in state map) returns MaybeNil.
func TestLookupNilState_NillableValue_UnvisitedBlock(t *testing.T) {
	t.Parallel()

	prog, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)
	_ = prog

	var fn *ssa.Function
	for _, member := range pkgs[0].Members {
		f, ok := member.(*ssa.Function)
		if ok && f.Name() == "DerefParam" {
			fn = f
			break
		}
	}
	require.NotNil(t, fn)

	ptrParam := fn.Params[0]
	unvisitedBlock := &ssa.BasicBlock{}

	a := &NilAnalyzer{
		state: make(map[*ssa.BasicBlock]map[ssa.Value]NilState),
		// unvisitedBlock is NOT in state — simulates an unvisited block
	}

	got := a.lookupNilState(unvisitedBlock, ptrParam)
	require.Equal(t, MaybeNil, got,
		"nillable value in unvisited block should default to MaybeNil")
}

// TestLookupNilState_ConstTakesPrecedence verifies that Const handling
// takes precedence over block state. Even if the block has state for
// a value, if the value is a *ssa.Const, the const path runs first.
func TestLookupNilState_ConstTakesPrecedence(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}
	nilConst := ssa.NewConst(nil, types.NewPointer(types.Typ[types.Int]))

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {
				// Even though state says DefinitelyNotNil,
				// the Const path should override.
				nilConst: DefinitelyNotNil,
			},
		},
	}

	got := a.lookupNilState(block, nilConst)
	require.Equal(t, DefinitelyNil, got,
		"nil Const should return DefinitelyNil regardless of block state")
}

// TestLookupNilState_NonNillableTakesPrecedence verifies that the
// non-nillable check takes precedence over block state.
func TestLookupNilState_NonNillableTakesPrecedence(t *testing.T) {
	t.Parallel()

	prog, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)
	_ = prog

	var fn *ssa.Function
	for _, member := range pkgs[0].Members {
		f, ok := member.(*ssa.Function)
		if ok && f.Name() == "Double" {
			fn = f
			break
		}
	}
	require.NotNil(t, fn)

	intParam := fn.Params[0] // x int — non-nillable
	block := fn.Blocks[0]

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {
				// Even though state says MaybeNil, the non-nillable
				// check should override.
				intParam: MaybeNil,
			},
		},
	}

	got := a.lookupNilState(block, intParam)
	require.Equal(t, DefinitelyNotNil, got,
		"non-nillable value should return DefinitelyNotNil regardless of block state")
}
