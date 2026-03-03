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

// provePackage is the main entry point for our prover.
func provePackage(path string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	prog, pkgs, err := loader.Load(path)
	if err != nil {
		return err
	}
	if len(pkgs) < 1 {
		return fmt.Errorf("no packages found at %s", path)
	}

	for _, pkg := range pkgs {
		if err := analyzePkg(wd, prog.Fset, pkg); err != nil {
			return err
		}
	}
	return nil
}

func analyzePkg(wd string, fset *token.FileSet, pkg *ssa.Package) error {
	findings := []analysis.Finding{}

	for _, member := range pkg.Members {
		fn, ok := member.(*ssa.Function)
		if !ok {
			continue
		}
		findings = append(findings, analyzeFunction(fn)...)
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

func analyzeFunction(fn *ssa.Function) []analysis.Finding {
	analyzer := analysis.Analyzer{}
	return analyzer.Analyze(fn)
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
