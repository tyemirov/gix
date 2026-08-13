package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/tyemirov/gix/cmd/cli"
)

const (
	exitErrorTemplateConstant = "%v\n"
	interruptExitCodeConstant = 130
)

// main executes the gix command-line application.
func main() {
	if executionError := cli.Execute(); executionError != nil {
		if errors.Is(executionError, context.Canceled) {
			os.Exit(interruptExitCodeConstant)
		}
		fmt.Fprintf(os.Stderr, exitErrorTemplateConstant, executionError)
		os.Exit(1)
	}
}
