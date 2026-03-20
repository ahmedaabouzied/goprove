package analysis

import (
	"go/constant"
	"go/token"
	"go/types"
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/loader"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
)

// isNillable tests

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

// lookupNilState tests

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

// transferInstruction tests

// "Always non-nil" producers: Alloc, MakeSlice, MakeMap, MakeChan, MakeInterface

// findFunc looks up a named function in an SSA package.
func findFunc(t *testing.T, pkg *ssa.Package, name string) *ssa.Function {
	t.Helper()
	for _, member := range pkg.Members {
		fn, ok := member.(*ssa.Function)
		if ok && fn.Name() == name {
			return fn
		}
	}
	t.Fatalf("function %s not found in package", name)
	return nil
}

// findInstr searches a function's blocks for the first instruction matching
// the given type. Returns the instruction and its containing block.
func findInstr[T ssa.Instruction](t *testing.T, fn *ssa.Function) (T, *ssa.BasicBlock) {
	t.Helper()
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			if v, ok := instr.(T); ok {
				return v, block
			}
		}
	}
	var zero T
	t.Fatalf("instruction of type %T not found in %s", zero, fn.Name())
	return zero, nil
}

// TestTransferInstruction_AlwaysNonNil is a table-driven test that verifies
// all SSA instructions that always produce non-nil values.
func TestTransferInstruction_AlwaysNonNil(t *testing.T) {
	t.Parallel()

	_, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)
	pkg := pkgs[0]

	tests := []struct {
		name   string
		fnName string
		// findInstr is a function that locates the target instruction.
		find func(t *testing.T, fn *ssa.Function) (ssa.Instruction, *ssa.BasicBlock)
	}{
		{
			name:   "new(T) produces non-nil",
			fnName: "AllocNew",
			find: func(t *testing.T, fn *ssa.Function) (ssa.Instruction, *ssa.BasicBlock) {
				v, b := findInstr[*ssa.Alloc](t, fn)
				return v, b
			},
		},
		{
			name:   "&x produces non-nil",
			fnName: "AllocAddr",
			find: func(t *testing.T, fn *ssa.Function) (ssa.Instruction, *ssa.BasicBlock) {
				v, b := findInstr[*ssa.Alloc](t, fn)
				return v, b
			},
		},
		{
			name:   "make([]T, n) produces non-nil",
			fnName: "MakeSliceFixture",
			find: func(t *testing.T, fn *ssa.Function) (ssa.Instruction, *ssa.BasicBlock) {
				v, b := findInstr[*ssa.MakeSlice](t, fn)
				return v, b
			},
		},
		{
			name:   "make([]T, n, cap) produces non-nil",
			fnName: "MakeSliceCapFixture",
			find: func(t *testing.T, fn *ssa.Function) (ssa.Instruction, *ssa.BasicBlock) {
				v, b := findInstr[*ssa.MakeSlice](t, fn)
				return v, b
			},
		},
		{
			name:   "make(map[K]V) produces non-nil",
			fnName: "MakeMapFixture",
			find: func(t *testing.T, fn *ssa.Function) (ssa.Instruction, *ssa.BasicBlock) {
				v, b := findInstr[*ssa.MakeMap](t, fn)
				return v, b
			},
		},
		{
			name:   "make(map[K]V, hint) produces non-nil",
			fnName: "MakeMapHintFixture",
			find: func(t *testing.T, fn *ssa.Function) (ssa.Instruction, *ssa.BasicBlock) {
				v, b := findInstr[*ssa.MakeMap](t, fn)
				return v, b
			},
		},
		{
			name:   "make(chan T) produces non-nil",
			fnName: "MakeChanFixture",
			find: func(t *testing.T, fn *ssa.Function) (ssa.Instruction, *ssa.BasicBlock) {
				v, b := findInstr[*ssa.MakeChan](t, fn)
				return v, b
			},
		},
		{
			name:   "make(chan T, size) produces non-nil",
			fnName: "MakeChanBufFixture",
			find: func(t *testing.T, fn *ssa.Function) (ssa.Instruction, *ssa.BasicBlock) {
				v, b := findInstr[*ssa.MakeChan](t, fn)
				return v, b
			},
		},
		{
			name:   "interface wrap produces non-nil",
			fnName: "MakeInterfaceFixture",
			find: func(t *testing.T, fn *ssa.Function) (ssa.Instruction, *ssa.BasicBlock) {
				v, b := findInstr[*ssa.MakeInterface](t, fn)
				return v, b
			},
		},
		{
			name:   "interface wrap of nil ptr produces non-nil interface",
			fnName: "MakeInterfaceNilPtrFixture",
			find: func(t *testing.T, fn *ssa.Function) (ssa.Instruction, *ssa.BasicBlock) {
				v, b := findInstr[*ssa.MakeInterface](t, fn)
				return v, b
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fn := findFunc(t, pkg, tt.fnName)
			instr, block := tt.find(t, fn)

			a := &NilAnalyzer{
				state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
					block: {},
				},
			}

			a.transferInstruction(block, instr)

			// The instruction must also be an ssa.Value to appear in state.
			v, ok := instr.(ssa.Value)
			require.True(t, ok, "instruction should implement ssa.Value")
			require.Equal(t, DefinitelyNotNil, a.state[block][v])
		})
	}
}

