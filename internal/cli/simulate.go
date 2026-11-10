package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/ninoamine/istio-route/internal/render"
	"github.com/ninoamine/istio-route/internal/simulate"
)

var (
	ErrRouteNotFound   = errors.New("HTTPRoute not found")
	ErrRouteIneligible = fmt.Errorf("%w: HTTPRoute is not attached to this waypoint", ErrUsage)
)

type SimulateRequest struct {
	WaypointName string
	Route        *simulate.RouteRef
	Flags        Flags
}

// selectionError explains a route-selection failure on stderr while still matching its exit-code sentinel.
type selectionError struct {
	sentinel error
	reason   string
}

func (e *selectionError) Error() string {
	if e.reason == "" {
		return e.sentinel.Error()
	}
	return e.reason
}

func (e *selectionError) Unwrap() error { return e.sentinel }

type SimulateService interface {
	Simulate(context.Context, SimulateRequest) (simulate.Result, simulate.Status, error)
}

func newSimulateCommand(service SimulateService, stdout, stderr io.Writer) *cobra.Command {
	flags := Flags{Output: render.FormatText}
	var routeValue string
	var route *simulate.RouteRef
	command := &cobra.Command{
		Use:           "simulate [flags] <waypoint-name>",
		Short:         "Explain the impact of removing an HTTPRoute from an ambient waypoint",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("%w: expected exactly one waypoint name, got %d", ErrUsage, len(args))
			}
			return nil
		},
		RunE: func(command *cobra.Command, args []string) error {
			if service == nil {
				return fmt.Errorf("simulate service is not configured")
			}
			flags.Stdin = command.InOrStdin()
			result, status, err := service.Simulate(command.Context(), SimulateRequest{
				WaypointName: args[0], Route: route, Flags: flags,
			})
			if err != nil {
				if errors.Is(err, simulate.ErrNotWaypoint) {
					return fmt.Errorf("%w: %v", ErrUsage, err)
				}
				return err
			}
			switch status {
			case simulate.StatusWaypointNotFound:
				return fmt.Errorf("%w: %s", ErrGatewayNotFound, args[0])
			case simulate.StatusRouteNotFound, simulate.StatusRouteIneligible:
				// The listing is the useful output for these outcomes; the exit code reflects the selection failure.
				sentinel := ErrRouteNotFound
				if status == simulate.StatusRouteIneligible {
					sentinel = ErrRouteIneligible
				}
				if err := writeSimulation(command.OutOrStdout(), result, flags); err != nil {
					return fmt.Errorf("%w; %v", sentinel, err)
				}
				return &selectionError{sentinel: sentinel, reason: result.SelectionReason}
			}
			return writeSimulation(command.OutOrStdout(), result, flags)
		},
	}
	command.SetOut(stdout)
	command.SetErr(stderr)
	command.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return fmt.Errorf("%w: %v", ErrUsage, err)
	})
	command.Flags().StringVar(&flags.Kubeconfig, "kubeconfig", "", "Path to kubeconfig")
	command.Flags().StringVar(&flags.Context, "context", "", "Kubernetes context to use")
	command.Flags().StringVarP(&flags.Namespace, "namespace", "n", "", "Namespace containing the waypoint")
	command.Flags().StringArrayVarP(&flags.Files, "file", "f", nil, "Read YAML manifests without connecting to a cluster")
	command.Flags().StringVar(&routeValue, "route", "", "HTTPRoute to simulate removing, as <namespace>/<name>")
	command.Flags().StringVar(&flags.Output, "output", flags.Output, "Output format: text, json, yaml, or mermaid")
	command.Flags().BoolVar(&flags.NoColor, "no-color", false, "Disable colored text output")
	command.PreRunE = func(_ *cobra.Command, _ []string) error {
		switch flags.Output {
		case render.FormatText, render.FormatJSON, render.FormatYAML, render.FormatMermaid:
		default:
			return fmt.Errorf("%w: unsupported output format %q (choose text, json, yaml, or mermaid)", ErrUsage, flags.Output)
		}
		parsed, err := parseRouteFlag(routeValue)
		if err != nil {
			return err
		}
		route = parsed
		return nil
	}
	return command
}

// parseRouteFlag reads --route as <namespace>/<name>. An empty value means no route was selected.
func parseRouteFlag(value string) (*simulate.RouteRef, error) {
	if value == "" {
		return nil, nil
	}
	namespace, name, found := strings.Cut(value, "/")
	if !found {
		return nil, fmt.Errorf("%w: --route must be <namespace>/<name>, got %q", ErrUsage, value)
	}
	if errs := validation.IsDNS1123Label(namespace); len(errs) > 0 {
		return nil, fmt.Errorf("%w: invalid --route namespace %q: %s", ErrUsage, namespace, strings.Join(errs, "; "))
	}
	if errs := validation.IsDNS1123Subdomain(name); len(errs) > 0 {
		return nil, fmt.Errorf("%w: invalid --route name %q: %s", ErrUsage, name, strings.Join(errs, "; "))
	}
	return &simulate.RouteRef{Namespace: namespace, Name: name}, nil
}

func writeSimulation(out io.Writer, result simulate.Result, flags Flags) error {
	report, err := render.RenderSimulation(result, flags.Output, render.Options{
		Color: colorEnabled(out, flags.NoColor),
	})
	if err != nil {
		return fmt.Errorf("render simulation: %w", err)
	}
	if _, err := fmt.Fprint(out, report); err != nil {
		return fmt.Errorf("write simulation: %w", err)
	}
	return nil
}
