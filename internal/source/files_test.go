package source

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninoamine/istio-route/internal/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestFilesSourceSnapshot(t *testing.T) {
	first := writeManifest(t, "first.yaml", `apiVersion: v1
kind: Service
metadata:
  name: api
  namespace: shop
spec:
  ports:
    - port: 8080
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: route
  namespace: shop
spec:
  hostnames:
    - shop.example.com
`)
	second := writeManifest(t, "second.yaml", `apiVersion: example.test/v1
kind: Widget
metadata:
  name: sample
  namespace: shop
`)

	snapshot, err := (&FilesSource{Files: []string{first, second}}).Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if snapshot.Metadata.Mode != model.SourceModeFiles {
		t.Errorf("source mode = %q, want files", snapshot.Metadata.Mode)
	}
	if len(snapshot.Metadata.Files) != 2 || snapshot.Metadata.Files[0] != first || snapshot.Metadata.Files[1] != second {
		t.Errorf("source files = %v, want [%q %q]", snapshot.Metadata.Files, first, second)
	}
	if len(snapshot.Resources) != 3 {
		t.Fatalf("resource count = %d, want 3", len(snapshot.Resources))
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("snapshot validation: %v", err)
	}

	service, ok := snapshot.Resources[0].Object.(*corev1.Service)
	if !ok {
		t.Fatalf("first object type = %T, want *corev1.Service", snapshot.Resources[0].Object)
	}
	if service.Name != "api" || snapshot.Resources[0].GVK.Kind != "Service" {
		t.Errorf("service object or GVK not preserved: %#v / %#v", service, snapshot.Resources[0].GVK)
	}

	var route *gatewayv1.HTTPRoute
	for _, resource := range snapshot.Resources {
		if resource.Identity.Kind == "HTTPRoute" {
			route, ok = resource.Object.(*gatewayv1.HTTPRoute)
			if !ok {
				t.Fatalf("HTTPRoute object type = %T", resource.Object)
			}
			if len(route.Status.Parents) != 0 {
				t.Errorf("absent manifest status was populated: %#v", route.Status)
			}
		}
	}
	if route == nil {
		t.Fatal("snapshot did not preserve HTTPRoute")
	}

	unknown, ok := snapshot.Resources[2].Object.(*unstructured.Unstructured)
	if !ok {
		t.Fatalf("unknown object type = %T, want *unstructured.Unstructured", snapshot.Resources[2].Object)
	}
	if unknown.GetAPIVersion() != "example.test/v1" || unknown.GetKind() != "Widget" {
		t.Errorf("unknown object GVK = %s/%s, want example.test/v1/Widget", unknown.GetAPIVersion(), unknown.GetKind())
	}
}

func TestFilesSourceRejectsDuplicateIdentities(t *testing.T) {
	first := writeManifest(t, "first.yaml", `apiVersion: v1
kind: Service
metadata:
  name: api
  namespace: shop
`)
	second := writeManifest(t, "second.yaml", `apiVersion: v1
kind: Service
metadata:
  name: api
  namespace: shop
`)

	_, err := (&FilesSource{Files: []string{first, second}}).Snapshot(context.Background())
	if err == nil || !strings.Contains(err.Error(), "duplicate resource") {
		t.Fatalf("Snapshot() error = %v, want duplicate resource error", err)
	}
}

func TestFilesSourceReportsMalformedYAMLAndMissingFiles(t *testing.T) {
	malformed := writeManifest(t, "malformed.yaml", "apiVersion: [\n")
	tests := []struct {
		name string
		file string
		want string
	}{
		{name: "malformed YAML", file: malformed, want: "malformed.yaml"},
		{name: "missing file", file: filepath.Join(t.TempDir(), "missing.yaml"), want: "missing.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := (&FilesSource{Files: []string{tt.file}}).Snapshot(context.Background())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Snapshot() error = %v, want file context %q", err, tt.want)
			}
		})
	}
}

func TestFilesSourceRequiresFilesAndHonorsContext(t *testing.T) {
	if _, err := (&FilesSource{}).Snapshot(context.Background()); err == nil {
		t.Fatal("Snapshot() succeeded without files")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	file := writeManifest(t, "valid.yaml", `apiVersion: v1
kind: Service
metadata:
  name: api
`)
	if _, err := (&FilesSource{Files: []string{file}}).Snapshot(ctx); err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("Snapshot() error = %v, want context cancellation", err)
	}
}

func writeManifest(t *testing.T, name, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
	return path
}
