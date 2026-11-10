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
		return reportError(stderr, err, exitCode(err))
	}
	return 0
}

// exitCode maps a command error to the process exit status: 2 usage, 3 target not found, 4 audit findings, 1 otherwise.
func exitCode(err error) int {
	switch {
	case errors.Is(err, cli.ErrTestInvalid):
		return 2
	case errors.Is(err, cli.ErrTestFailure):
		return 1
	case errors.Is(err, cli.ErrUsage):
		return 2
	case errors.Is(err, cli.ErrHostnameNotFound),
		errors.Is(err, cli.ErrGatewayNotFound),
		errors.Is(err, cli.ErrRouteNotFound):
		return 3
	case errors.Is(err, cli.ErrAuditFindings):
		return 4
	default:
		return 1
	}
}

func reportError(stderr io.Writer, err error, code int) int {
	if _, writeErr := fmt.Fprintln(stderr, err); writeErr != nil {
		return 1
	}
	return code
}
