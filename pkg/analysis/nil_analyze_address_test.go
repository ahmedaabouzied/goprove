package analysis_test

import (
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"github.com/ahmedaabouzied/goprove/pkg/loader"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
)

// TestAddress_FieldNotChecked verifies that a field access WITHOUT a nil
// check still produces a warning.
func TestAddress_FieldNotChecked(t *testing.T) {
	t.Parallel()
	fn, _ := findTestdataFunc(t, "AddrFieldNotChecked")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.NotEmpty(t, findings,
		"field access without nil check should warn")
}

// TestAddress_FieldReload tests if o.In != nil { o.In.Val } — two loads
// from the same field. The second load should inherit the nil check.
func TestAddress_FieldReload(t *testing.T) {
	t.Parallel()
	fn, _ := findTestdataFunc(t, "AddrFieldReload")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// Only the parameter 'o' itself should warn (MaybeNil).
	// The field o.In should NOT warn after the nil check.
	nilFindings := 0
	for _, f := range findings {
		if f.Severity == analysis.Warning || f.Severity == analysis.Bug {
			nilFindings++
		}
	}
	// Expect 1 warning max: the parameter o is unchecked.
	// o.In after nil check should NOT produce a warning.
	require.LessOrEqual(t, nilFindings, 1,
		"field reload after nil check should not produce extra warnings")
}

// TestAddress_FieldReloadMultiple tests multiple accesses of o.In.Val
// after a single nil check on o.In.
func TestAddress_FieldReloadMultiple(t *testing.T) {
	t.Parallel()
	fn, _ := findTestdataFunc(t, "AddrFieldReloadMultiple")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	nilFindings := 0
	for _, f := range findings {
		if f.Severity == analysis.Warning || f.Severity == analysis.Bug {
			nilFindings++
		}
	}
	require.LessOrEqual(t, nilFindings, 1,
		"multiple field reloads after nil check should not produce extra warnings")
}

// TestAddress_NestedFieldCheck tests nested if blocks where the field
// nil check is in an outer block and the use is in an inner block.
func TestAddress_NestedFieldCheck(t *testing.T) {
	t.Parallel()
	fn, _ := findTestdataFunc(t, "AddrNestedFieldCheck")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	nilFindings := 0
	for _, f := range findings {
		if f.Severity == analysis.Warning || f.Severity == analysis.Bug {
			nilFindings++
		}
	}
	require.LessOrEqual(t, nilFindings, 1,
		"nested field check should propagate to inner blocks")
}

// ===========================================================================
// Address model tests
//
// These tests verify that nil state is tracked per memory address, not per
// SSA register. When the same field is loaded twice, the second load should
// inherit the nil state from the first if it was refined by a nil check.
// ===========================================================================
func findTestdataFunc(t *testing.T, name string) (*ssa.Function, []*ssa.Package) {
	t.Helper()
	_, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)

	for _, member := range pkgs[0].Members {
		fn, ok := member.(*ssa.Function)
		if ok && fn.Name() == name {
			return fn, pkgs
		}
	}
	t.Fatalf("function %s not found in testdata", name)
	return nil, nil
}
