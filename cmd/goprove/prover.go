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
	nilAnalyzer := &analysis.NilAnalyzer{}
	return &Prover{path, analyzer, nilAnalyzer, prog, pkgs}, nil
}

// provePackage is the main entry point for our prover.
func (p *Prover) Prove() error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	for _, pkg := range p.pkgs {
		if pkg == nil {
			continue
		}
		if err := p.analyzePkg(wd, p.prog.Fset, pkg); err != nil {
			return err
		}
	}
	return nil
}

func (p *Prover) analyzePkg(wd string, fset *token.FileSet, pkg *ssa.Package) error {
	findings := []analysis.Finding{}

	for _, member := range pkg.Members {
		fn, ok := member.(*ssa.Function)
		if !ok {
			continue
		}
		findings = append(findings, p.analyzeFunction(fn)...)
	}

	// sort findings
	sort.Slice(findings, func(i, j int) bool {
		// Sort the findings by severity first
		if findings[i].Severity != findings[j].Severity {
			return findings[i].Severity > findings[j].Severity
		}
		// Sort the findings by line number
		return findings[i].Pos < findings[j].Pos
	})

	w := bufio.NewWriter(os.Stdout)
	for _, finding := range findings {
		printFinding(wd, w, fset, finding)
	}

	return w.Flush()
}

func (p *Prover) analyzeFunction(fn *ssa.Function) []analysis.Finding {
	findings := p.intervalAnalyzer.Analyze(fn)
	findings = append(findings, p.nilAnalyzer.Analyze(fn)...)
	return findings
}

func printFinding(wd string, w *bufio.Writer, fset *token.FileSet, finding analysis.Finding) error {
	pos := fset.Position(finding.Pos)
	fileName, err := filepath.Rel(wd, pos.Filename)
	if err != nil {
		return err
	}
	pos.Filename = fileName
	switch finding.Severity {
	case analysis.Bug:
		fmt.Fprintf(w, "\033[31m Error: %s %s \033[0m\n", pos, finding.Message)
	case analysis.Warning:
		fmt.Fprintf(w, "\033[33m Warning: %s %s \033[0m\n", pos, finding.Message)
	default:
	}
	return nil
}