// TestTransferInstruction_Alloc_HeapEscaped tests that a heap-escaping alloc
// (e.g. returned pointer) is still DefinitelyNotNil.
func TestTransferInstruction_Alloc_HeapEscaped(t *testing.T) {
	t.Parallel()

	_, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)

	fn := findFunc(t, pkgs[0], "AllocAddr")

	// Find the heap alloc specifically (Alloc.Heap == true).
	var heapAlloc *ssa.Alloc
	var allocBlock *ssa.BasicBlock
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			if a, ok := instr.(*ssa.Alloc); ok && a.Heap {
				heapAlloc = a
				allocBlock = block
			}
		}
	}
	// If there's no heap alloc (optimizer kept it on stack), skip.
	if heapAlloc == nil {
		t.Skip("no heap alloc found — optimizer kept it on stack")
	}

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			allocBlock: {},
		},
	}

	a.transferInstruction(allocBlock, heapAlloc)
	require.Equal(t, DefinitelyNotNil, a.state[allocBlock][heapAlloc])
}

// TestTransferInstruction_Phi_ViaSwitch verifies that Phi nodes are correctly
// dispatched through transferInstruction's switch statement.
func TestTransferInstruction_Phi_ViaSwitch(t *testing.T) {
	t.Parallel()

	pred1 := &ssa.BasicBlock{}
	pred2 := &ssa.BasicBlock{}
	block := &ssa.BasicBlock{}
	block.Preds = []*ssa.BasicBlock{pred1, pred2}

	phi := &ssa.Phi{
		Edges: []ssa.Value{newNonNilConst(), newNilConst()},
	}

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			pred1: {},
			pred2: {},
			block: {},
		},
	}

	a.transferInstruction(block, phi)
	require.Equal(t, MaybeNil, a.state[block][phi],
		"Phi through transferInstruction: NotNil+Nil should be MaybeNil")
}

// transferInstruction: unhandled instructions

// TestTransferInstruction_UnhandledInstr verifies that an instruction not
// covered by the switch (e.g. *ssa.Store) does not write to state.
func TestTransferInstruction_UnhandledInstr(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}
	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {},
		},
	}

	// ssa.Store is an instruction that doesn't produce a Value.
	// transferInstruction should not panic and should leave state untouched.
	store := &ssa.Store{}
	a.transferInstruction(block, store)
	require.Empty(t, a.state[block], "unhandled instruction should not modify state")
}

// TestTransferInstruction_BinOp_NoStateChange verifies that a *ssa.BinOp
// (not handled yet in nil analysis) does not modify state.
func TestTransferInstruction_BinOp_NoStateChange(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}
	param := &ssa.Parameter{}

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {param: MaybeNil},
		},
	}

	binOp := &ssa.BinOp{Op: token.ADD}
	a.transferInstruction(block, binOp)

	require.Equal(t, MaybeNil, a.state[block][param],
		"BinOp should not affect existing nil state")
	_, hasBinOp := a.state[block][binOp]
	require.False(t, hasBinOp, "BinOp should not be added to nil state")
}

// transferPhi tests

// findNillablePhi searches a function for a Phi instruction with a nillable type.
func findNillablePhi(t *testing.T, fn *ssa.Function) (*ssa.Phi, *ssa.BasicBlock) {
	t.Helper()
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			if p, ok := instr.(*ssa.Phi); ok {
				if isNillable(p) {
					return p, block
				}
			}
		}
	}
	return nil, nil
}

// TestTransferPhi_BothNotNil tests a Phi where both edges are DefinitelyNotNil.
// Result should be DefinitelyNotNil.
func TestTransferPhi_BothNotNil(t *testing.T) {
	t.Parallel()

	_, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)

	fn := findFunc(t, pkgs[0], "PhiBothNotNil")
	phi, phiBlock := findNillablePhi(t, fn)
	if phi == nil {
		t.Skip("no nillable Phi found — optimizer may have simplified")
	}

	// Set up state: both predecessors have DefinitelyNotNil for their allocs.
	a := &NilAnalyzer{
		state: make(map[*ssa.BasicBlock]map[ssa.Value]NilState),
	}

	for i, edge := range phi.Edges {
		pred := phiBlock.Preds[i]
		if a.state[pred] == nil {
			a.state[pred] = make(map[ssa.Value]NilState)
		}
		a.state[pred][edge] = DefinitelyNotNil
	}

	a.state[phiBlock] = make(map[ssa.Value]NilState)
	a.transferPhi(phiBlock, phi)

	require.Equal(t, DefinitelyNotNil, a.state[phiBlock][phi],
		"Phi with both edges DefinitelyNotNil should be DefinitelyNotNil")
}

