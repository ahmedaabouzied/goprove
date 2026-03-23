package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"

	"github.com/ahmedaabouzied/goprove/consts"
)

func main() {
	flag.Usage = printHelp
	flag.Parse()
	args := flag.Args()

	validateArgs(args)

	targetPackage := args[0]

	p, err := NewProver(targetPackage)
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
	fmt.Fprintln(w, "\t Example: goprove fmt")
	fmt.Fprintln(w, "\t Flags:")
	fmt.Fprintln(w, "\t \t -h : Prints help message.")
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
