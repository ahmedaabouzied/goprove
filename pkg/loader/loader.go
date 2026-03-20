package loader

import (
	"fmt"
	"os"

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
		for _, pkgErr := range pkg.Errors {
			if pkgErr.Kind == packages.ListError || pkgErr.Kind == packages.ParseError {
				return nil, nil, fmt.Errorf("package %s: %s", pkg.PkgPath, pkgErr.Msg)
			}
			// Soft errors (type-checking in transitive deps, etc.) — warn and continue.
			fmt.Fprintf(os.Stderr, "warning: %s: %s\n", pkg.PkgPath, pkgErr.Msg)
		}
	}

	return prog, pkgs, nil
}