// TestTransferPhi_OneNilOneNotNil tests a Phi where one edge is nil and
// the other is non-nil. Result should be MaybeNil.
func TestTransferPhi_OneNilOneNotNil(t *testing.T) {
	t.Parallel()

	_, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)

	fn := findFunc(t, pkgs[0], "PhiOneBranchNil")
	phi, phiBlock := findNillablePhi(t, fn)
	if phi == nil {
		t.Skip("no nillable Phi found — optimizer may have simplified")
	}

	a := &NilAnalyzer{
		state: make(map[*ssa.BasicBlock]map[ssa.Value]NilState),
	}

	for i, edge := range phi.Edges {
		pred := phiBlock.Preds[i]
		if a.state[pred] == nil {
			a.state[pred] = make(map[ssa.Value]NilState)
		}
		if _, isConst := edge.(*ssa.Const); !isConst {
			a.state[pred][edge] = DefinitelyNotNil
		}
	}

	a.state[phiBlock] = make(map[ssa.Value]NilState)
	a.transferPhi(phiBlock, phi)

	require.Equal(t, MaybeNil, a.state[phiBlock][phi],
		"Phi with one nil and one non-nil edge should be MaybeNil")
}

// TestTransferPhi_AllNil tests a Phi where all edges are DefinitelyNil.
// Result should be DefinitelyNil.
func TestTransferPhi_AllNil(t *testing.T) {
	t.Parallel()

	_, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)

	fn := findFunc(t, pkgs[0], "PhiAllNil")
	phi, phiBlock := findNillablePhi(t, fn)
	if phi == nil {
		t.Skip("no nillable Phi found — optimizer may have simplified")
	}

	a := &NilAnalyzer{
		state: make(map[*ssa.BasicBlock]map[ssa.Value]NilState),
	}

	for i := range phi.Edges {
		pred := phiBlock.Preds[i]
		if a.state[pred] == nil {
			a.state[pred] = make(map[ssa.Value]NilState)
		}
	}

	a.state[phiBlock] = make(map[ssa.Value]NilState)
	a.transferPhi(phiBlock, phi)

	require.Equal(t, DefinitelyNil, a.state[phiBlock][phi],
		"Phi with all nil edges should be DefinitelyNil")
}

// Synthetic Phi tests (no SSA build, direct struct construction)

// ptrType is a reusable *int type for synthetic tests.
var ptrType = types.NewPointer(types.Typ[types.Int])

// newNilConst creates a nil *ssa.Const of pointer type.
func newNilConst() *ssa.Const { return ssa.NewConst(nil, ptrType) }

// newNonNilConst creates a non-nil *ssa.Const (integer constant).
func newNonNilConst() *ssa.Const {
	return ssa.NewConst(constant.MakeInt64(42), types.Typ[types.Int])
}

// TestTransferPhi_Synthetic_SingleEdge tests a Phi with a single predecessor.
// Uses a nil const as the edge value, with state set on the predecessor.
func TestTransferPhi_Synthetic_SingleEdge(t *testing.T) {
	t.Parallel()

	pred := &ssa.BasicBlock{}
	block := &ssa.BasicBlock{}
	block.Preds = []*ssa.BasicBlock{pred}

	// A non-nil const — lookupNilState returns DefinitelyNotNil for non-nil consts.
	edge := newNonNilConst()
	phi := &ssa.Phi{
		Edges: []ssa.Value{edge},
	}

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			pred:  {},
			block: {},
		},
	}

	a.transferPhi(block, phi)
	require.Equal(t, DefinitelyNotNil, a.state[block][phi])
}

// TestTransferPhi_Synthetic_JoinTable exercises all NilState combinations
// through Phi with two edges. Uses nil/non-nil consts which lookupNilState
// can resolve without needing typed Parameters.
func TestTransferPhi_Synthetic_JoinTable(t *testing.T) {
	t.Parallel()

	// We test the join through transferPhi by using consts that lookupNilState
	// resolves to known states, combined with state map entries.
	// nil const → DefinitelyNil, non-nil const → DefinitelyNotNil.
	// For MaybeNil/NilBottom we use a nillable const and control state map.
	tests := []struct {
		name  string
		left  NilState
		right NilState
		want  NilState
	}{
		{"Nil+Nil", DefinitelyNil, DefinitelyNil, DefinitelyNil},
		{"Nil+NotNil", DefinitelyNil, DefinitelyNotNil, MaybeNil},
		{"NotNil+NotNil", DefinitelyNotNil, DefinitelyNotNil, DefinitelyNotNil},
		{"NotNil+Nil", DefinitelyNotNil, DefinitelyNil, MaybeNil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pred1 := &ssa.BasicBlock{}
			pred2 := &ssa.BasicBlock{}
			block := &ssa.BasicBlock{}
			block.Preds = []*ssa.BasicBlock{pred1, pred2}

			// Choose consts based on desired nil state.
			var edge1, edge2 ssa.Value
			if tt.left == DefinitelyNil {
				edge1 = newNilConst()
			} else {
				edge1 = newNonNilConst()
			}
			if tt.right == DefinitelyNil {
				edge2 = newNilConst()
			} else {
				edge2 = newNonNilConst()
			}

			phi := &ssa.Phi{
				Edges: []ssa.Value{edge1, edge2},
			}

			a := &NilAnalyzer{
				state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
					pred1: {},
					pred2: {},
					block: {},
				},
			}

			a.transferPhi(block, phi)
			require.Equal(t, tt.want, a.state[block][phi],
				"Join(%v, %v) should be %v", tt.left, tt.right, tt.want)
		})
	}
}

