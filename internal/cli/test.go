package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/ninoamine/istio-route/internal/audit"
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
		files        []string
		manifestSets []string
		junit        string
	)
	command := &cobra.Command{
		Use: "test [-f <file|->]... [--manifests <name>=<file>]... " +
			"[--junit <file>] <feature file|directory>...",
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
			sets, err := behavior.ParseManifestSets(files, manifestSets)
			if err != nil {
				return fmt.Errorf("%w: %w", ErrTestInvalid, err)
			}
			environments := make([]behavior.Environment, 0, len(sets))
			for _, set := range sets {
				snapshot, err := (&source.FilesSource{
					Files: set.Files,
					Stdin: cmd.InOrStdin(),
				}).Snapshot(cmd.Context())
				if err != nil {
					return fmt.Errorf("%w: read rendered manifests for %q: %w", ErrTestInvalid, set.Name, err)
				}
				if len(snapshot.Resources) == 0 {
					return fmt.Errorf("%w: rendered manifest input for %q contains no resources", ErrTestInvalid, set.Name)
				}
				endpointReader := resolve.NewSnapshotEndpointReader(snapshot)
				trace := func(ctx context.Context, hostname string) (model.Analysis, error) {
					analysis, traceErr := analyzeSnapshot(ctx, hostname, snapshot, endpointReader)
					if errors.Is(traceErr, ErrHostnameNotFound) {
						return analysis, behavior.ErrRouteNotFound
					}
					return analysis, traceErr
				}
				auditGateway := func(ctx context.Context, namespace, gatewayName string) (audit.Result, audit.Outcome, error) {
					return audit.Analyze(ctx, audit.Request{
						GatewayName: gatewayName,
						Namespace:   namespace,
						Source:      snapshot.Metadata,
					}, snapshot)
				}
				environments = append(environments, behavior.Environment{
					Name: set.Name, Trace: trace, Audit: auditGateway,
				})
			}
			result, err := behavior.Run(cmd.Context(), behavior.RunOptions{
				Paths:        args,
				Environments: environments,
				Output:       cmd.OutOrStdout(),
				Diagnostics:  cmd.ErrOrStderr(),
				JUnitPath:    junit,
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
	command.Flags().StringArrayVar(&manifestSets, "manifests", nil, "Read rendered YAML as a named environment (name=file)")
	command.Flags().StringVar(&junit, "junit", "", "Write a JUnit XML report to this file")
	return command
}
