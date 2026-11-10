package simulate

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/ninoamine/istio-route/internal/source"
)

func TestRunEndToEnd(t *testing.T) {
	primary := RouteRef{Namespace: "shop", Name: "primary"}
	tests := []struct {
		name       string
		fixture    string
		request    Request
		wantStatus Status
		wantKind   ResultKind
		wantTotals Totals
		wantRoutes int
	}{
		{
			name:       "removing primary leaves /api covered and /admin unrouted",
			fixture:    "simulate-impact.yaml",
			request:    Request{WaypointName: "waypoint", Namespace: "mesh", Route: &primary},
			wantStatus: StatusFound,
			wantKind:   ResultImpact,
			wantTotals: Totals{Unrouted: 3, StillCovered: 3},
		},
		{
			name:       "listing without a route lists attached routes",
			fixture:    "simulate-impact.yaml",
			request:    Request{WaypointName: "waypoint", Namespace: "mesh"},
			wantStatus: StatusFound,
			wantKind:   ResultListing,
			wantRoutes: 2,
		},
		{
			name:       "missing route reports listing and route-not-found",
			fixture:    "simulate-impact.yaml",
			request:    Request{WaypointName: "waypoint", Namespace: "mesh", Route: &RouteRef{Namespace: "shop", Name: "absent"}},
			wantStatus: StatusRouteNotFound,
			wantKind:   ResultListing,
			wantRoutes: 2,
		},
		{
			name:       "unknown waypoint reports waypoint-not-found",
			fixture:    "simulate-impact.yaml",
			request:    Request{WaypointName: "absent", Namespace: "mesh"},
			wantStatus: StatusWaypointNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := loadManifest(t, tt.fixture)
			result, status, err := Run(context.Background(), tt.request, snapshot)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if status != tt.wantStatus {
				t.Fatalf("status = %q, want %q", status, tt.wantStatus)
			}
			if tt.wantStatus != StatusFound && tt.wantStatus != StatusRouteNotFound && tt.wantStatus != StatusRouteIneligible {
				return
			}
			if result.Kind != tt.wantKind {
				t.Fatalf("kind = %q, want %q", result.Kind, tt.wantKind)
			}
			if tt.wantKind == ResultImpact && result.Totals != tt.wantTotals {
				t.Fatalf("totals = %+v, want %+v", result.Totals, tt.wantTotals)
			}
			if tt.wantKind == ResultListing && len(result.Routes) != tt.wantRoutes {
				t.Fatalf("listed %d routes, want %d", len(result.Routes), tt.wantRoutes)
			}
			if tt.wantKind == ResultImpact && (result.SelectedRoute == nil || *result.SelectedRoute != primary) {
				t.Fatalf("selected route = %v, want %v", result.SelectedRoute, primary)
			}
		})
	}
}

func TestRunListing(t *testing.T) {
	snapshot := loadManifest(t, "simulate-listing.yaml")
	result, status, err := Run(context.Background(), Request{WaypointName: "waypoint", Namespace: "mesh"}, snapshot)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if status != StatusFound {
		t.Fatalf("status = %q, want %q", status, StatusFound)
	}
	if result.Kind != ResultListing {
		t.Fatalf("kind = %q, want %q", result.Kind, ResultListing)
	}

	want := []RouteRef{
		{Namespace: "shop", Name: "alpha"},
		{Namespace: "shop", Name: "bravo"},
		{Namespace: "shop", Name: "charlie"},
	}
	got := make([]RouteRef, 0, len(result.Routes))
	for _, listing := range result.Routes {
		got = append(got, listing.Route)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("listed routes = %v, want %v", got, want)
	}

	states := make(map[RouteRef]AttachmentState, len(result.Routes))
	for _, listing := range result.Routes {
		for _, listener := range listing.Listeners {
			states[listing.Route] = listener.State
		}
	}
	if states[RouteRef{Namespace: "shop", Name: "bravo"}] != AttachmentUnconfirmed {
		t.Fatalf("bravo state = %q, want %q", states[RouteRef{Namespace: "shop", Name: "bravo"}], AttachmentUnconfirmed)
	}
	if states[RouteRef{Namespace: "shop", Name: "alpha"}] != AttachmentAccepted {
		t.Fatalf("alpha state = %q, want %q", states[RouteRef{Namespace: "shop", Name: "alpha"}], AttachmentAccepted)
	}
	if result.Scope.ServiceParentedExcluded != 1 {
		t.Fatalf("ServiceParentedExcluded = %d, want 1", result.Scope.ServiceParentedExcluded)
	}
}