// TestTransferPhi_Synthetic_ThreeEdges tests a Phi with three predecessors.
func TestTransferPhi_Synthetic_ThreeEdges(t *testing.T) {
	t.Parallel()

	pred1 := &ssa.BasicBlock{}
	pred2 := &ssa.BasicBlock{}
	pred3 := &ssa.BasicBlock{}
	block := &ssa.BasicBlock{}
	block.Preds = []*ssa.BasicBlock{pred1, pred2, pred3}

	phi := &ssa.Phi{
		Edges: []ssa.Value{newNonNilConst(), newNonNilConst(), newNilConst()},
	}

	// NotNil + NotNil + Nil = MaybeNil
	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			pred1: {},
			pred2: {},
			pred3: {},
			block: {},
		},
	}

	a.transferPhi(block, phi)
	require.Equal(t, MaybeNil, a.state[block][phi],
		"NotNil+NotNil+Nil should be MaybeNil")
}

// TestTransferPhi_Synthetic_NilConst tests that Phi correctly handles
// a nil *ssa.Const edge via lookupNilState (no state map entry needed).
func TestTransferPhi_Synthetic_NilConst(t *testing.T) {
	t.Parallel()

	pred1 := &ssa.BasicBlock{}
	pred2 := &ssa.BasicBlock{}
	block := &ssa.BasicBlock{}
	block.Preds = []*ssa.BasicBlock{pred1, pred2}

	phi := &ssa.Phi{
		Edges: []ssa.Value{newNonNilConst(), newNilConst()},
	}

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			pred1: {},
			pred2: {},
			block: {},
		},
	}

	a.transferPhi(block, phi)
	require.Equal(t, MaybeNil, a.state[block][phi],
		"NotNil + nil const should be MaybeNil")
}

// TestTransferPhi_Synthetic_NonNilConst tests that Phi handles a non-nil
// *ssa.Const (e.g. integer constant) — lookupNilState returns DefinitelyNotNil.
func TestTransferPhi_Synthetic_NonNilConst(t *testing.T) {
	t.Parallel()

	pred1 := &ssa.BasicBlock{}
	pred2 := &ssa.BasicBlock{}
	block := &ssa.BasicBlock{}
	block.Preds = []*ssa.BasicBlock{pred1, pred2}

	phi := &ssa.Phi{
		Edges: []ssa.Value{newNonNilConst(), newNonNilConst()},
	}

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			pred1: {},
			pred2: {},
			block: {},
		},
	}

	a.transferPhi(block, phi)
	require.Equal(t, DefinitelyNotNil, a.state[block][phi],
		"NotNil + non-nil const should be DefinitelyNotNil")
}

// ===========================================================================
// refineFromCondition tests
// ===========================================================================

// ---------------------------------------------------------------------------
// Synthetic refineFromCondition tests — direct BinOp construction
// ---------------------------------------------------------------------------

// newNonNilPtrConst creates a non-nil *ssa.Const with a pointer type.
// This represents a variable in synthetic tests — it won't match IsNil(),
// so refineFromCondition will correctly identify it as the variable side.
func newNonNilPtrConst() *ssa.Const {
	return ssa.NewConst(constant.MakeInt64(0xDEAD), ptrType)
}

// TestRefineFromCondition_NEQ_TrueBranch tests p != nil on the true branch.
// Variable should become DefinitelyNotNil.
func TestRefineFromCondition_NEQ_TrueBranch(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}
	variable := newNonNilPtrConst() // stand-in for a nillable variable

	cond := &ssa.BinOp{Op: token.NEQ}
	cond.X = variable
	cond.Y = newNilConst() // nil constant

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {},
		},
	}

	a.refineFromCondition(block, cond, true)
	require.Equal(t, DefinitelyNotNil, a.state[block][variable],
		"p != nil, true branch should be DefinitelyNotNil")
}

