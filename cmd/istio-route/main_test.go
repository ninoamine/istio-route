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
			name:       "named manifests are not accepted in single-set phase",
			args:       []string{"test", "-f", "../../testdata/manifests/behavior.yaml", "--manifests", "prod=rendered.yaml", "../../testdata/features/routing-passes.feature"},
			wantCode:   2,
			wantStderr: cli.ErrUsage.Error(),
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
