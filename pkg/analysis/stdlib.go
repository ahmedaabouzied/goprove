package analysis

import (
	"fmt"
	"sort"

	"github.com/ahmedaabouzied/goprove/pkg/loader"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// GenerateStdlibCache loads all Go standard library
// packages, analyzes every function for nil return
// states, and returns a populated SummaryCache.
// The progressFn callback is invoked for each package
// being analyzed.
func GenerateStdlibCache(
	goproveVersion string,
	progressFn func(current, total int, pkgPath string),
) (*SummaryCache, error) {
	prog, pkgs, err := loader.Load("std")
	if err != nil {
		return nil, fmt.Errorf("loading stdlib: %w", err)
	}

	stdlibPkgs := make(map[*ssa.Package]bool, len(pkgs))
	for _, pkg := range pkgs {
		if pkg != nil {
			stdlibPkgs[pkg] = true
		}
	}

	// Collect all functions including methods, closures,
	// and init functions. Skip external/assembly stubs
	// that have no Go body to analyze.
	allFuncs := ssautil.AllFunctions(prog)
	var funcs []*ssa.Function
	for fn := range allFuncs {
		if fn.Package() != nil && stdlibPkgs[fn.Package()] && len(fn.Blocks) > 0 {
			funcs = append(funcs, fn)
		}
	}

	// Sort by package path then function name for stable
	// progress reporting and deterministic output.
	sort.Slice(funcs, func(i, j int) bool {
		iPkg := funcs[i].Package().Pkg.Path()
		jPkg := funcs[j].Package().Pkg.Path()
		if iPkg != jPkg {
			return iPkg < jPkg
		}
		return funcs[i].RelString(nil) < funcs[j].RelString(nil)
	})

	resolver := NewCHAResolver(prog)
	analyzer := NewNilAnalyzer(resolver, nil, nil)
	analyzer.SetTargetPackages(pkgs)

	cache := NewSummaryCache()
	cache.SetGoproveVersion(goproveVersion)

	currentPkg := ""
	pkgIndex := 0

	// Count unique packages for progress.
	pkgSet := make(map[string]bool)
	for _, fn := range funcs {
		pkgSet[fn.Package().Pkg.Path()] = true
	}
	totalPkgs := len(pkgSet)

	for _, fn := range funcs {
		pkg := fn.Package().Pkg.Path()
		if pkg != currentPkg {
			currentPkg = pkg
			pkgIndex++
			if progressFn != nil {
				progressFn(pkgIndex, totalPkgs, pkg)
			}
		}

		states := safeSummarize(analyzer, fn)
		if states != nil {
			cache.Set(fn.RelString(nil), states)
		}
	}

	return cache, nil
}

// safeSummarize wraps SummarizeFunction with a
// recover to handle panics from edge cases in
// stdlib SSA (assembly shims, generics, etc.).
func safeSummarize(a *NilAnalyzer, fn *ssa.Function) (states []NilState) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("warning: skipped %s: %v\n", fn.RelString(nil), r)
			states = nil
		}
	}()
	return a.SummarizeFunction(fn)
}