// TestRefineFromCondition_NEQ_FalseBranch tests p != nil on the false branch.
// Variable should become DefinitelyNil.
func TestRefineFromCondition_NEQ_FalseBranch(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}
	variable := newNonNilPtrConst()

	cond := &ssa.BinOp{Op: token.NEQ}
	cond.X = variable
	cond.Y = newNilConst()

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {},
		},
	}

	a.refineFromCondition(block, cond, false)
	require.Equal(t, DefinitelyNil, a.state[block][variable],
		"p != nil, false branch should be DefinitelyNil")
}

// TestRefineFromCondition_EQL_TrueBranch tests p == nil on the true branch.
// Variable should become DefinitelyNil.
func TestRefineFromCondition_EQL_TrueBranch(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}
	variable := newNonNilPtrConst()

	cond := &ssa.BinOp{Op: token.EQL}
	cond.X = variable
	cond.Y = newNilConst()

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {},
		},
	}

	a.refineFromCondition(block, cond, true)
	require.Equal(t, DefinitelyNil, a.state[block][variable],
		"p == nil, true branch should be DefinitelyNil")
}

// TestRefineFromCondition_EQL_FalseBranch tests p == nil on the false branch.
// Variable should become DefinitelyNotNil.
func TestRefineFromCondition_EQL_FalseBranch(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}
	variable := newNonNilPtrConst()

	cond := &ssa.BinOp{Op: token.EQL}
	cond.X = variable
	cond.Y = newNilConst()

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {},
		},
	}

	a.refineFromCondition(block, cond, false)
	require.Equal(t, DefinitelyNotNil, a.state[block][variable],
		"p == nil, false branch should be DefinitelyNotNil")
}

// TestRefineFromCondition_NilOnLeft tests nil == p (nil on X side).
// Should identify p (Y) as the variable.
func TestRefineFromCondition_NilOnLeft_EQL_True(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}
	variable := newNonNilPtrConst()

	cond := &ssa.BinOp{Op: token.EQL}
	cond.X = newNilConst() // nil on left
	cond.Y = variable      // variable on right

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {},
		},
	}

	a.refineFromCondition(block, cond, true)
	require.Equal(t, DefinitelyNil, a.state[block][variable],
		"nil == p, true branch should set p to DefinitelyNil")
}

// TestRefineFromCondition_NilOnLeft_NEQ_True tests nil != p on the true branch.
func TestRefineFromCondition_NilOnLeft_NEQ_True(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}
	variable := newNonNilPtrConst()

	cond := &ssa.BinOp{Op: token.NEQ}
	cond.X = newNilConst() // nil on left
	cond.Y = variable

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {},
		},
	}

	a.refineFromCondition(block, cond, true)
	require.Equal(t, DefinitelyNotNil, a.state[block][variable],
		"nil != p, true branch should set p to DefinitelyNotNil")
}

// TestRefineFromCondition_NilOnLeft_NEQ_False tests nil != p on the false branch.
func TestRefineFromCondition_NilOnLeft_NEQ_False(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}
	variable := newNonNilPtrConst()

	cond := &ssa.BinOp{Op: token.NEQ}
	cond.X = newNilConst()
	cond.Y = variable

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {},
		},
	}

	a.refineFromCondition(block, cond, false)
	require.Equal(t, DefinitelyNil, a.state[block][variable],
		"nil != p, false branch should set p to DefinitelyNil")
}

// TestRefineFromCondition_NilOnLeft_EQL_False tests nil == p on the false branch.
func TestRefineFromCondition_NilOnLeft_EQL_False(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}
	variable := newNonNilPtrConst()

	cond := &ssa.BinOp{Op: token.EQL}
	cond.X = newNilConst()
	cond.Y = variable

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {},
		},
	}

	a.refineFromCondition(block, cond, false)
	require.Equal(t, DefinitelyNotNil, a.state[block][variable],
		"nil == p, false branch should set p to DefinitelyNotNil")
}

// ---------------------------------------------------------------------------
// refineFromCondition: no-op / early-return cases
// ---------------------------------------------------------------------------

// TestRefineFromCondition_NeitherSideNil tests that comparing two non-nil
// values (e.g. p == q) does not modify state.
func TestRefineFromCondition_NeitherSideNil(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}
	p := newNonNilConst()
	q := newNonNilConst()

	cond := &ssa.BinOp{Op: token.EQL}
	cond.X = p
	cond.Y = q

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {},
		},
	}

	a.refineFromCondition(block, cond, true)

	_, hasP := a.state[block][p]
	_, hasQ := a.state[block][q]
	require.False(t, hasP, "p == q should not write state for p")
	require.False(t, hasQ, "p == q should not write state for q")
}

