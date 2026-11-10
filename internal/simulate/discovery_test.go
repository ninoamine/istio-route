package simulate

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func listingNames(listings []RouteListing) []string {
	names := make([]string, 0, len(listings))
	for _, listing := range listings {
		names = append(names, listing.Route.Namespace+"/"+listing.Route.Name)
	}
	return names
}

func TestEligibleRoutes(t *testing.T) {
	snapshot := loadManifest(t, "simulate-attachment.yaml")
	attachments := EvaluateAttachments(snapshot, resolveFixtureWaypoint(t, snapshot, "waypoint", "app"))

	tests := []struct {
		name  string
		input []RouteAttachments
		want  []string
	}{
		{
			name:  "accepted and unconfirmed listed, rejected and listener-less omitted",
			input: attachments,
			want: []string{
				"app/accepted-a-rejected-b",
				"app/accepted-a-unconfirmed-b",
				"app/missing-observed",
				"app/section-a-only",
				"app/stale-observed",
				"app/zero-observed",
			},
		},
		{
			name:  "no attached routes yields an empty list",
			input: nil,
			want:  []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listings := EligibleRoutes(tt.input)
			if listings == nil {
				t.Fatalf("EligibleRoutes() = nil, want non-nil empty slice")
			}
			if got := listingNames(listings); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("EligibleRoutes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEligibleRoutesUnconfirmedIsLabeled(t *testing.T) {
	snapshot := loadManifest(t, "simulate-attachment.yaml")
	attachments := EvaluateAttachments(snapshot, resolveFixtureWaypoint(t, snapshot, "waypoint", "app"))

	for _, listing := range EligibleRoutes(attachments) {
		if listing.Route.Name != "stale-observed" {
			continue
		}
		if len(listing.Listeners) != 1 || listing.Listeners[0].State != AttachmentUnconfirmed {
			t.Errorf("stale-observed listeners = %+v, want one unconfirmed listener", listing.Listeners)
		}
		if listing.Listeners[0].Reason != reasonStaleStatus {
			t.Errorf("reason = %q, want %q", listing.Listeners[0].Reason, reasonStaleStatus)
		}
		return
	}
	t.Fatalf("stale-observed missing from eligible routes")
}

func TestEligibleRoutesDeterministicOrder(t *testing.T) {
	snapshot := loadManifest(t, "simulate-attachment.yaml")
	attachments := EvaluateAttachments(snapshot, resolveFixtureWaypoint(t, snapshot, "waypoint", "app"))
	want := listingNames(EligibleRoutes(attachments))

	reversed := slices.Clone(attachments)
	slices.Reverse(reversed)
	rotated := append(slices.Clone(attachments[1:]), attachments[0])

	for name, input := range map[string][]RouteAttachments{"reversed": reversed, "rotated": rotated} {
		if got := listingNames(EligibleRoutes(input)); !reflect.DeepEqual(got, want) {
			t.Errorf("%s input: EligibleRoutes() = %v, want %v", name, got, want)
		}
	}
}

func TestSelectRoute(t *testing.T) {
	snapshot := loadManifest(t, "simulate-attachment.yaml")
	attachments := EvaluateAttachments(snapshot, resolveFixtureWaypoint(t, snapshot, "waypoint", "app"))

	tests := []struct {
		name       string
		ref        RouteRef
		wantErr    error
		wantReason string
	}{
		{
			name: "accepted route is selected",
			ref:  RouteRef{Namespace: "app", Name: "section-a-only"},
		},
		{
			name: "unconfirmed route is selected",
			ref:  RouteRef{Namespace: "app", Name: "stale-observed"},
		},
		{
			name:    "absent route is not found",
			ref:     RouteRef{Namespace: "app", Name: "does-not-exist"},
			wantErr: ErrRouteNotFound,
		},
		{
			name:    "existing route attached to another waypoint is ineligible",
			ref:     RouteRef{Namespace: "other", Name: "other-ns-parent"},
			wantErr: ErrRouteIneligible,
		},
		{
			name:       "route rejected on every listener is ineligible",
			ref:        RouteRef{Namespace: "app", Name: "rejected-current"},
			wantErr:    ErrRouteIneligible,
			wantReason: "rejected on every applicable listener",
		},
		{
			name:       "route without applicable listeners is ineligible",
			ref:        RouteRef{Namespace: "other", Name: "other-same"},
			wantErr:    ErrRouteIneligible,
			wantReason: reasonNoListeners,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selection, err := SelectRoute(snapshot, attachments, tt.ref)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("SelectRoute() error = %v", err)
				}
				if selection.Route != tt.ref {
					t.Errorf("selected route = %v, want %v", selection.Route, tt.ref)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("SelectRoute() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantReason != "" && !strings.Contains(err.Error(), tt.wantReason) {
				t.Errorf("error %q does not contain %q", err, tt.wantReason)
			}
		})
	}
}

func TestSelectRouteNoApplicableListenersOnWaypoint(t *testing.T) {
	snapshot := withListeners(t, loadManifest(t, "simulate-attachment.yaml"), "ambient",
		gatewayv1.Listener{Name: "tcp", Port: 9000, Protocol: gatewayv1.TCPProtocolType})
	attachments := EvaluateAttachments(snapshot, resolveFixtureWaypoint(t, snapshot, "ambient", "app"))

	if listings := EligibleRoutes(attachments); len(listings) != 0 {
		t.Errorf("EligibleRoutes() = %v, want none", listingNames(listings))
	}
	_, err := SelectRoute(snapshot, attachments, RouteRef{Namespace: "app", Name: "hbone-route"})
	if !errors.Is(err, ErrRouteIneligible) {
		t.Fatalf("SelectRoute() error = %v, want ErrRouteIneligible", err)
	}
	if !strings.Contains(err.Error(), reasonNoListeners) {
		t.Errorf("error %q does not contain %q", err, reasonNoListeners)
	}
}
