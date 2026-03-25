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
	"golang.org/x/tools/go/ssa"
)

type Prover struct {
	path             string
	intervalAnalyzer *analysis.Analyzer
	nilAnalyzer      *analysis.NilAnalyzer
	prog             *ssa.Program
	pkgs             []*ssa.Package
}

func NewProver(path string) (*Prover, error) {
	prog, pkgs, err := loader.Load(path)
	if err != nil {
		return nil, err
	}
	if len(pkgs) < 1 {
		return nil, fmt.Errorf("no packages found at %s", path)
	}
	resolver := analysis.NewCHAResolver(prog)
	analyzer := analysis.NewAnalyzer(resolver)
	analyzer.SetTargetPackages(pkgs)
	nilAnalyzer := analysis.NewNilAnalyzer(resolver, nil)
	nilAnalyzer.SetTargetPackages(pkgs)
	paramStates := analysis.ComputeParamNilStatesAnalysis(pkgs, nilAnalyzer)
	nilAnalyzer.SetParamNilStates(paramStates)
	return &Prover{path, analyzer, nilAnalyzer, prog, pkgs}, nil
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
	var allFindings []analysis.Finding
	for _, pkg := range p.pkgs {
		if pkg == nil || analyzedPkgs[pkg] {
			continue
		}
		analyzedPkgs[pkg] = true
		allFindings = append(allFindings, p.analyzePkg(pkg)...)
	}

	// Sort findings by severity (bugs first), then by position.
	sort.Slice(allFindings, func(i, j int) bool {
		if allFindings[i].Severity != allFindings[j].Severity {
			return allFindings[i].Severity > allFindings[j].Severity
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
		output := formatFinding(wd, fset, finding)
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

func (p *Prover) analyzeFunction(fn *ssa.Function) []analysis.Finding {
	findings := p.intervalAnalyzer.Analyze(fn)
	findings = append(findings, p.nilAnalyzer.Analyze(fn)...)
	return findings
}

func printFinding(wd string, w *bufio.Writer, fset *token.FileSet, finding analysis.Finding) error {
	_, err := fmt.Fprint(w, formatFinding(wd, fset, finding))
	return err
}

func formatFinding(wd string, fset *token.FileSet, finding analysis.Finding) string {
	pos := fset.Position(finding.Pos)
	fileName, err := filepath.Rel(wd, pos.Filename)
	if err != nil {
		fileName = pos.Filename
	}
	pos.Filename = fileName
	switch finding.Severity {
	case analysis.Bug:
		return fmt.Sprintf("\033[31m Error: %s %s \033[0m\n", pos, finding.Message)
	case analysis.Warning:
		return fmt.Sprintf("\033[33m Warning: %s %s \033[0m\n", pos, finding.Message)
	default:
		return ""
	}
}