// TestRefineFromCondition_UnsupportedOp tests that an unsupported operator
// (e.g. token.LSS) does not modify state even when one side is nil.
func TestRefineFromCondition_UnsupportedOp(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}
	variable := newNonNilPtrConst()

	cond := &ssa.BinOp{Op: token.LSS} // < operator — not valid for nil checks
	cond.X = variable
	cond.Y = newNilConst()

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {},
		},
	}

	a.refineFromCondition(block, cond, true)

	_, has := a.state[block][variable]
	require.False(t, has, "unsupported op should not write state")
}

// TestRefineFromCondition_GTR_NoOp tests token.GTR is a no-op.
func TestRefineFromCondition_GTR_NoOp(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}
	variable := newNonNilPtrConst()

	cond := &ssa.BinOp{Op: token.GTR}
	cond.X = variable
	cond.Y = newNilConst()

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {},
		},
	}

	a.refineFromCondition(block, cond, true)

	_, has := a.state[block][variable]
	require.False(t, has, "GTR should not refine nil state")
}

// TestRefineFromCondition_LEQ_NoOp tests token.LEQ is a no-op.
func TestRefineFromCondition_LEQ_NoOp(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}
	variable := newNonNilPtrConst()

	cond := &ssa.BinOp{Op: token.LEQ}
	cond.X = variable
	cond.Y = newNilConst()

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {},
		},
	}

	a.refineFromCondition(block, cond, false)

	_, has := a.state[block][variable]
	require.False(t, has, "LEQ should not refine nil state")
}

// TestRefineFromCondition_AND_NoOp tests token.AND (bitwise) is a no-op.
func TestRefineFromCondition_AND_NoOp(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}
	variable := newNonNilPtrConst()

	cond := &ssa.BinOp{Op: token.AND}
	cond.X = variable
	cond.Y = newNilConst()

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {},
		},
	}

	a.refineFromCondition(block, cond, true)

	_, has := a.state[block][variable]
	require.False(t, has, "AND should not refine nil state")
}

// TestRefineFromCondition_BothNilConsts tests that when both sides are nil
// consts, the X side is treated as the nil const (X matches first) and Y
// becomes the variable. This is a degenerate case (nil == nil) but should
// not panic.
func TestRefineFromCondition_BothNilConsts(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}
	nilX := ssa.NewConst(nil, ptrType)
	nilY := ssa.NewConst(nil, ptrType)

	cond := &ssa.BinOp{Op: token.EQL}
	cond.X = nilX
	cond.Y = nilY

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {},
		},
	}

	// Should not panic. X is identified as nil, Y becomes the "variable".
	a.refineFromCondition(block, cond, true)
	require.Equal(t, DefinitelyNil, a.state[block][nilY],
		"both nil: EQL+true should set variable to DefinitelyNil")
}

// ---------------------------------------------------------------------------
// refineFromCondition: table-driven exhaustive test
// ---------------------------------------------------------------------------

// TestRefineFromCondition_Table exercises all 8 combinations of
// {EQL, NEQ} × {true, false} × {nil on X, nil on Y}.
func TestRefineFromCondition_Table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		op          token.Token
		nilOnLeft   bool // if true, nil const is cond.X; if false, cond.Y
		isTrueBranch bool
		want        NilState
	}{
		// nil on right (p OP nil)
		{"p==nil true", token.EQL, false, true, DefinitelyNil},
		{"p==nil false", token.EQL, false, false, DefinitelyNotNil},
		{"p!=nil true", token.NEQ, false, true, DefinitelyNotNil},
		{"p!=nil false", token.NEQ, false, false, DefinitelyNil},
		// nil on left (nil OP p)
		{"nil==p true", token.EQL, true, true, DefinitelyNil},
		{"nil==p false", token.EQL, true, false, DefinitelyNotNil},
		{"nil!=p true", token.NEQ, true, true, DefinitelyNotNil},
		{"nil!=p false", token.NEQ, true, false, DefinitelyNil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			block := &ssa.BasicBlock{}
			variable := newNonNilPtrConst()
			nilConst := newNilConst()

			cond := &ssa.BinOp{Op: tt.op}
			if tt.nilOnLeft {
				cond.X = nilConst
				cond.Y = variable
			} else {
				cond.X = variable
				cond.Y = nilConst
			}

			a := &NilAnalyzer{
				state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
					block: {},
				},
			}

			a.refineFromCondition(block, cond, tt.isTrueBranch)
			require.Equal(t, tt.want, a.state[block][variable])
		})
	}
}

// ===========================================================================
// refineFromPredecessor tests
// ===========================================================================

// ---------------------------------------------------------------------------
// Real SSA refineFromPredecessor tests
// ---------------------------------------------------------------------------

