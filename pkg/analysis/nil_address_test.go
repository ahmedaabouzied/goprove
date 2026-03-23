package analysis

import (
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
)

// ===========================================================================
// nil_address.go unit tests
// ===========================================================================

func TestResolveAddress_Global(t *testing.T) {
	t.Parallel()

	g := &ssa.Global{}
	key, ok := resolveAddress(g)
	require.True(t, ok)
	require.Equal(t, addrGlobal, key.kind)
	require.Equal(t, ssa.Value(g), key.base)
	require.Equal(t, -1, key.field)
}

func TestResolveAddress_FieldAddr(t *testing.T) {
	t.Parallel()

	param := ssa.NewConst(nil, types.NewPointer(types.Typ[types.Int]))
	fa := &ssa.FieldAddr{Field: 3}
	fa.X = param

	key, ok := resolveAddress(fa)
	require.True(t, ok)
	require.Equal(t, addrField, key.kind)
	require.Equal(t, ssa.Value(param), key.base)
	require.Equal(t, 3, key.field)
}

func TestResolveAddress_IndexAddr(t *testing.T) {
	t.Parallel()

	param := ssa.NewConst(nil, types.NewSlice(types.Typ[types.Int]))
	ia := &ssa.IndexAddr{}
	ia.X = param

	key, ok := resolveAddress(ia)
	require.True(t, ok)
	require.Equal(t, addrIndex, key.kind)
	require.Equal(t, ssa.Value(param), key.base)
	require.Equal(t, -1, key.field)
}

func TestResolveAddress_Unsupported(t *testing.T) {
	t.Parallel()

	// Parameter is not a trackable address.
	param := ssa.NewConst(nil, types.NewPointer(types.Typ[types.Int]))
	_, ok := resolveAddress(param)
	require.False(t, ok, "Const should not be a resolvable address")
}

func TestResolveAddress_TwoFieldAddrs_SameBaseField_Match(t *testing.T) {
	t.Parallel()

	// Two different FieldAddr instructions with the same base and field index
	// should produce equal addressKeys.
	param := ssa.NewConst(nil, types.NewPointer(types.Typ[types.Int]))

	fa1 := &ssa.FieldAddr{Field: 2}
	fa1.X = param
	fa2 := &ssa.FieldAddr{Field: 2}
	fa2.X = param

	key1, ok1 := resolveAddress(fa1)
	key2, ok2 := resolveAddress(fa2)
	require.True(t, ok1)
	require.True(t, ok2)
	require.Equal(t, key1, key2,
		"same base + same field index → equal addressKey")
}

func TestResolveAddress_TwoFieldAddrs_DifferentField_NoMatch(t *testing.T) {
	t.Parallel()

	param := ssa.NewConst(nil, types.NewPointer(types.Typ[types.Int]))

	fa1 := &ssa.FieldAddr{Field: 0}
	fa1.X = param
	fa2 := &ssa.FieldAddr{Field: 1}
	fa2.X = param

	key1, _ := resolveAddress(fa1)
	key2, _ := resolveAddress(fa2)
	require.NotEqual(t, key1, key2,
		"different field index → different addressKey")
}

func TestResolveAddress_TwoFieldAddrs_DifferentBase_NoMatch(t *testing.T) {
	t.Parallel()

	// Use two different Const values with different types as bases.
	base1 := ssa.NewConst(nil, types.NewPointer(types.Typ[types.Int]))
	base2 := ssa.NewConst(nil, types.NewPointer(types.Typ[types.String]))

	fa1 := &ssa.FieldAddr{Field: 0}
	fa1.X = base1
	fa2 := &ssa.FieldAddr{Field: 0}
	fa2.X = base2

	key1, _ := resolveAddress(fa1)
	key2, _ := resolveAddress(fa2)
	require.NotEqual(t, key1, key2,
		"different base → different addressKey")
}

func TestResolveLoadAddress_MUL(t *testing.T) {
	t.Parallel()

	g := &ssa.Global{}
	unOp := &ssa.UnOp{Op: token.MUL}
	unOp.X = g

	key, ok := resolveLoadAddress(unOp)
	require.True(t, ok)
	require.Equal(t, addrGlobal, key.kind)
}

func TestResolveLoadAddress_NotMUL(t *testing.T) {
	t.Parallel()

	unOp := &ssa.UnOp{Op: token.SUB}
	_, ok := resolveLoadAddress(unOp)
	require.False(t, ok, "non-MUL UnOp should not resolve")
}

func TestResolveLoadAddress_UntrackedSource(t *testing.T) {
	t.Parallel()

	param := ssa.NewConst(nil, types.NewPointer(types.Typ[types.Int]))
	unOp := &ssa.UnOp{Op: token.MUL}
	unOp.X = param

	_, ok := resolveLoadAddress(unOp)
	require.False(t, ok, "load from non-address source should not resolve")
}
