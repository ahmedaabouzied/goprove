package main

import (
	"bufio"
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"sort"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"github.com/ahmedaabouzied/goprove/pkg/loader"
	"github.com/ahmedaabouzied/goprove/pkg/version"
	"golang.org/x/tools/go/ssa"
)

func NewProver(path string, progress *Progress, noColor bool) (*Prover, error) {
	done := progress.Phase("Loading packages")
	prog, pkgs, err := loader.Load(path)
	if err != nil {
		return nil, err
	}
	done()
	if len(pkgs) < 1 {
		return nil, fmt.Errorf("no packages found at %s", path)
	}

	done = progress.Phase("Building call graph")
	resolver := analysis.NewCHAResolver(prog)
	done()

	analyzer := analysis.NewAnalyzer(resolver)
	analyzer.SetTargetPackages(pkgs)

	// Load nil analysis cache: project-local first, then
	// global stdlib cache as fallback.
	cache, _ := analysis.LoadSummaryCache(".goprove/cache.json")
	if stdlibPath, pathErr := analysis.DefaultCachePath(version.Version); pathErr == nil {
		if stdlibCache, loadErr := analysis.LoadAndValidateCache(stdlibPath, version.Version); loadErr == nil {
			if cache == nil {
				cache = stdlibCache
			} else {
				cache.Merge(stdlibCache)
			}
		}
	}

	nilAnalyzer := analysis.NewNilAnalyzer(resolver, nil, cache)
	nilAnalyzer.SetTargetPackages(pkgs)

	done = progress.Phase("Computing parameter states")
	paramStates := analysis.ComputeParamNilStatesAnalysis(pkgs, nilAnalyzer)
	nilAnalyzer.SetParamNilStates(paramStates)
	done()

	return &Prover{path, analyzer, nilAnalyzer, prog, pkgs, progress, noColor}, nil
}

type Prover struct {
	path             string
	intervalAnalyzer *analysis.Analyzer
	nilAnalyzer      *analysis.NilAnalyzer
	prog             *ssa.Program
	pkgs             []*ssa.Package
	progress         *Progress
	noColor          bool
}

// Prove is the main entry point for our prover.
// Returns the number of unique findings and any error.
func (p *Prover) Prove() (int, error) {
	wd, err := os.Getwd()
	if err != nil {
		return 0, err
	}

	// Collect findings from all packages.
	// Deduplicate packages — ssautil.AllPackages can return the same
	// package multiple times when using ./... patterns.
	analyzedPkgs := make(map[*ssa.Package]bool)
	total := len(p.pkgs)
	count := 0
	var allFindings []analysis.Finding
	for _, pkg := range p.pkgs {
		if pkg == nil || analyzedPkgs[pkg] {
			continue
		}
		analyzedPkgs[pkg] = true
		count++
		p.progress.Pkg(count, total, pkg.Pkg.Path())
		allFindings = append(allFindings, p.analyzePkg(pkg)...)
	}
	p.progress.Done()

	// Sort findings by severity (warnings first, bugs last), then by position.
	sort.Slice(allFindings, func(i, j int) bool {
		if allFindings[i].Severity != allFindings[j].Severity {
			return allFindings[i].Severity < allFindings[j].Severity
		}
		return allFindings[i].Pos < allFindings[j].Pos
	})

	// Deduplicate across all packages.
	// The same source line can be reported via different analysis paths
	// (interprocedural summaries, CHA dispatch, multi-package ./...).
	// Use file:line:message as key (not token.Pos, which differs per FileSet).
	seen := make(map[string]bool)

	bugs := 0
	warnings := 0

	w := bufio.NewWriter(os.Stdout)
	fset := p.prog.Fset
	for _, finding := range allFindings {
		output := formatFinding(wd, fset, finding, p.noColor)
		if output == "" {
			continue
		}
		if seen[output] {
			continue
		}
		seen[output] = true

		switch finding.Severity {
		case analysis.Bug:
			bugs += 1
		case analysis.Warning:
			warnings += 1
			// We don't care about counting others.
		default:
		}

		fmt.Fprint(w, output)
	}

	fmt.Fprintf(w, "\n Summary: %d bugs, %d warnings.\n", bugs, warnings)
	return len(seen), w.Flush()
}

func (p *Prover) analyzeFunction(fn *ssa.Function) []analysis.Finding {
	findings := p.intervalAnalyzer.Analyze(fn)
	findings = append(findings, p.nilAnalyzer.Analyze(fn)...)
	return findings
}

func (p *Prover) analyzePkg(pkg *ssa.Package) []analysis.Finding {
	var findings []analysis.Finding
	for _, member := range pkg.Members {
		fn, ok := member.(*ssa.Function)
		if !ok {
			continue
		}
		findings = append(findings, p.analyzeFunction(fn)...)
	}
	return findings
}

func formatFinding(wd string, fset *token.FileSet, finding analysis.Finding, noColor bool) string {
	pos := fset.Position(finding.Pos)
	fileName, err := filepath.Rel(wd, pos.Filename)
	if err != nil {
		fileName = pos.Filename
	}
	pos.Filename = fileName
	switch finding.Severity {
	case analysis.Bug:
		if noColor {
			return fmt.Sprintf(" Error: %s %s \n", pos, finding.Message)
		}
		return fmt.Sprintf("\033[31m Error: %s %s \033[0m\n", pos, finding.Message)
	case analysis.Warning:
		if noColor {
			return fmt.Sprintf(" Warning: %s %s \n", pos, finding.Message)
		}
		return fmt.Sprintf("\033[33m Warning: %s %s \033[0m\n", pos, finding.Message)
	default:
		return ""
	}
}

func printFinding(wd string, w *bufio.Writer, fset *token.FileSet, finding analysis.Finding, noColor bool) error {
	_, err := fmt.Fprint(w, formatFinding(wd, fset, finding, noColor))
	return err
}
