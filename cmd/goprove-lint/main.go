// goprove-lint is a go/analysis-compatible binary that can be used
// as a golangci-lint plugin or with go vet.
//
// Usage:
//
//	goprove-lint ./...
//
// Or with go vet:
//
//	go vet -vettool=$(which goprove-lint) ./...
package main

import (
	"github.com/ahmedaabouzied/goprove/pkg/analyzer"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(analyzer.Analyzer)
}
