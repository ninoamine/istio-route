package cli

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninoamine/istio-route/internal/model"
)

type fakeAnalysisService struct {
	analysis model.Analysis
	request  Request
	err      error
	calls    int
}

func (s *fakeAnalysisService) Analyze(_ context.Context, request Request) (model.Analysis, error) {
	s.calls++
	s.request = request
	return s.analysis, s.err
}

func TestRootCommandLogsToStderr(t *testing.T) {
	previous := slog.Default()
	defer slog.SetDefault(previous)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	NewRootCommand(nil, stdout, stderr)
	slog.Info("logger check")

	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "logger check") {
		t.Fatalf("stderr = %q, want log message", stderr.String())
	}
}

func TestRootCommand(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		service        AnalysisService
		wantErr        error
		wantCalls      int
		wantHost       string
		wantErrText    string
		wantFormat     string
		wantContext    string
		wantFiles      []string
		wantNamespace  string
		wantKubeconfig string
		wantNoColor    bool
	}{
		{
			name:      "requires exactly one hostname",
			args:      []string{},
			wantErr:   ErrUsage,
			wantCalls: 0,
		},
		{
			name:      "rejects multiple hostnames",
			args:      []string{"shop.example.com", "api.example.com"},
			wantErr:   ErrUsage,
			wantCalls: 0,
		},
		{
			name:        "rejects unsupported output",
			args:        []string{"--output", "xml", "shop.example.com"},
			wantErr:     ErrUsage,
			wantCalls:   0,
			wantErrText: "unsupported output format",
		},
		{
			name:      "requires analysis service",
			args:      []string{"shop.example.com"},
			wantErr:   ErrAnalysisUnavailable,
			wantCalls: 0,
		},
		{
			name: "delegates hostname and all source flags",
			args: []string{
				"--file", "first.yaml", "--file", "second.yaml",
				"--context", "staging", "--namespace", "shop",
				"--kubeconfig", "/tmp/kubeconfig", "--output", "yaml",
				"--no-color", "shop.example.com",
			},
			service:        &fakeAnalysisService{},
			wantCalls:      1,
			wantHost:       "shop.example.com",
			wantFormat:     "yaml",
			wantContext:    "staging",
			wantFiles:      []string{"first.yaml", "second.yaml"},
			wantNamespace:  "shop",
			wantKubeconfig: "/tmp/kubeconfig",
			wantNoColor:    true,
		},
		{
			name: "propagates analysis error",
			args: []string{"shop.example.com"},
			service: &fakeAnalysisService{
				err: errors.New("cluster is unavailable"),
			},
			wantCalls:   1,
			wantErrText: "cluster is unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			command := NewRootCommand(tt.service, stdout, stderr)
			command.SetArgs(tt.args)
			err := command.Execute()

			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("Execute() error = %v, want errors.Is(_, %v)", err, tt.wantErr)
			}
			if tt.wantErrText != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErrText)) {
				t.Fatalf("Execute() error = %v, want substring %q", err, tt.wantErrText)
			}

			fake, ok := tt.service.(*fakeAnalysisService)
			if ok {
				if fake.calls != tt.wantCalls {
					t.Fatalf("Analyze() called %d times, want %d", fake.calls, tt.wantCalls)
				}
				if tt.wantHost != "" && fake.request.Hostname != tt.wantHost {
					t.Errorf("request hostname = %q, want %q", fake.request.Hostname, tt.wantHost)
				}
				if tt.wantContext != "" && fake.request.Flags.Context != tt.wantContext {
					t.Errorf("request context = %q, want %q", fake.request.Flags.Context, tt.wantContext)
				}
				if tt.wantNamespace != "" && fake.request.Flags.Namespace != tt.wantNamespace {
					t.Errorf("request namespace = %q, want %q", fake.request.Flags.Namespace, tt.wantNamespace)
				}
				if tt.wantKubeconfig != "" && fake.request.Flags.Kubeconfig != tt.wantKubeconfig {
					t.Errorf("request kubeconfig = %q, want %q", fake.request.Flags.Kubeconfig, tt.wantKubeconfig)
				}
				if fake.request.Flags.NoColor != tt.wantNoColor {
					t.Errorf("request no-color = %t, want %t", fake.request.Flags.NoColor, tt.wantNoColor)
				}
				if tt.wantFormat != "" && fake.request.Flags.Output != tt.wantFormat {
					t.Errorf("request output = %q, want %q", fake.request.Flags.Output, tt.wantFormat)
				}
				if len(fake.request.Flags.Files) != len(tt.wantFiles) {
					t.Fatalf("request files = %v, want %v", fake.request.Flags.Files, tt.wantFiles)
				}
				for i := range tt.wantFiles {
					if fake.request.Flags.Files[i] != tt.wantFiles[i] {
						t.Errorf("request files = %v, want %v", fake.request.Flags.Files, tt.wantFiles)
						break
					}
				}

			}
		})
	}
}

