package main

import (
	"fmt"
	"os"

	"github.com/tyemirov/gix/cmd/cli"
)

const (
	exitErrorTemplateConstant = "%v\n"
)

// main executes the gix command-line application.
func main() {
	if executionError := cli.Execute(); executionError != nil {
		fmt.Fprintf(os.Stderr, exitErrorTemplateConstant, executionError)
		os.Exit(1)
	}
}
