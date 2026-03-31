package analysis

import (
	"go/token"

	"golang.org/x/tools/go/ssa"
)

// addressKey identifies a memory address in the abstract heap.
// Two addressKeys are equal if they refer to the same runtime memory location.
type addressKey struct {
	// base is the SSA value that the address is derived from.
	// For FieldAddr: the struct pointer (e.g., parameter 'o').
	// For Global: the *ssa.Global itself.
	// For IndexAddr: the array/slice pointer.
	base ssa.Value

	// field is the field or element index for composite addresses.
	// For FieldAddr: the struct field index.
	// For Global: -1 (not applicable).
	// For IndexAddr: -1 (we don't track individual indices).
	field int

	// kind distinguishes address types with the same base.
	kind addressKind
}

type addressKind uint8

const (
	addrGlobal addressKind = iota // package-level variable
	addrField                     // struct field: base.field
	addrIndex                     // array/slice element: base[i]
	addrLocal                     // address taken local variable
)

// resolveAddress extracts an addressKey from an SSA value that represents
// a memory address (the operand of a Load or Store).
//
// Returns the key and true if the address can be resolved.
// Returns zero key and false if the address is not trackable.
func resolveAddress(addr ssa.Value) (addressKey, bool) {
	switch v := addr.(type) {
	case *ssa.Global:
		return addressKey{base: v, field: -1, kind: addrGlobal}, true
	case *ssa.FieldAddr:
		return addressKey{base: v.X, field: v.Field, kind: addrField}, true
	case *ssa.IndexAddr:
		return addressKey{base: v.X, field: -1, kind: addrIndex}, true
	case *ssa.Alloc:
		return addressKey{base: v, field: -1, kind: addrLocal}, true
	default:
		return addressKey{}, false
	}
}

// resolveLoadAddress extracts the addressKey for a Load instruction.
// A Load is *ssa.UnOp with Op == token.MUL.
// Returns the key and true if this is a load from a trackable address.
func resolveLoadAddress(v *ssa.UnOp) (addressKey, bool) {
	if v.Op != token.MUL {
		return addressKey{}, false
	}
	return resolveAddress(v.X)
}
