package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninoamine/istio-route/internal/model"
	"github.com/ninoamine/istio-route/internal/source"
)

func TestDefaultAnalysisServiceSupportsOfflineFilesWithoutClusterInitialization(t *testing.T) {
	file := filepath.Join(t.TempDir(), "service-entry.yaml")
	manifest := `apiVersion: networking.istio.io/v1
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
	if err := os.WriteFile(file, []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	service := NewDefaultAnalysisService()
	analysis, err := service.Analyze(context.Background(), Request{
		Hostname: "shop.example.com",
		Flags: Flags{
			Files: []string{file}, Kubeconfig: "/path/that/must/not/be/read",
		},
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if analysis.Source.Mode != model.SourceModeFiles {
		t.Errorf("analysis source mode = %q, want files", analysis.Source.Mode)
	}
	if len(analysis.Routes) != 1 || analysis.Routes[0].Resource.Kind != "ServiceEntry" {
		t.Fatalf("analysis routes = %#v, want one ServiceEntry route", analysis.Routes)
	}
}

func TestDefaultAnalysisServiceReadsOfflineStdinAndReturnsNotFound(t *testing.T) {
	manifest := `apiVersion: v1
kind: Service
metadata:
  name: unrelated
  namespace: shop
`
	service := NewDefaultAnalysisService()
	analysis, err := service.Analyze(context.Background(), Request{
		Hostname: "absent.example.com",
		Flags: Flags{
			Files: []string{"-"},
			Stdin: strings.NewReader(manifest),
		},
	})
	if !errors.Is(err, ErrHostnameNotFound) {
		t.Fatalf("Analyze() error = %v, want ErrHostnameNotFound", err)
	}
	if analysis.Hostname != "absent.example.com" {
		t.Errorf("analysis hostname = %q, want absent.example.com", analysis.Hostname)
	}
	if analysis.Source.Mode != model.SourceModeFiles {
		t.Errorf("analysis source mode = %q, want files", analysis.Source.Mode)
	}
	if len(analysis.Source.Files) != 1 || analysis.Source.Files[0] != "-" {
		t.Errorf("analysis source files = %v, want [-]", analysis.Source.Files)
	}
	if len(analysis.Routes) != 0 {
		t.Errorf("analysis routes = %v, want none", analysis.Routes)
	}
}

func TestDefaultAnalysisServiceSurfacesOfflineSourceErrors(t *testing.T) {
	service := NewDefaultAnalysisService()
	_, err := service.Analyze(context.Background(), Request{
		Hostname: "shop.example.com",
		Flags:    Flags{Files: []string{filepath.Join(t.TempDir(), "missing.yaml")}},
	})
	var sourceErr *source.Error
	if !errors.As(err, &sourceErr) {
		t.Fatalf("Analyze() error = %v, want source.Error", err)
	}
	if !strings.Contains(err.Error(), "missing.yaml") {
		t.Errorf("Analyze() error = %v, want missing file path", err)
	}
}
