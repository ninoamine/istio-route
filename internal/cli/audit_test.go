package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninoamine/istio-route/internal/audit"
	"github.com/ninoamine/istio-route/internal/model"
)

type fakeGatewayAuditService struct {
	result  audit.Result
	outcome audit.Outcome
	err     error
	request GatewayAuditRequest
}

func (s *fakeGatewayAuditService) AnalyzeGateway(
	_ context.Context,
	request GatewayAuditRequest,
) (audit.Result, audit.Outcome, error) {
	s.request = request
	return s.result, s.outcome, s.err
}

func TestAuditCommandContract(t *testing.T) {
	sourceErr := errors.New("source unavailable")
	tests := []struct {
		name        string
		args        []string
		service     *fakeGatewayAuditService
		wantErr     error
		wantOutput  string
		wantRequest string
	}{
		{
			name: "renders success",
			args: []string{"-n", "infra", "--output", "json", "edge"},
			service: &fakeGatewayAuditService{
				result: audit.Result{Gateway: audit.Gateway{
					Identity: model.ResourceIdentity{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway", Namespace: "infra", Name: "edge"},
					Class: model.GatewayClassRef{
						Identity:   model.ResourceIdentity{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "GatewayClass", Name: "istio"},
						Controller: "istio.io/gateway-controller",
					},
				}},
				outcome: audit.OutcomeTargetFound,
			},
			wantOutput: `"name": "edge"`, wantRequest: "edge",
		},
		{
			name: "findings return findings sentinel after rendering",
			args: []string{"edge"},
			service: &fakeGatewayAuditService{
				result: audit.Result{
					Gateway: audit.Gateway{
						Identity: model.ResourceIdentity{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway", Namespace: "infra", Name: "edge"},
						Class: model.GatewayClassRef{
							Identity:   model.ResourceIdentity{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "GatewayClass", Name: "istio"},
							Controller: "istio.io/gateway-controller",
						},
					},
					Findings: []model.Finding{{
						Code: "rejected-route", Severity: model.FindingSeverityError,
						Message: "route is rejected", Category: string(audit.CategoryOrphaned),
					}},
				},
				outcome: audit.OutcomeTargetFound,
			},
			wantErr: ErrAuditFindings, wantOutput: "rejected-route", wantRequest: "edge",
		},
		{
			name:        "not found returns not found sentinel",
			args:        []string{"edge"},
			service:     &fakeGatewayAuditService{outcome: audit.OutcomeTargetNotFound},
			wantErr:     ErrGatewayNotFound,
			wantRequest: "edge",
		},
		{
			name:        "source error remains an error",
			args:        []string{"edge"},
			service:     &fakeGatewayAuditService{err: sourceErr},
			wantErr:     sourceErr,
			wantRequest: "edge",
		},
		{
			name:    "wrong argument count",
			args:    []string{"edge", "other"},
			service: &fakeGatewayAuditService{},
			wantErr: ErrUsage,
		},
		{
			name:    "invalid output",
			args:    []string{"--output", "xml", "edge"},
			service: &fakeGatewayAuditService{},
			wantErr: ErrUsage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
			command := newAuditCommand(tt.service, stdout, stderr)
			command.SetArgs(tt.args)
			err := command.Execute()
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("Execute() error = %v, want nil", err)
				}
			} else if err == nil || !errors.Is(err, tt.wantErr) {
				t.Fatalf("Execute() error = %v, want %v", err, tt.wantErr)
			}
			if !strings.Contains(stdout.String(), tt.wantOutput) {
				t.Errorf("stdout = %q, want substring %q", stdout.String(), tt.wantOutput)
			}
			if tt.wantRequest != "" && tt.service.request.GatewayName != tt.wantRequest {
				t.Errorf("service request gateway = %q, want %q", tt.service.request.GatewayName, tt.wantRequest)
			}
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

func TestDefaultGatewayAuditServiceUsesOfflineFiles(t *testing.T) {
	service := NewDefaultGatewayAuditService()
	result, outcome, err := service.AnalyzeGateway(context.Background(), GatewayAuditRequest{
		GatewayName: "edge",
		Flags: Flags{
			Files:      []string{filepath.Join("..", "..", "testdata", "manifests", "gateway-audit-healthy.yaml")},
			Kubeconfig: filepath.Join(t.TempDir(), "must-not-be-read"),
		},
	})
	if err != nil {
		t.Fatalf("AnalyzeGateway() error = %v", err)
	}
	if outcome != audit.OutcomeTargetFound || result.Gateway.Identity.Name != "edge" {
		t.Fatalf("AnalyzeGateway() = %#v, %q, want offline edge Gateway", result, outcome)
	}
	if result.Source.Mode != model.SourceModeFiles {
		t.Errorf("source mode = %q, want files", result.Source.Mode)
	}
}

func TestDefaultGatewayAuditServiceRequiresNamespaceWithoutKubeconfigDefault(t *testing.T) {
	config := filepath.Join(t.TempDir(), "kubeconfig")
	content := `apiVersion: v1
kind: Config
clusters:
  - name: local
    cluster:
      server: https://127.0.0.1:6443
contexts:
  - name: local
    context:
      cluster: local
      user: local
current-context: local
users:
  - name: local
    user: {}
`
	if err := os.WriteFile(config, []byte(content), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	_, _, err := NewDefaultGatewayAuditService().AnalyzeGateway(context.Background(), GatewayAuditRequest{
		GatewayName: "edge", Flags: Flags{Kubeconfig: config},
	})
	if !errors.Is(err, ErrUsage) || !strings.Contains(err.Error(), "--namespace") {
		t.Fatalf("AnalyzeGateway() error = %v, want namespace usage error", err)
	}
}
