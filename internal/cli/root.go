package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/ninoamine/istio-route/internal/model"
	"github.com/ninoamine/istio-route/internal/render"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	ErrUsage               = errors.New("invalid command usage")
	ErrHostnameNotFound    = errors.New("hostname not found")
	ErrAnalysisUnavailable = errors.New("analysis service is not configured")
)

type Flags struct {
	Kubeconfig string
	Context    string
	Namespace  string
	Files      []string
	Output     string
	NoColor    bool
}

type Request struct {
	Hostname string
	Flags    Flags
}

type AnalysisService interface {
	Analyze(context.Context, Request) (model.Analysis, error)
}

func colorEnabled(output io.Writer, noColor bool) bool {
	if noColor {
		return false
	}
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled {
		return false
	}
	file, ok := output.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func NewRootCommand(service AnalysisService, stdout, stderr io.Writer) *cobra.Command {
	flags := Flags{Output: "text"}
	cmd := &cobra.Command{
		Use:           "istio-route [flags] <hostname>",
		Short:         "Explain how traffic for a hostname is routed",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("%w: expected exactly one hostname, got %d", ErrUsage, len(args))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if service == nil {
				return ErrAnalysisUnavailable
			}
			analysis, err := service.Analyze(cmd.Context(), Request{
				Hostname: args[0],
				Flags:    flags,
			})
			for _, family := range analysis.Source.Families {
				if !family.Available {
					slog.Info("skipping unavailable resource family", "family", family.Family, "reason", family.Reason)
				}
			}
			if err != nil {
				return err
			}
			report, err := render.Render(analysis, flags.Output, render.Options{
				Color: colorEnabled(cmd.OutOrStdout(), flags.NoColor),
			})
			if err != nil {
				return fmt.Errorf("render %s report: %w", flags.Output, err)
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), report)
			if err != nil {
				return fmt.Errorf("write analysis report: %w", err)
			}
			return nil
		},
	}

	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return fmt.Errorf("%w: %v", ErrUsage, err)
	})
	cmd.Flags().StringVar(&flags.Kubeconfig, "kubeconfig", "", "Path to kubeconfig")
	cmd.Flags().StringVar(&flags.Context, "context", "", "Kubernetes context to use")
	cmd.Flags().StringVarP(&flags.Namespace, "namespace", "n", "", "Limit analysis to a namespace")
	cmd.Flags().StringArrayVarP(&flags.Files, "file", "f", nil, "Read YAML manifests without connecting to a cluster")
	cmd.Flags().StringVar(&flags.Output, "output", flags.Output, "Output format: text, json, yaml, or mermaid")
	cmd.Flags().BoolVar(&flags.NoColor, "no-color", false, "Disable colored text output")
	cmd.Version = "dev"
	cmd.InitDefaultVersionFlag()

	cmd.PreRunE = func(_ *cobra.Command, _ []string) error {
		switch flags.Output {
		case "text", "json", "yaml", "mermaid":
		default:
			return fmt.Errorf("%w: unsupported output format %q (choose text, json, yaml, or mermaid)", ErrUsage, flags.Output)
		}
		return nil
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(stderr, nil)))
	return cmd
}
