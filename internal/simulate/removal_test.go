package simulate

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/ninoamine/istio-route/internal/model"
	"github.com/ninoamine/istio-route/internal/source"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	httpRouteKey = gatewayAPIGroup + "/HTTPRoute"
)

func resourceKeys(snapshot source.Snapshot) []string {
	keys := make([]string, 0, len(snapshot.Resources))
	for _, resource := range snapshot.Resources {
		identity := resource.Identity
		keys = append(keys, identity.Group+"/"+identity.Kind+" "+identity.Namespace+"/"+identity.Name)
	}
	return keys
}

func TestWithout(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*testing.T, source.Snapshot) source.Snapshot
		target  RouteRef
		removed []string
	}{
		{
			name:    "removes only the exact HTTPRoute",
			target:  RouteRef{Namespace: "shop", Name: "primary"},
			removed: []string{httpRouteKey + " shop/primary"},
		},
		{
			name:   "same name in another namespace is kept",
			target: RouteRef{Namespace: "shop", Name: "primary"},
			mutate: func(t *testing.T, s source.Snapshot) source.Snapshot {
				copied := s.Resources[findRoute(t, s, "shop", "primary")]
				copied.Identity.Namespace = "other"
				s.Resources = append(s.Resources, copied)
				return s
			},
			removed: []string{httpRouteKey + " shop/primary"},
		},
		{
			name:   "same-named Service is kept",
			target: RouteRef{Namespace: "shop", Name: "primary"},
			mutate: func(_ *testing.T, s source.Snapshot) source.Snapshot {
				s.Resources = append(s.Resources, source.Resource{
					Identity: model.ResourceIdentity{Version: "v1", Kind: "Service", Namespace: "shop", Name: "primary"},
					GVK:      schema.GroupVersionKind{Version: "v1", Kind: "Service"},
				})
				return s
			},
			removed: []string{httpRouteKey + " shop/primary"},
		},
		{
			name:   "non-existent identity removes nothing",
			target: RouteRef{Namespace: "shop", Name: "missing"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := loadManifest(t, "simulate-impact.yaml")
			if tt.mutate != nil {
				snapshot = tt.mutate(t, snapshot)
			}
			original := append([]source.Resource(nil), snapshot.Resources...)
			originalKeys := resourceKeys(snapshot)

			got := Without(snapshot, tt.target)

			if !reflect.DeepEqual(resourceKeys(snapshot), originalKeys) {
				t.Fatalf("original snapshot mutated: got %v, want %v", resourceKeys(snapshot), originalKeys)
			}
			for i := range original {
				if snapshot.Resources[i].Object != original[i].Object {
					t.Fatalf("original resource %d changed", i)
				}
			}

			var wantKeys []string
			var wantResources []source.Resource
			for i, key := range originalKeys {
				if slices.Contains(tt.removed, key) {
					continue
				}
				wantKeys = append(wantKeys, key)
				wantResources = append(wantResources, original[i])
			}
			if !reflect.DeepEqual(resourceKeys(got), wantKeys) {
				t.Fatalf("Without() keys = %v, want %v", resourceKeys(got), wantKeys)
			}
			for i := range wantResources {
				if got.Resources[i].Object != wantResources[i].Object {
					t.Fatalf("retained resource %d lost object identity", i)
				}
			}
			if !reflect.DeepEqual(got.Metadata, snapshot.Metadata) {
				t.Fatalf("metadata changed: got %+v, want %+v", got.Metadata, snapshot.Metadata)
			}
		})
	}
}

func TestImpactFixtureBytesUnchanged(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "manifests", "simulate-impact.yaml")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	beforeHash := sha256.Sum256(before)

	_ = Without(loadManifest(t, "simulate-impact.yaml"), RouteRef{Namespace: "shop", Name: "primary"})

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("re-read fixture: %v", err)
	}
	if sha256.Sum256(after) != beforeHash {
		t.Fatal("fixture bytes changed after Without")
	}
}

func findRoute(t *testing.T, snapshot source.Snapshot, namespace, name string) int {
	t.Helper()
	for i, resource := range snapshot.Resources {
		identity := resource.Identity
		if identity.Group == gatewayAPIGroup && identity.Kind == "HTTPRoute" &&
			identity.Namespace == namespace && identity.Name == name {
			return i
		}
	}
	t.Fatalf("route %s/%s not found", namespace, name)
	return -1
}
