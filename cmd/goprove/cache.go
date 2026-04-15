package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"github.com/ahmedaabouzied/goprove/pkg/version"
)

func runCacheStdlib(args []string) {
	fs := flag.NewFlagSet("cache stdlib", flag.ExitOnError)
	outputPath := fs.String("o", "", "output path for cache file")
	if err := fs.Parse(args); err != nil {
		printErrAndOsExit(err.Error())
	}

	outPath := *outputPath
	if outPath == "" {
		p, err := analysis.DefaultCachePath(version.Version)
		if err != nil {
			printErrAndOsExit(err.Error())
		}
		outPath = p
	}

	progress := NewProgress(*noColorFlag)

	fmt.Fprintf(os.Stderr, "Generating stdlib cache (Go %s, goprove %s)...\n",
		runtime.Version(), version.Version)

	cache, err := analysis.GenerateStdlibCache(
		version.Version,
		func(current, total int, pkgPath string) {
			progress.Pkg(current, total, pkgPath)
		},
	)
	if err != nil {
		printErrAndOsExit(err.Error())
	}
	progress.Done()

	if err := cache.Save(outPath); err != nil {
		printErrAndOsExit(err.Error())
	}

	fmt.Fprintf(os.Stderr, "Wrote %d function summaries to %s\n", cache.Len(), outPath)
}