// TestRefineFromPredecessor_NotNil_TrueBranch tests that after `if p != nil`,
// the true branch has p as DefinitelyNotNil.
func TestRefineFromPredecessor_NotNil_TrueBranch(t *testing.T) {
	t.Parallel()

	_, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)

	fn := findFunc(t, pkgs[0], "RefineNotNil")
	require.NotEmpty(t, fn.Blocks)

	// Find the If instruction in the entry block.
	var ifBlock *ssa.BasicBlock
	for _, block := range fn.Blocks {
		lastInstr := block.Instrs[len(block.Instrs)-1]
		if _, ok := lastInstr.(*ssa.If); ok {
			ifBlock = block
			break
		}
	}
	require.NotNil(t, ifBlock, "should have a block ending with If")
	require.Len(t, ifBlock.Succs, 2, "If block should have 2 successors")

	trueBranch := ifBlock.Succs[0]
	falseBranch := ifBlock.Succs[1]

	// The param p is fn.Params[0].
	p := fn.Params[0]

	// Test true branch: p should be refined to DefinitelyNotNil.
	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			trueBranch: {p: MaybeNil}, // before refinement
		},
	}
	a.refineFromPredecessor(trueBranch)
	require.Equal(t, DefinitelyNotNil, a.state[trueBranch][p],
		"p != nil: true branch should refine p to DefinitelyNotNil")

	// Test false branch: p should be refined to DefinitelyNil.
	a2 := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			falseBranch: {p: MaybeNil},
		},
	}
	a2.refineFromPredecessor(falseBranch)
	require.Equal(t, DefinitelyNil, a2.state[falseBranch][p],
		"p != nil: false branch should refine p to DefinitelyNil")
}

// TestRefineFromPredecessor_EqlNil tests that after `if p == nil`,
// the true branch has p as DefinitelyNil, and the false branch as DefinitelyNotNil.
func TestRefineFromPredecessor_EqlNil(t *testing.T) {
	t.Parallel()

	_, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)

	fn := findFunc(t, pkgs[0], "RefineEqlNil")

	var ifBlock *ssa.BasicBlock
	for _, block := range fn.Blocks {
		lastInstr := block.Instrs[len(block.Instrs)-1]
		if _, ok := lastInstr.(*ssa.If); ok {
			ifBlock = block
			break
		}
	}
	require.NotNil(t, ifBlock)

	trueBranch := ifBlock.Succs[0]
	falseBranch := ifBlock.Succs[1]
	p := fn.Params[0]

	// True branch: p == nil → DefinitelyNil
	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			trueBranch: {p: MaybeNil},
		},
	}
	a.refineFromPredecessor(trueBranch)
	require.Equal(t, DefinitelyNil, a.state[trueBranch][p],
		"p == nil: true branch should refine p to DefinitelyNil")

	// False branch: p == nil is false → DefinitelyNotNil
	a2 := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			falseBranch: {p: MaybeNil},
		},
	}
	a2.refineFromPredecessor(falseBranch)
	require.Equal(t, DefinitelyNotNil, a2.state[falseBranch][p],
		"p == nil: false branch should refine p to DefinitelyNotNil")
}

// TestRefineFromPredecessor_NilOnLeft tests nil == p (nil constant on X side).
func TestRefineFromPredecessor_NilOnLeft(t *testing.T) {
	t.Parallel()

	_, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)

	fn := findFunc(t, pkgs[0], "RefineNilOnLeft")

	var ifBlock *ssa.BasicBlock
	for _, block := range fn.Blocks {
		lastInstr := block.Instrs[len(block.Instrs)-1]
		if _, ok := lastInstr.(*ssa.If); ok {
			ifBlock = block
			break
		}
	}
	require.NotNil(t, ifBlock)

	trueBranch := ifBlock.Succs[0]
	falseBranch := ifBlock.Succs[1]
	p := fn.Params[0]

	// True branch: nil == p → DefinitelyNil
	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			trueBranch: {p: MaybeNil},
		},
	}
	a.refineFromPredecessor(trueBranch)
	require.Equal(t, DefinitelyNil, a.state[trueBranch][p],
		"nil == p: true branch should refine p to DefinitelyNil")

	// False branch: nil == p is false → DefinitelyNotNil
	a2 := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			falseBranch: {p: MaybeNil},
		},
	}
	a2.refineFromPredecessor(falseBranch)
	require.Equal(t, DefinitelyNotNil, a2.state[falseBranch][p],
		"nil == p: false branch should refine p to DefinitelyNotNil")
}

// TestRefineFromPredecessor_Interface tests nil check on an interface value.
func TestRefineFromPredecessor_Interface(t *testing.T) {
	t.Parallel()

	_, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)

	fn := findFunc(t, pkgs[0], "RefineInterface")

	var ifBlock *ssa.BasicBlock
	for _, block := range fn.Blocks {
		lastInstr := block.Instrs[len(block.Instrs)-1]
		if _, ok := lastInstr.(*ssa.If); ok {
			ifBlock = block
			break
		}
	}
	require.NotNil(t, ifBlock)

	trueBranch := ifBlock.Succs[0]
	s := fn.Params[0]

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			trueBranch: {s: MaybeNil},
		},
	}
	a.refineFromPredecessor(trueBranch)
	require.Equal(t, DefinitelyNotNil, a.state[trueBranch][s],
		"s != nil: true branch should refine interface to DefinitelyNotNil")
}