func TestRootCommandWritesAnalysisToStdout(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	service := &fakeAnalysisService{
		analysis: model.Analysis{Hostname: "shop.example.com"},
	}
	command := NewRootCommand(service, stdout, stderr)
	command.SetArgs([]string{"shop.example.com"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := stdout.String(), "shop.example.com\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("stderr = %q, want empty", got)
	}
}

func TestRootCommandRendersFormatsAndKeepsDiagnosticsOnStderr(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	tests := []struct {
		format   string
		prefix   string
		contains string
	}{
		{format: "text", prefix: "shop.example.com\n", contains: "⚠ sample warning"},
		{format: "json", prefix: "{\n", contains: `"hostname": "shop.example.com"`},
		{format: "yaml", prefix: "findings:\n", contains: "sample warning"},
		{format: "mermaid", prefix: "flowchart TD\n", contains: "shop.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			service := &fakeAnalysisService{
				analysis: model.Analysis{
					Hostname: "shop.example.com",
					Source: model.SourceSnapshot{
						Mode: model.SourceModeCluster,
						Families: []model.ResourceFamilyStatus{{
							Family: "istio", Available: false, Reason: "API group not found",
						}},
					},
					Findings: []model.Finding{{
						Code: "sample", Severity: model.FindingSeverityWarning, Message: "sample warning",
					}},
				},
			}
			command := NewRootCommand(service, stdout, stderr)
			command.SetArgs([]string{"--output", tt.format, "shop.example.com"})
			if err := command.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !strings.HasPrefix(stdout.String(), tt.prefix) ||
				!strings.Contains(stdout.String(), tt.contains) {
				t.Errorf("stdout = %q, want prefix %q and substring %q", stdout.String(), tt.prefix, tt.contains)
			}
			if !strings.Contains(stderr.String(), "skipping unavailable resource family") ||
				!strings.Contains(stderr.String(), "API group not found") {
				t.Errorf("stderr = %q, want skipped-family diagnostic", stderr.String())
			}
			if strings.Contains(stdout.String(), "skipping unavailable resource family") {
				t.Errorf("stdout contains log output: %q", stdout.String())
			}
			if strings.Contains(stdout.String(), "\x1b[") {
				t.Errorf("non-TTY stdout contains ANSI styling: %q", stdout.String())
			}
		})
	}
}

func TestRootCommandLogsUnavailableFamiliesWhenHostnameIsNotFound(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	service := &fakeAnalysisService{
		analysis: model.Analysis{
			Source: model.SourceSnapshot{Families: []model.ResourceFamilyStatus{{
				Family: "gateway-api", Available: false, Reason: "CRD not installed",
			}}},
		},
		err: ErrHostnameNotFound,
	}
	command := NewRootCommand(service, stdout, stderr)
	command.SetArgs([]string{"shop.example.com"})
	if err := command.Execute(); !errors.Is(err, ErrHostnameNotFound) {
		t.Fatalf("Execute() error = %v, want hostname-not-found", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "skipping unavailable resource family") ||
		!strings.Contains(stderr.String(), "CRD not installed") {
		t.Errorf("stderr = %q, want skipped-family diagnostic", stderr.String())
	}
}

func TestColorEnabledRequiresTerminalAndHonorsOverrides(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	t.Cleanup(func() {
		if err := reader.Close(); err != nil {
			t.Errorf("close pipe reader: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Errorf("close pipe writer: %v", err)
		}
	})

	t.Setenv("NO_COLOR", "")
	if colorEnabled(writer, false) {
		t.Error("colorEnabled() = true for non-terminal output")
	}
	if colorEnabled(writer, true) {
		t.Error("colorEnabled() = true when --no-color is set")
	}
	t.Setenv("NO_COLOR", "1")
	if colorEnabled(writer, false) {
		t.Error("colorEnabled() = true when NO_COLOR is set")
	}
}

func TestRootCommandLoadsRepeatedFilesWithoutInitializingCluster(t *testing.T) {
	dir := t.TempDir()
	serviceEntryPath := filepath.Join(dir, "service-entry.yaml")
	serviceEntry := `apiVersion: networking.istio.io/v1
kind: ServiceEntry
metadata:
  name: shop
  namespace: shop
spec:
  hosts:
    - shop.example.com
  location: MESH_EXTERNAL
  resolution: DNS
  ports:
    - number: 80
      name: http
      protocol: HTTP
`
	if err := os.WriteFile(serviceEntryPath, []byte(serviceEntry), 0o600); err != nil {
		t.Fatalf("write ServiceEntry manifest: %v", err)
	}
	configMapPath := filepath.Join(dir, "additional.yaml")
	configMap := `apiVersion: v1
kind: ConfigMap
metadata:
  name: additional
  namespace: shop
`
	if err := os.WriteFile(configMapPath, []byte(configMap), 0o600); err != nil {
		t.Fatalf("write additional manifest: %v", err)
	}

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	command := NewRootCommand(NewDefaultAnalysisService(), stdout, stderr)
	command.SetArgs([]string{
		"--kubeconfig", filepath.Join(dir, "does-not-exist"),
		"--file", serviceEntryPath, "--file", configMapPath,
		"shop.example.com",
	})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.HasPrefix(stdout.String(), "shop.example.com\n") ||
		!strings.Contains(stdout.String(), "[Istio] ServiceEntry shop/shop") {
		t.Errorf("stdout = %q, want offline ServiceEntry report", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want no cluster initialization or diagnostics", stderr.String())
	}
}

func TestRootCommandResolvesOfflineEndpointAndPodStatus(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	command := NewRootCommand(NewDefaultAnalysisService(), stdout, stderr)
	command.SetArgs([]string{
		"--file", "../../testdata/manifests/endpoints.yaml",
		"shop.example.com",
	})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{
		"Service shop/api (present)",
		"Endpoint 10.0.0.10 (ready)",
		"Pod shop/api-0 (ready, phase Running)",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}
