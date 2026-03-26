package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"

	"github.com/ahmedaabouzied/goprove/consts"
	"github.com/ahmedaabouzied/goprove/pkg/updater"
	"github.com/ahmedaabouzied/goprove/pkg/version"
)

var interactive = flag.Bool("i", false, "show progress during analysis")

func main() {
	flag.Usage = printHelp
	flag.Parse()
	args := flag.Args()
	if len(args) > 0 && args[0] == "version" {
		// version command
		fmt.Println(version.Info())
		return
	}

	validateArgs(args)

	targetPackage := args[0]

	var progress *Progress
	if *interactive {
		progress = NewProgress()
	}

	// Check for updates
	if latest := updater.CheckForUpdates(); latest != "" {
		fmt.Fprintf(os.Stderr, "A new version of goprove is available: %s (current: %s)\n", latest, version.Version)
		fmt.Fprintln(os.Stderr, "Upgrade with: go install github.com/ahmedaabouzied/goprove/cmd/goprove@latest")
	}

	p, err := NewProver(targetPackage, progress)
	if err != nil {
		printErrAndOsExit(err.Error())
	}
	findings, err := p.Prove()
	if err != nil {
		printErrAndOsExit(err.Error())
	}
	if findings > 0 {
		os.Exit(1)
	}
}

func printHelp() {
	w := bufio.NewWriter(os.Stdout)
	fmt.Fprintln(w, "goprove: A code prover for Golang.")
	fmt.Fprintln(w, "\t Usage: goprove <target_package>")
	fmt.Fprintln(w, "\t Commands:")
	fmt.Fprintln(w, "\t \t version : Prints version information.")
	fmt.Fprintln(w, "\t Example: goprove fmt")
	fmt.Fprintln(w, "\t Flags:")
	fmt.Fprintln(w, "\t \t -i : Show progress during analysis.")
	fmt.Fprintln(w, "\t \t -h : Prints help message.")
	fmt.Fprintf(w, "Version: %s \n", version.Info())
	if err := w.Flush(); err != nil {
		printErrAndOsExit(err.Error())
	}
}

func validateArgs(args []string) {
	if len(args) < 1 {
		printErrAndOsExit("missing target package required argument")
	}
}

func printErrAndOsExit(msg string) {
	if len(msg) > consts.OneKB { // Checking input bounds for safety.
		msg = msg[:consts.OneKB] // Truncate the error message.
	}
	_, _ = fmt.Fprintln(os.Stderr, msg)
	// We don't have to check errors here. If stderr is broken and we can't write to it,
	// the user has more things to worry about.
	os.Exit(1)
}
