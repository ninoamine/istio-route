package simulate

import (
	"reflect"
	"testing"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/ninoamine/istio-route/internal/source"
)

func resolveFixtureWaypoint(t *testing.T, snapshot source.Snapshot, name, namespace string) Waypoint {
	t.Helper()
	waypoint, err := ResolveWaypoint(snapshot, name, namespace)
	if err != nil {
		t.Fatalf("ResolveWaypoint(%s/%s): %v", namespace, name, err)
	}
	return waypoint
}

func findAttachment(t *testing.T, attachments []RouteAttachments, ref RouteRef) RouteAttachments {
	t.Helper()
	for _, attachment := range attachments {
		if attachment.Route == ref {
			return attachment
		}
	}
	t.Fatalf("no attachment for %s/%s", ref.Namespace, ref.Name)
	return RouteAttachments{}
}

func attachmentStates(attachment RouteAttachments) map[string]AttachmentState {
	states := make(map[string]AttachmentState, len(attachment.Listeners))
	for _, listener := range attachment.Listeners {
		states[listener.Listener] = listener.State
	}
	return states
}

func TestEvaluateAttachmentStates(t *testing.T) {
	snapshot := loadManifest(t, "simulate-attachment.yaml")

	tests := []struct {
		name      string
		waypoint  string
		namespace string
		route     string
		want      map[string]AttachmentState
	}{
		{
			name:      "accepted on A and rejected on B",
			waypoint:  "waypoint",
			namespace: "app",
			route:     "accepted-a-rejected-b",
			want:      map[string]AttachmentState{"a": AttachmentAccepted, "b": AttachmentRejected, "same": AttachmentUnconfirmed},
		},
		{
			name:      "accepted on A and unconfirmed on B",
			waypoint:  "waypoint",
			namespace: "app",
			route:     "accepted-a-unconfirmed-b",
			want:      map[string]AttachmentState{"a": AttachmentAccepted, "b": AttachmentUnconfirmed, "same": AttachmentUnconfirmed},
		},
		{
			name:      "observedGeneration mismatched",
			waypoint:  "waypoint",
			namespace: "app",
			route:     "stale-observed",
			want:      map[string]AttachmentState{"a": AttachmentUnconfirmed},
		},
		{
			name:      "observedGeneration zero",
			waypoint:  "waypoint",
			namespace: "app",
			route:     "zero-observed",
			want:      map[string]AttachmentState{"a": AttachmentUnconfirmed},
		},
		{
			name:      "observedGeneration missing",
			waypoint:  "waypoint",
			namespace: "app",
			route:     "missing-observed",
			want:      map[string]AttachmentState{"a": AttachmentUnconfirmed},
		},
		{
			name:      "rejected current on every listener",
			waypoint:  "waypoint",
			namespace: "app",
			route:     "rejected-current",
			want:      map[string]AttachmentState{"a": AttachmentRejected, "b": AttachmentRejected, "same": AttachmentRejected},
		},
		{
			name:      "section name selects only listener A",
			waypoint:  "waypoint",
			namespace: "app",
			route:     "section-a-only",
			want:      map[string]AttachmentState{"a": AttachmentAccepted},
		},
		{
			name:      "HBONE-only waypoint accepts a route without sectionName",
			waypoint:  "ambient",
			namespace: "app",
			route:     "hbone-route",
			want:      map[string]AttachmentState{"mesh": AttachmentAccepted},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			waypoint := resolveFixtureWaypoint(t, snapshot, tt.waypoint, tt.namespace)
			attachments := EvaluateAttachments(snapshot, waypoint)
			got := attachmentStates(findAttachment(t, attachments, RouteRef{Namespace: "app", Name: tt.route}))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("listener states = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluateAttachmentReasons(t *testing.T) {
	snapshot := loadManifest(t, "simulate-attachment.yaml")
	waypoint := resolveFixtureWaypoint(t, snapshot, "waypoint", "app")
	attachments := EvaluateAttachments(snapshot, waypoint)

	tests := []struct {
		name   string
		route  string
		listen string
		want   string
	}{
		{"rejection reason applies to listener B only", "accepted-a-rejected-b", "b", "listener b rejects this route"},
		{"stale observedGeneration", "stale-observed", "a", reasonStaleStatus},
		{"zero observedGeneration", "zero-observed", "a", reasonStaleStatus},
		{"missing observedGeneration", "missing-observed", "a", reasonStaleStatus},
		{"no status for listener", "accepted-a-unconfirmed-b", "b", reasonNoStatus},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attachment := findAttachment(t, attachments, RouteRef{Namespace: "app", Name: tt.route})
			for _, listener := range attachment.Listeners {
				if listener.Listener == tt.listen {
					if listener.Reason != tt.want {
						t.Errorf("reason = %q, want %q", listener.Reason, tt.want)
					}
					return
				}
			}
			t.Fatalf("listener %q not evaluated for route %s", tt.listen, tt.route)
		})
	}
}

func TestEvaluateAttachmentScope(t *testing.T) {
	snapshot := loadManifest(t, "simulate-attachment.yaml")
	appWaypoint := resolveFixtureWaypoint(t, snapshot, "waypoint", "app")
	otherWaypoint := resolveFixtureWaypoint(t, snapshot, "waypoint", "other")

	t.Run("same-named waypoint in another namespace does not attach", func(t *testing.T) {
		for _, attachment := range EvaluateAttachments(snapshot, appWaypoint) {
			if attachment.Route == (RouteRef{Namespace: "other", Name: "other-ns-parent"}) {
				t.Fatalf("route other/other-ns-parent attached to app/waypoint")
			}
		}
	})

	t.Run("HBONE listener without allowedRoutes is unconfirmed", func(t *testing.T) {
		attachment := findAttachment(t, EvaluateAttachments(snapshot, otherWaypoint),
			RouteRef{Namespace: "other", Name: "other-ns-parent"})
		got := attachmentStates(attachment)
		want := map[string]AttachmentState{"mesh": AttachmentUnconfirmed}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("listener states = %v, want %v", got, want)
		}
		if reason := attachment.Listeners[0].Reason; reason != reasonHBONEAllowed {
			t.Errorf("reason = %q, want %q", reason, reasonHBONEAllowed)
		}
	})

	t.Run("route from another namespace is not applicable under Same", func(t *testing.T) {
		attachment := findAttachment(t, EvaluateAttachments(snapshot, appWaypoint),
			RouteRef{Namespace: "other", Name: "other-same"})
		if len(attachment.Listeners) != 0 {
			t.Errorf("listeners = %v, want none", attachment.Listeners)
		}
		if attachment.Reason != reasonNoListeners {
			t.Errorf("reason = %q, want %q", attachment.Reason, reasonNoListeners)
		}
	})

	t.Run("service-parented route is not attached", func(t *testing.T) {
		for _, attachment := range EvaluateAttachments(snapshot, appWaypoint) {
			if attachment.Route.Name == "service-parented" {
				t.Fatalf("service-parented route attached to waypoint")
			}
		}
		want := []ServiceParentedExclusion{{Route: RouteRef{Namespace: "app", Name: "service-parented"}, Service: "api"}}
		if got := ServiceParentedExclusions(snapshot); !reflect.DeepEqual(got, want) {
			t.Errorf("ServiceParentedExclusions() = %v, want %v", got, want)
		}
	})
}

func TestEvaluateAttachmentsWithoutApplicableListeners(t *testing.T) {
	snapshot := withListeners(t, loadManifest(t, "simulate-attachment.yaml"), "ambient",
		gatewayv1.Listener{Name: "tcp", Port: 9000, Protocol: gatewayv1.TCPProtocolType})
	waypoint := resolveFixtureWaypoint(t, snapshot, "ambient", "app")

	attachment := findAttachment(t, EvaluateAttachments(snapshot, waypoint),
		RouteRef{Namespace: "app", Name: "hbone-route"})
	if len(attachment.Listeners) != 0 {
		t.Errorf("listeners = %v, want none", attachment.Listeners)
	}
	if attachment.Reason != reasonNoListeners {
		t.Errorf("reason = %q, want %q", attachment.Reason, reasonNoListeners)
	}
}
