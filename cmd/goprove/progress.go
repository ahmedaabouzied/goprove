package main

import (
	"fmt"
	"os"
	"time"
)

func NewProgress(noColor bool) *Progress {
	return &Progress{start: time.Now(), noColor: noColor}
}

// Progress writes phase and per-package progress to stderr.
// nil-safe: all methods are no-ops on a nil receiver.
type Progress struct {
	start   time.Time
	noColor bool
}

// Done prints the final summary line.
func (p *Progress) Done() {
	if p == nil {
		return
	}
	if p.noColor {
		fmt.Fprintf(os.Stderr, "Analysis complete (%s)\n", time.Since(p.start).Round(time.Millisecond))
	} else {
		fmt.Fprintf(os.Stderr, "\r\033[2K\033[37mAnalysis complete (%s)\033[0m\n", time.Since(p.start).Round(time.Millisecond))
	}
}

// Phase prints a phase start message. Returns a function to call when the phase is done.
func (p *Progress) Phase(name string) func() {
	if p == nil {
		return func() {}
	}
	t := time.Now()
	if p.noColor {
		fmt.Fprintf(os.Stderr, "%s...\n", name)
	} else {
		fmt.Fprintf(os.Stderr, "\r\033[2K\033[37m%s...\033[0m", name)
	}
	return func() {
		if p.noColor {
			fmt.Fprintf(os.Stderr, "%s... done (%s)\n", name, time.Since(t).Round(time.Millisecond))
		} else {
			fmt.Fprintf(os.Stderr, "\r\033[2K\033[37m%s... done (%s)\033[0m\n", name, time.Since(t).Round(time.Millisecond))
		}
	}
}

// Pkg prints per-package progress, overwriting the current line.
func (p *Progress) Pkg(current, total int, pkgPath string) {
	if p == nil {
		return
	}
	if p.noColor {
		fmt.Fprintf(os.Stderr, "Analyzing [%d/%d] %s...\n", current, total, pkgPath)
	} else {
		fmt.Fprintf(os.Stderr, "\r\033[2K\033[37mAnalyzing [%d/%d] %s...\033[0m", current, total, pkgPath)
	}
}
