// Package analyzer provides go/analysis.Analyzer wrappers for GoProve's
// nil pointer, division by zero, and integer overflow analyzers.
//
// This enables integration with golangci-lint, go vet, and any tool
// that supports the go/analysis framework.
//
// Usage with singlechecker (standalone):
//
//	func main() {
//	    singlechecker.Main(analyzer.Analyzer)
//	}
//
// Usage with multichecker:
//
//	func main() {
//	    multichecker.Main(analyzer.NilAnalyzer, analyzer.IntervalAnalyzer)
//	}
package analyzer

import (
	"fmt"
	"go/token"

	goanalysis "golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"

	proveanalysis "github.com/ahmedaabouzied/goprove/pkg/analysis"
)

// Analyzer is the combined GoProve analyzer that runs both nil pointer
// and interval (division by zero, integer overflow) analysis.
// Use this for a single-analyzer integration.
var Analyzer = &goanalysis.Analyzer{
	Name:     "goprove",
	Doc:      "proves nil safety, division by zero, and integer overflow using abstract interpretation",
	URL:      "https://github.com/ahmedaabouzied/goprove",
	Requires: []*goanalysis.Analyzer{buildssa.Analyzer},
	Run:      runCombined,
}

// NilAnalyzer runs only the nil pointer dereference analysis.
var NilAnalyzer = &goanalysis.Analyzer{
	Name:     "goprovenil",
	Doc:      "proves nil pointer safety using abstract interpretation with address-based memory model",
	URL:      "https://github.com/ahmedaabouzied/goprove",
	Requires: []*goanalysis.Analyzer{buildssa.Analyzer},
	Run:      runNil,
}

// IntervalAnalyzer runs only the interval analysis (division by zero, overflow).
var IntervalAnalyzer = &goanalysis.Analyzer{
	Name:     "goproveinterval",
	Doc:      "proves division by zero and integer overflow safety using interval abstract interpretation",
	URL:      "https://github.com/ahmedaabouzied/goprove",
	Requires: []*goanalysis.Analyzer{buildssa.Analyzer},
	Run:      runInterval,
}

func runCombined(pass *goanalysis.Pass) (interface{}, error) {
	ssaResult := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	reportFindings(pass, analyzePackage(ssaResult.Pkg, ssaResult.SrcFuncs, true, true))
	return nil, nil
}

func runNil(pass *goanalysis.Pass) (interface{}, error) {
	ssaResult := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	reportFindings(pass, analyzePackage(ssaResult.Pkg, ssaResult.SrcFuncs, true, false))
	return nil, nil
}

func runInterval(pass *goanalysis.Pass) (interface{}, error) {
	ssaResult := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	reportFindings(pass, analyzePackage(ssaResult.Pkg, ssaResult.SrcFuncs, false, true))
	return nil, nil
}

// analyzePackage runs the requested analyzers on all functions in the package.
func analyzePackage(
	pkg *ssa.Package,
	srcFuncs []*ssa.Function,
	runNil bool,
	runInterval bool,
) []proveanalysis.Finding {
	var findings []proveanalysis.Finding

	// Build analyzers.
	// Note: whole-program parameter analysis is not available in go/analysis
	// mode because the framework analyzes one package at a time. Interprocedural
	// return summaries still work within the package scope.
	var nilAnalyzer *proveanalysis.NilAnalyzer
	var intervalAnalyzer *proveanalysis.Analyzer

	if runNil {
		// Compute param nil states from call sites within this package.
		paramStates := proveanalysis.ComputeParamNilStates(nil, []*ssa.Package{pkg})
		nilAnalyzer = proveanalysis.NewNilAnalyzer(nil, paramStates)
	}
	if runInterval {
		intervalAnalyzer = proveanalysis.NewAnalyzer(nil)
	}

	// Deduplicate findings by position + message.
	seen := make(map[string]bool)

	for _, fn := range srcFuncs {
		if runInterval && intervalAnalyzer != nil {
			for _, f := range intervalAnalyzer.Analyze(fn) {
				key := fmt.Sprintf("%d:%s", f.Pos, f.Message)
				if !seen[key] {
					seen[key] = true
					findings = append(findings, f)
				}
			}
		}
		if runNil && nilAnalyzer != nil {
			for _, f := range nilAnalyzer.Analyze(fn) {
				key := fmt.Sprintf("%d:%s", f.Pos, f.Message)
				if !seen[key] {
					seen[key] = true
					findings = append(findings, f)
				}
			}
		}
	}

	return findings
}

// reportFindings converts GoProve findings into go/analysis diagnostics.
func reportFindings(pass *goanalysis.Pass, findings []proveanalysis.Finding) {
	for _, f := range findings {
		var category string
		switch f.Severity {
		case proveanalysis.Bug:
			category = "BUG"
		case proveanalysis.Warning:
			category = "WARNING"
		default:
			continue
		}

		pass.Report(goanalysis.Diagnostic{
			Pos:      f.Pos,
			End:      token.NoPos,
			Category: category,
			Message:  f.Message,
		})
	}
}