// TestRefineFromPredecessor_Slice tests nil check on a slice value.
func TestRefineFromPredecessor_Slice(t *testing.T) {
	t.Parallel()

	_, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)

	fn := findFunc(t, pkgs[0], "RefineSlice")

	var ifBlock *ssa.BasicBlock
	for _, block := range fn.Blocks {
		lastInstr := block.Instrs[len(block.Instrs)-1]
		if _, ok := lastInstr.(*ssa.If); ok {
			ifBlock = block
			break
		}
	}
	require.NotNil(t, ifBlock)

	trueBranch := ifBlock.Succs[0]
	s := fn.Params[0]

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			trueBranch: {s: MaybeNil},
		},
	}
	a.refineFromPredecessor(trueBranch)
	require.Equal(t, DefinitelyNotNil, a.state[trueBranch][s],
		"s != nil: true branch should refine slice to DefinitelyNotNil")
}

// ---------------------------------------------------------------------------
// refineFromPredecessor: edge cases
// ---------------------------------------------------------------------------

// TestRefineFromPredecessor_NoPredecessors tests a block with no predecessors
// (entry block). refineFromPredecessor should be a no-op.
func TestRefineFromPredecessor_NoPredecessors(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}
	// No predecessors — block.Preds is empty.

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {},
		},
	}

	// Should not panic, should be a no-op.
	a.refineFromPredecessor(block)
	require.Empty(t, a.state[block], "no predecessors should leave state empty")
}

// TestRefineFromPredecessor_PredNotEndingWithIf tests that a predecessor
// ending with a non-If instruction (e.g. Jump) doesn't refine anything.
func TestRefineFromPredecessor_PredNotEndingWithIf(t *testing.T) {
	t.Parallel()

	// Build a synthetic CFG: pred -> block, where pred ends with Jump (not If).
	pred := &ssa.BasicBlock{}
	block := &ssa.BasicBlock{}
	block.Preds = []*ssa.BasicBlock{pred}

	// pred ends with a Jump (represented by any non-If instruction).
	pred.Instrs = []ssa.Instruction{&ssa.Jump{}}

	param := newNonNilPtrConst()

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {param: MaybeNil},
		},
	}

	a.refineFromPredecessor(block)
	require.Equal(t, MaybeNil, a.state[block][param],
		"predecessor ending with Jump should not refine state")
}

// TestRefineFromPredecessor_CondNotBinOp tests that when the If condition
// is not a *ssa.BinOp (e.g. a plain bool value), no refinement occurs.
// We test this synthetically since real SSA typically uses BinOp for nil checks.
func TestRefineFromPredecessor_CondNotBinOp(t *testing.T) {
	t.Parallel()

	// Build a synthetic CFG: pred -> block, where pred ends with If
	// but the condition is a bool constant, not a BinOp.
	pred := &ssa.BasicBlock{}
	block := &ssa.BasicBlock{}
	block.Preds = []*ssa.BasicBlock{pred}
	pred.Succs = []*ssa.BasicBlock{block, {}}

	// The If condition must be an ssa.Value. Use a bool Const.
	boolConst := ssa.NewConst(constant.MakeBool(true), types.Typ[types.Bool])
	ifInstr := &ssa.If{}
	ifInstr.Cond = boolConst
	pred.Instrs = []ssa.Instruction{ifInstr}

	param := ssa.NewConst(nil, ptrType)

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {param: MaybeNil},
		},
	}

	a.refineFromPredecessor(block)
	require.Equal(t, MaybeNil, a.state[block][param],
		"If condition that's not a BinOp should not refine state")
}

// TestRefineFromPredecessor_NonNilComparison tests that if p == q (two
// non-nil values compared), no refinement occurs.
func TestRefineFromPredecessor_NonNilComparison(t *testing.T) {
	t.Parallel()

	pred := &ssa.BasicBlock{}
	block := &ssa.BasicBlock{}
	block.Preds = []*ssa.BasicBlock{pred}
	pred.Succs = []*ssa.BasicBlock{block, {}}

	p := newNonNilConst()
	q := newNonNilConst()

	cond := &ssa.BinOp{Op: token.EQL}
	cond.X = p
	cond.Y = q

	ifInstr := &ssa.If{}
	ifInstr.Cond = cond
	pred.Instrs = []ssa.Instruction{ifInstr}

	a := &NilAnalyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]NilState{
			block: {},
		},
	}

	a.refineFromPredecessor(block)

	_, hasP := a.state[block][p]
	_, hasQ := a.state[block][q]
	require.False(t, hasP, "non-nil comparison should not add p to state")
	require.False(t, hasQ, "non-nil comparison should not add q to state")
}
