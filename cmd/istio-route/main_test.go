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
