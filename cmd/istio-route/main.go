package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ninoamine/istio-route/internal/cli"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	return runWithService(args, stdout, stderr, cli.NewDefaultAnalysisService())
}

func runWithService(args []string, stdout, stderr io.Writer, service cli.AnalysisService) int {
	command := cli.NewRootCommand(service, stdout, stderr)
	command.SetArgs(args)
	if err := command.Execute(); err != nil {
		switch {
		case errors.Is(err, cli.ErrUsage):
			return reportError(stderr, err, 2)
		case errors.Is(err, cli.ErrHostnameNotFound):
			return reportError(stderr, err, 3)
		default:
			return reportError(stderr, err, 1)
		}
	}
	return 0
}

func reportError(stderr io.Writer, err error, code int) int {
	if _, writeErr := fmt.Fprintln(stderr, err); writeErr != nil {
		return 1
	}
	return code
}
