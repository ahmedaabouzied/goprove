package loader

import (
	"fmt"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func Load(patterns ...string) (*ssa.Program, []*ssa.Package, error) {
	cfg := packages.Config{Mode: packages.LoadAllSyntax}
	initial, err := packages.Load(&cfg, patterns...)
	if err != nil {
		return nil, nil, err
	}

	prog, pkgs := ssautil.AllPackages(initial, ssa.SanityCheckFunctions|ssa.InstantiateGenerics)
	prog.Build()

	for _, pkg := range initial {
		if len(pkg.Errors) > 0 {
			return nil, nil, fmt.Errorf("package %s has build errors. Run `go build` to check them out", pkg.Dir)
		}
	}

	return prog, pkgs, nil
}
