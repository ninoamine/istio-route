package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/ninoamine/istio-route/internal/audit"
	"github.com/ninoamine/istio-route/internal/render"
	"github.com/ninoamine/istio-route/internal/source"
	"github.com/spf13/cobra"
)

var (
	ErrGatewayNotFound = errors.New("gateway not found")
	ErrAuditFindings   = errors.New("audit findings present")
)

type GatewayAuditRequest struct {
	GatewayName string
	Flags       Flags
}

type GatewayAuditService interface {
	AnalyzeGateway(context.Context, GatewayAuditRequest) (audit.Result, audit.Outcome, error)
}

type defaultGatewayAuditService struct{}

func NewDefaultGatewayAuditService() GatewayAuditService {
	return defaultGatewayAuditService{}
}

func (defaultGatewayAuditService) AnalyzeGateway(
	ctx context.Context,
	request GatewayAuditRequest,
) (audit.Result, audit.Outcome, error) {
	var snapshot source.Snapshot
	if len(request.Flags.Files) > 0 {
		result, outcome, err := audit.AnalyzeFiles(
			ctx, request.GatewayName, request.Flags.Namespace,
			request.Flags.Files, request.Flags.Stdin,
		)
		if err != nil {
			return normalizeAuditError(request.GatewayName, err)
		}
		return result, outcome, nil
	} else {
		cluster, err := source.NewClusterSource(
			ctx, request.Flags.Kubeconfig, request.Flags.Context, request.Flags.Namespace,
		)
		if err != nil {
			return audit.Result{}, "", fmt.Errorf("initialize Gateway audit source: %w", err)
		}
		if cluster.Namespace == "" {
			return audit.Result{}, "", fmt.Errorf("%w: --namespace is required when the kubeconfig has no default namespace", ErrUsage)
		}
		snapshot, err = cluster.GatewaySnapshot(ctx, request.GatewayName, cluster.Namespace)
		if err != nil {
			if errors.Is(err, source.ErrGatewayNotFound) {
				return audit.Result{}, audit.OutcomeTargetNotFound, nil
			}
			return audit.Result{}, "", fmt.Errorf("read Gateway audit snapshot: %w", err)
		}
	}
	result, outcome, err := audit.Analyze(ctx, audit.Request{
		GatewayName: request.GatewayName, Namespace: request.Flags.Namespace,
		Source: snapshot.Metadata,
	}, snapshot)
	if err != nil {
		return normalizeAuditError(request.GatewayName, err)
	}
	return result, outcome, nil
}

func normalizeAuditError(gatewayName string, err error) (audit.Result, audit.Outcome, error) {
	if errors.Is(err, audit.ErrAmbiguousGateway) {
		return audit.Result{}, "", fmt.Errorf("%w: %w", ErrUsage, err)
	}
	return audit.Result{}, "", fmt.Errorf("analyze Gateway %q: %w", gatewayName, err)
}

func newAuditCommand(service GatewayAuditService, stdout, stderr io.Writer) *cobra.Command {
	flags := Flags{Output: render.FormatText}
	command := &cobra.Command{
		Use:           "analyze [flags] <gateway-name>",
		Short:         "Audit one Gateway API Gateway or ambient waypoint",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("%w: expected exactly one Gateway name, got %d", ErrUsage, len(args))
			}
			return nil
		},
		RunE: func(command *cobra.Command, args []string) error {
			if service == nil {
				return fmt.Errorf("gateway audit service is not configured")
			}
			flags.Stdin = command.InOrStdin()
			result, outcome, err := service.AnalyzeGateway(command.Context(), GatewayAuditRequest{
				GatewayName: args[0], Flags: flags,
			})
			if err != nil {
				return err
			}
			if outcome == audit.OutcomeTargetNotFound {
				return fmt.Errorf("%w: %s", ErrGatewayNotFound, args[0])
			}
			report, err := render.RenderAudit(result, flags.Output, render.Options{
				Color: colorEnabled(command.OutOrStdout(), flags.NoColor),
			})
			if err != nil {
				return fmt.Errorf("render Gateway audit report: %w", err)
			}
			if _, err := fmt.Fprint(command.OutOrStdout(), report); err != nil {
				return fmt.Errorf("write Gateway audit report: %w", err)
			}
			if len(result.Findings) > 0 {
				return ErrAuditFindings
			}
			return nil
		},
	}
	command.SetOut(stdout)
	command.SetErr(stderr)
	command.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return fmt.Errorf("%w: %v", ErrUsage, err)
	})
	command.Flags().StringVar(&flags.Kubeconfig, "kubeconfig", "", "Path to kubeconfig")
	command.Flags().StringVar(&flags.Context, "context", "", "Kubernetes context to use")
	command.Flags().StringVarP(&flags.Namespace, "namespace", "n", "", "Namespace containing the Gateway")
	command.Flags().StringArrayVarP(&flags.Files, "file", "f", nil, "Read YAML manifests without connecting to a cluster")
	command.Flags().StringVar(&flags.Output, "output", flags.Output, "Output format: text, json, yaml, or mermaid")
	command.Flags().BoolVar(&flags.NoColor, "no-color", false, "Disable colored text output")
	command.PreRunE = func(_ *cobra.Command, _ []string) error {
		switch flags.Output {
		case render.FormatText, render.FormatJSON, render.FormatYAML, render.FormatMermaid:
			return nil
		default:
			return fmt.Errorf("%w: unsupported output format %q (choose text, json, yaml, or mermaid)", ErrUsage, flags.Output)
		}
	}
	return command
}
