package main

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/ninoamine/istio-route/internal/cli"
	"github.com/ninoamine/istio-route/internal/model"
)

type failingAnalysisService struct {
	err error
}

func (s failingAnalysisService) Analyze(context.Context, cli.Request) (model.Analysis, error) {
	return model.Analysis{}, s.err
}

func TestRunExitCodes(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		service    cli.AnalysisService
		wantCode   int
		wantStderr string
		wantStdout string
	}{
		{
			name:     "help succeeds",
			args:     []string{"--help"},
			wantCode: 0,
		},
		{
			name:       "usage error",
			args:       nil,
			wantCode:   2,
			wantStderr: "invalid command usage",
		},
		{
			name:       "runtime error",
			args:       []string{"shop.example.com"},
			wantCode:   1,
			wantStderr: "analysis service is not configured",
		},
		{
			name:       "hostname not found",
			args:       []string{"absent.example.com"},
			service:    failingAnalysisService{err: cli.ErrHostnameNotFound},
			wantCode:   3,
			wantStderr: "hostname not found",
		},
		{
			name:       "simulate without waypoint is usage error",
			args:       []string{"simulate"},
			wantCode:   2,
			wantStderr: "invalid command usage",
		},
		{
			name:       "simulate without files and unreadable kubeconfig is runtime error",
			args:       []string{"simulate", "--kubeconfig", "/nonexistent/istio-route-kubeconfig", "waypoint"},
			wantCode:   1,
			wantStderr: "initialize simulation source",
		},
		{
			name:       "simulate listing succeeds through the subcommand",
			args:       []string{"simulate", "-f", "../../testdata/manifests/simulate-impact.yaml", "-n", "mesh", "waypoint"},
			wantCode:   0,
			wantStdout: "Waypoint: mesh/waypoint",
		},
		{
			name:       "simulate impact succeeds for an attached route",
			args:       []string{"simulate", "-f", "../../testdata/manifests/simulate-impact.yaml", "-n", "mesh", "--route", "shop/primary", "waypoint"},
			wantCode:   0,
			wantStdout: "UNROUTED",
		},
		{
			name:       "simulate missing route is not-found status",
			args:       []string{"simulate", "-f", "../../testdata/manifests/simulate-multi-waypoint.yaml", "-n", "mesh", "--route", "shop/absent", "wp-a"},
			wantCode:   3,
			wantStdout: "Attached HTTPRoutes",
			wantStderr: "HTTPRoute not found",
		},
		{
			name:       "simulate ineligible route is usage status",
			args:       []string{"simulate", "-f", "../../testdata/manifests/simulate-multi-waypoint.yaml", "-n", "mesh", "--route", "shop/only-b", "wp-a"},
			wantCode:   2,
			wantStdout: "Attached HTTPRoutes",
			wantStderr: "not attached to this waypoint",
		},
		{
			name:       "simulate unknown waypoint is not-found status",
			args:       []string{"simulate", "-f", "../../testdata/manifests/simulate-multi-waypoint.yaml", "-n", "mesh", "absent"},
			wantCode:   3,
			wantStderr: "gateway not found: absent",
		},
		{
			name:       "simulate ambiguous waypoint is usage status",
			args:       []string{"simulate", "-f", "../../testdata/manifests/simulate-multi-waypoint.yaml", "wp-a"},
			wantCode:   2,
			wantStderr: "ambiguous Gateway",
		},
		{
			name:       "simulate ingress gateway is usage status",
			args:       []string{"simulate", "-f", "../../testdata/manifests/simulate-ingress.yaml", "-n", "app", "ingress"},
			wantCode:   2,
			wantStderr: "not an Istio ambient waypoint",
		},
		{
			name:       "simulate namespace-less manifest is runtime error",
			args:       []string{"simulate", "-f", "../../testdata/manifests/simulate-missing-namespace.yaml", "-n", "app", "waypoint"},
			wantCode:   1,
			wantStderr: "missing namespace",
		},
		{
			name:       "simulate Istio-only manifest is runtime error",
			args:       []string{"simulate", "-f", "../../testdata/manifests/istio.yaml", "-n", "mesh", "waypoint"},
			wantCode:   1,
			wantStderr: "no Gateway API resources were supplied",
		},
		{
			name:       "simulate unsupported output is usage error",
			args:       []string{"simulate", "-f", "../../testdata/manifests/simulate-impact.yaml", "-n", "mesh", "--output", "xml", "waypoint"},
			wantCode:   2,
			wantStderr: "unsupported output format",
		},
		{
			name:       "simulate malformed route is usage error",
			args:       []string{"simulate", "-f", "../../testdata/manifests/simulate-impact.yaml", "-n", "mesh", "--route", "shop", "waypoint"},
			wantCode:   2,
			wantStderr: "--route must be <namespace>/<name>",
		},
		{
			name:       "behavior tests pass",
			args:       []string{"test", "-f", "../../testdata/manifests/behavior.yaml", "../../testdata/features/routing-passes.feature"},
			wantCode:   0,
			wantStdout: "Shop traffic routing",
		},
		{
			name:       "behavior assertion failure",
			args:       []string{"test", "-f", "../../testdata/manifests/behavior.yaml", "../../testdata/features/routing-fails.feature"},
			wantCode:   1,
			wantStderr: cli.ErrTestFailure.Error(),
			wantStdout: "expected route 1 is",
		},
		{
			name:       "invalid behavior input",
			args:       []string{"test", "../../testdata/features/routing-passes.feature"},
			wantCode:   2,
			wantStderr: cli.ErrTestInvalid.Error(),
		},
		{
			name:       "named manifests check multiple environments",
			args:       []string{"test", "--manifests", "prod=../../testdata/manifests/behavior-prod.yaml", "--manifests", "staging=../../testdata/manifests/behavior-staging.yaml", "../../testdata/features/environments.feature"},
			wantCode:   0,
			wantStdout: "6 scenarios: 6 passed, 0 failed",
		},
		{
			name:     "healthy offline Gateway audit",
			args:     []string{"analyze", "-f", "../../testdata/manifests/gateway-audit-healthy.yaml", "-n", "infra", "edge"},
			wantCode: 0, wantStdout: "Gateway infra/edge",
		},
		{
			name:     "audit findings use findings exit code",
			args:     []string{"analyze", "-f", "../../testdata/manifests/gateway-audit-rejected.yaml", "-n", "infra", "edge"},
			wantCode: 4, wantStdout: "rejected-route", wantStderr: cli.ErrAuditFindings.Error(),
		},
		{
			name:     "unknown offline Gateway uses not-found exit code",
			args:     []string{"analyze", "-f", "../../testdata/manifests/gateway-audit-healthy.yaml", "-n", "infra", "absent"},
			wantCode: 3, wantStderr: cli.ErrGatewayNotFound.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			code := runWithService(tt.args, stdout, stderr, tt.service)
			if code != tt.wantCode {
				t.Fatalf("runWithService() = %d, want %d; stderr: %s", code, tt.wantCode, stderr)
			}
			if tt.wantStderr != "" && !bytes.Contains(stderr.Bytes(), []byte(tt.wantStderr)) {
				t.Errorf("stderr = %q, want substring %q", stderr.String(), tt.wantStderr)
			}
			if tt.wantStdout != "" && !bytes.Contains(stdout.Bytes(), []byte(tt.wantStdout)) {
				t.Errorf("stdout = %q, want substring %q", stdout.String(), tt.wantStdout)
			}
			if tt.wantCode != 0 && tt.wantStdout == "" && stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty when command fails", stdout.String())
			}
		})
	}
}

func TestRunReportsAnalysisFailure(t *testing.T) {
	cause := errors.New("kube API unavailable")
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	code := runWithService(
		[]string{"shop.example.com"},
		stdout,
		stderr,
		failingAnalysisService{err: cause},
	)
	if code != 1 {
		t.Fatalf("runWithService() = %d, want 1", code)
	}
	if !bytes.Contains(stderr.Bytes(), []byte(cause.Error())) {
		t.Fatalf("stderr = %q, want error %q", stderr.String(), cause)
	}
}
