package simulate

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ninoamine/istio-route/internal/source"
)

func loadManifest(t *testing.T, name string) source.Snapshot {
	t.Helper()
	input := filepath.Join("..", "..", "testdata", "manifests", name)
	snapshot, err := (&source.FilesSource{Files: []string{input}}).Snapshot(context.Background())
	if err != nil {
		t.Fatalf("load fixture %q: %v", name, err)
	}
	return snapshot
}

// withNamespaces sets metadata.namespace on the named resources; an empty value leaves it missing.
func withNamespaces(snapshot source.Snapshot, namespaces map[string]string) source.Snapshot {
	for i := range snapshot.Resources {
		if ns, ok := namespaces[snapshot.Resources[i].Identity.Name]; ok {
			snapshot.Resources[i].Identity.Namespace = ns
		}
	}
	return snapshot
}

func TestValidateLocalNamespaces(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		namespaces map[string]string
		route      *RouteRef
		wantErr    string
	}{
		{
			name:       "all namespaced",
			namespaces: map[string]string{"waypoint": "app", "selected": "app", "unrelated": "app"},
			route:      &RouteRef{Namespace: "app", Name: "selected"},
		},
		{
			name:       "namespace-less waypoint gateway",
			namespaces: map[string]string{"waypoint": "", "selected": "app", "unrelated": "app"},
			route:      &RouteRef{Namespace: "app", Name: "selected"},
			wantErr:    `Gateway "waypoint": missing namespace`,
		},
		{
			name:       "namespace-less selected route",
			namespaces: map[string]string{"waypoint": "app", "selected": "", "unrelated": "app"},
			route:      &RouteRef{Namespace: "app", Name: "selected"},
			wantErr:    `HTTPRoute "selected": missing namespace`,
		},
		{
			name:       "namespace-less unrelated route",
			namespaces: map[string]string{"waypoint": "app", "selected": "app", "unrelated": ""},
			route:      &RouteRef{Namespace: "app", Name: "selected"},
		},
		{
			name:       "namespace-less route attached to waypoint but not selected",
			namespaces: map[string]string{"waypoint": "app", "selected": "", "unrelated": "app"},
			route:      &RouteRef{Namespace: "app", Name: "unrelated"},
			wantErr:    `HTTPRoute "selected": missing namespace`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			snapshot := withNamespaces(loadManifest(t, "simulate-missing-namespace.yaml"), tt.namespaces)
			err := ValidateLocalNamespaces(snapshot, "waypoint", tt.route)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateLocalNamespaces() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateLocalNamespaces() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestRequireGatewayAPI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		manifest string
		wantErr  bool
	}{
		{name: "istio-only manifest", manifest: "istio.yaml", wantErr: true},
		{name: "gateway api manifest", manifest: "gateway-audit-waypoint.yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := RequireGatewayAPI(loadManifest(t, tt.manifest))
			if (err != nil) != tt.wantErr {
				t.Fatalf("RequireGatewayAPI() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
