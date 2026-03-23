// goprove-multi exposes GoProve's analyzers individually for tools
// that support multiple analyzers (e.g., go vet with -vettool).
//
// Available analyzers:
//   - goprovenil: nil pointer dereference detection
//   - goproveinterval: division by zero and integer overflow detection
//
// Usage:
//
//	goprove-multi -goprovenil -goproveinterval ./...
package main

import (
	"github.com/ahmedaabouzied/goprove/pkg/analyzer"
	"golang.org/x/tools/go/analysis/multichecker"
)

func main() {
	multichecker.Main(
		analyzer.NilAnalyzer,
		analyzer.IntervalAnalyzer,
	)
}
