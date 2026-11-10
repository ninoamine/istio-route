package simulate

import (
	"errors"
	"testing"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/ninoamine/istio-route/internal/audit"
	"github.com/ninoamine/istio-route/internal/source"
)

// withoutGatewayClass drops the named GatewayClass from the snapshot.
func withoutGatewayClass(snapshot source.Snapshot, name string) source.Snapshot {
	kept := snapshot.Resources[:0]
	for _, res := range snapshot.Resources {
		if res.Identity.Kind == "GatewayClass" && res.Identity.Name == name {
			continue
		}
		kept = append(kept, res)
	}
	snapshot.Resources = kept
	return snapshot
}

// withListeners replaces the listeners of the named Gateway in the snapshot.
func withListeners(t *testing.T, snapshot source.Snapshot, name string, listeners ...gatewayv1.Listener) source.Snapshot {
	t.Helper()
	for _, res := range snapshot.Resources {
		gw, ok := res.Object.(*gatewayv1.Gateway)
		if ok && gw.Name == name {
			gw.Spec.Listeners = listeners
			return snapshot
		}
	}
	t.Fatalf("gateway %q not found in fixture", name)
	return snapshot
}

// withDuplicateGateway adds a copy of the named Gateway in another namespace.
func withDuplicateGateway(t *testing.T, snapshot source.Snapshot, name, namespace string) source.Snapshot {
	t.Helper()
	for _, res := range snapshot.Resources {
		gw, ok := res.Object.(*gatewayv1.Gateway)
		if !ok || gw.Name != name {
			continue
		}
		dup := gw.DeepCopy()
		dup.Namespace = namespace
		res.Identity.Namespace = namespace
		res.Object = dup
		snapshot.Resources = append(snapshot.Resources, res)
		return snapshot
	}
	t.Fatalf("gateway %q not found in fixture", name)
	return snapshot
}

func TestResolveWaypoint(t *testing.T) {
	t.Parallel()

	hbone := gatewayv1.Listener{Name: "mesh", Port: 15008, Protocol: "HBONE"}
	httpListener := gatewayv1.Listener{Name: "http", Port: 80, Protocol: gatewayv1.HTTPProtocolType}
	tcpListener := gatewayv1.Listener{Name: "tcp", Port: 9000, Protocol: gatewayv1.TCPProtocolType}

	tests := []struct {
		name            string
		build           func(t *testing.T) source.Snapshot
		waypointName    string
		namespace       string
		wantErr         error
		wantValidation  ValidationState
		wantListenerLen int
		wantNamespace   string
	}{
		{
			name:            "valid waypoint is confirmed",
			build:           func(t *testing.T) source.Snapshot { return loadManifest(t, "gateway-audit-waypoint.yaml") },
			waypointName:    "waypoint",
			wantValidation:  ValidationConfirmed,
			wantListenerLen: 1,
			wantNamespace:   "app",
		},
		{
			name: "missing GatewayClass leaves waypoint unconfirmed",
			build: func(t *testing.T) source.Snapshot {
				return withoutGatewayClass(loadManifest(t, "gateway-audit-waypoint.yaml"), "istio-waypoint")
			},
			waypointName:    "waypoint",
			wantValidation:  ValidationUnconfirmed,
			wantListenerLen: 1,
			wantNamespace:   "app",
		},
		{
			name:         "ingress gateway is not a waypoint",
			build:        func(t *testing.T) source.Snapshot { return loadManifest(t, "simulate-ingress.yaml") },
			waypointName: "ingress",
			wantErr:      ErrNotWaypoint,
		},
		{
			name:            "HBONE-only waypoint has one applicable listener",
			build:           func(t *testing.T) source.Snapshot { return loadManifest(t, "gateway-audit-waypoint.yaml") },
			waypointName:    "waypoint",
			wantValidation:  ValidationConfirmed,
			wantListenerLen: 1,
			wantNamespace:   "app",
		},
		{
			name: "HBONE and HTTP waypoint has both applicable listeners",
			build: func(t *testing.T) source.Snapshot {
				return withListeners(t, loadManifest(t, "gateway-audit-waypoint.yaml"), "waypoint", hbone, httpListener)
			},
			waypointName:    "waypoint",
			wantValidation:  ValidationConfirmed,
			wantListenerLen: 2,
			wantNamespace:   "app",
		},
		{
			name: "TCP-only waypoint resolves with zero applicable listeners",
			build: func(t *testing.T) source.Snapshot {
				return withListeners(t, loadManifest(t, "gateway-audit-waypoint.yaml"), "waypoint", tcpListener)
			},
			waypointName:    "waypoint",
			wantValidation:  ValidationConfirmed,
			wantListenerLen: 0,
			wantNamespace:   "app",
		},
		{
			name:         "unknown waypoint name",
			build:        func(t *testing.T) source.Snapshot { return loadManifest(t, "gateway-audit-waypoint.yaml") },
			waypointName: "missing",
			wantErr:      ErrWaypointNotFound,
		},
		{
			name: "same name in two namespaces without namespace is ambiguous",
			build: func(t *testing.T) source.Snapshot {
				return withDuplicateGateway(t, loadManifest(t, "gateway-audit-waypoint.yaml"), "waypoint", "other")
			},
			waypointName: "waypoint",
			wantErr:      audit.ErrAmbiguousGateway,
		},
		{
			name: "same name with namespace selects that gateway",
			build: func(t *testing.T) source.Snapshot {
				return withDuplicateGateway(t, loadManifest(t, "gateway-audit-waypoint.yaml"), "waypoint", "other")
			},
			waypointName:    "waypoint",
			namespace:       "app",
			wantValidation:  ValidationConfirmed,
			wantListenerLen: 1,
			wantNamespace:   "app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			waypoint, err := ResolveWaypoint(tt.build(t), tt.waypointName, tt.namespace)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ResolveWaypoint() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveWaypoint() error = %v", err)
			}
			if waypoint.Validation != tt.wantValidation {
				t.Fatalf("Validation = %q, want %q", waypoint.Validation, tt.wantValidation)
			}
			if tt.wantValidation == ValidationUnconfirmed && waypoint.ValidationReason == "" {
				t.Fatal("unconfirmed waypoint has no validation reason")
			}
			if len(waypoint.Listeners) != tt.wantListenerLen {
				t.Fatalf("listeners = %d, want %d", len(waypoint.Listeners), tt.wantListenerLen)
			}
			if waypoint.Namespace != tt.wantNamespace {
				t.Fatalf("Namespace = %q, want %q", waypoint.Namespace, tt.wantNamespace)
			}
		})
	}
}
