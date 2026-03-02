package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"github.com/ahmedaabouzied/goprove/pkg/loader"
	"golang.org/x/tools/go/ssa"
)

// provePackage is the main entry point for our prover.
func provePackage(path string) error {
	_, pkgs, err := loader.Load(path)
	if err != nil {
		return err
	}
	if len(pkgs) < 1 {
		return fmt.Errorf("no packages found at %s", path)
	}

	for _, pkg := range pkgs {
		if err := analyzePkg(pkg); err != nil {
			return err
		}
	}
	return nil
}

func analyzePkg(pkg *ssa.Package) error {
	for _, member := range pkg.Members {
		fn, ok := member.(*ssa.Function)
		if !ok {
			continue
		}
		if err := analyzeFunction(fn); err != nil {
			return err
		}
	}
	return nil
}

func analyzeFunction(fn *ssa.Function) error {
	w := bufio.NewWriter(os.Stdout)
	analyzer := analysis.Analyzer{}
	findings := analyzer.Analyze(fn)
	for _, finding := range findings {
		printFinding(w, finding)
	}
	return w.Flush()
}

func printFinding(w *bufio.Writer, finding analysis.Finding) {
	switch finding.Severity {
	case analysis.Bug:
		fmt.Fprintf(w, "\033[31m %d %s \n", finding.Pos, finding.Message)
	case analysis.Warning:
		fmt.Fprintf(w, "\033[33m %d %s \n", finding.Pos, finding.Message)
	case analysis.Safe:
		fmt.Fprintf(w, "\033[32m %d %s \n", finding.Pos, finding.Message)
	default:
	}
}