func TestRunListingWithoutAttachedRoutes(t *testing.T) {
	snapshot := loadManifest(t, "simulate-listing.yaml")
	snapshot.Resources = slices.DeleteFunc(slices.Clone(snapshot.Resources), func(res source.Resource) bool {
		_, ok := res.Object.(*gatewayv1.HTTPRoute)
		return ok
	})

	result, status, err := Run(context.Background(), Request{WaypointName: "waypoint", Namespace: "mesh"}, snapshot)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if status != StatusFound {
		t.Fatalf("status = %q, want %q", status, StatusFound)
	}
	if result.Routes == nil || len(result.Routes) != 0 {
		t.Fatalf("routes = %v, want explicit empty list", result.Routes)
	}
}

func TestRunRouteSelectionReturnsListingOnly(t *testing.T) {
	tests := []struct {
		name       string
		route      RouteRef
		wantStatus Status
	}{
		{name: "missing route", route: RouteRef{Namespace: "shop", Name: "absent"}, wantStatus: StatusRouteNotFound},
		{name: "attached only to another waypoint", route: RouteRef{Namespace: "shop", Name: "east"}, wantStatus: StatusRouteIneligible},
		{name: "rejected on every listener", route: RouteRef{Namespace: "shop", Name: "rejected"}, wantStatus: StatusRouteIneligible},
		{name: "service-parented route", route: RouteRef{Namespace: "app", Name: "legacy"}, wantStatus: StatusRouteIneligible},
		{name: "grpc route is not an HTTPRoute", route: RouteRef{Namespace: "shop", Name: "grpc"}, wantStatus: StatusRouteNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route := tt.route
			snapshot := loadManifest(t, "simulate-listing.yaml")
			result, status, err := Run(context.Background(), Request{WaypointName: "waypoint", Namespace: "mesh", Route: &route}, snapshot)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if status != tt.wantStatus {
				t.Fatalf("status = %q, want %q", status, tt.wantStatus)
			}
			if result.Kind != ResultListing {
				t.Fatalf("kind = %q, want %q (no impact report)", result.Kind, ResultListing)
			}
			if len(result.Routes) != 3 {
				t.Fatalf("listed %d routes, want 3", len(result.Routes))
			}
			if len(result.Entries) != 0 || result.SelectedRoute != nil {
				t.Fatalf("listing carried impact data: entries=%d selected=%v", len(result.Entries), result.SelectedRoute)
			}
			if result.SelectionReason == "" {
				t.Fatalf("SelectionReason is empty, want an explanation")
			}
		})
	}
}

func TestRunIsReadOnly(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "manifests", "simulate-impact.yaml")
	before := fileHash(t, path)

	snapshot, err := (&source.FilesSource{Files: []string{path}}).Snapshot(context.Background())
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	request := Request{WaypointName: "waypoint", Namespace: "mesh", Route: &RouteRef{Namespace: "shop", Name: "primary"}}
	if _, _, err := Run(context.Background(), request, snapshot); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if after := fileHash(t, path); after != before {
		t.Fatalf("fixture hash changed: before %x after %x", before, after)
	}
}

func TestRunLocalModeWarnsAboutPartialView(t *testing.T) {
	snapshot := loadManifest(t, "simulate-impact.yaml")
	result, _, err := Run(context.Background(), Request{WaypointName: "waypoint", Namespace: "mesh"}, snapshot)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, warning := range result.Warnings {
		if strings.Contains(warning, "partial view") {
			return
		}
	}
	t.Fatalf("warnings = %v, want a partial-view notice", result.Warnings)
}

func fileHash(t *testing.T, path string) [sha256.Size]byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return sha256.Sum256(data)
}
