package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/ninoamine/istio-route/internal/behavior"
	"github.com/ninoamine/istio-route/internal/model"
	"github.com/ninoamine/istio-route/internal/resolve"
	"github.com/ninoamine/istio-route/internal/source"
	"github.com/spf13/cobra"
)

var (
	ErrTestFailure = errors.New("one or more behavior scenarios failed")
	ErrTestInvalid = errors.New("invalid behavior test input")
)

func newTestCommand(stdout, stderr io.Writer) *cobra.Command {
	var (
		files []string
		junit string
	)
	command := &cobra.Command{
		Use:           "test [-f <file|->]... [--junit <file>] <feature file|directory>...",
		Short:         "Test rendered routing behavior with Gherkin scenarios",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("%w: at least one feature file or directory is required", ErrTestInvalid)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(files) == 0 {
				return fmt.Errorf("%w: at least one rendered manifest file is required with -f", ErrTestInvalid)
			}
			snapshot, err := (&source.FilesSource{
				Files: files,
				Stdin: cmd.InOrStdin(),
			}).Snapshot(cmd.Context())
			if err != nil {
				return fmt.Errorf("%w: read rendered manifests: %w", ErrTestInvalid, err)
			}
			if len(snapshot.Resources) == 0 {
				return fmt.Errorf("%w: rendered manifest input contains no resources", ErrTestInvalid)
			}

			endpointReader := resolve.NewSnapshotEndpointReader(snapshot)
			trace := func(ctx context.Context, hostname string) (analysisResult model.Analysis, traceErr error) {
				analysisResult, traceErr = analyzeSnapshot(ctx, hostname, snapshot, endpointReader)
				if errors.Is(traceErr, ErrHostnameNotFound) {
					return analysisResult, behavior.ErrRouteNotFound
				}
				return analysisResult, traceErr
			}
			result, err := behavior.Run(cmd.Context(), behavior.RunOptions{
				Paths:       args,
				Trace:       trace,
				Output:      cmd.OutOrStdout(),
				Diagnostics: cmd.ErrOrStderr(),
				JUnitPath:   junit,
			})
			if err != nil {
				return fmt.Errorf("%w: %w", ErrTestInvalid, err)
			}
			if result.Invalid {
				return ErrTestInvalid
			}
			if result.Failed {
				return ErrTestFailure
			}
			return nil
		},
	}
	command.SetOut(stdout)
	command.SetErr(stderr)
	command.Flags().StringArrayVarP(&files, "file", "f", nil, "Read rendered YAML manifests without connecting to a cluster")
	command.Flags().StringVar(&junit, "junit", "", "Write a JUnit XML report to this file")
	return command
}
