package simulate

import (
	"reflect"
	"testing"
)

func pathCandidate(name string, route RouteRef, prefix string, state AttachmentState) Candidate {
	return Candidate{
		Route:     route,
		RuleIndex: 1,
		Region:    pathAtom("PathPrefix", prefix),
		State:     state,
		Matches:   []Predicate{pathAtom("PathPrefix", prefix)},
		Backends:  []BackendRef{{Namespace: "shop", Name: name}},
	}
}

func TestWinner(t *testing.T) {
	routeA := RouteRef{Namespace: "shop", Name: "a"}
	routeB := RouteRef{Namespace: "shop", Name: "b"}
	accepted := View{
		"http": {
			pathCandidate("a", routeA, "/api", AttachmentAccepted),
			pathCandidate("b", routeB, "/", AttachmentAccepted),
		},
	}
	unconfirmedA := View{
		"http": {
			pathCandidate("a", routeA, "/api", AttachmentUnconfirmed),
			pathCandidate("b", routeB, "/", AttachmentAccepted),
		},
	}

	tests := []struct {
		name      string
		view      View
		listener  string
		region    Predicate
		wantState string
		wantRoute *RouteRef
		wantCert  Certainty
	}{
		{
			name:      "region inside the higher-precedence rule is served by it",
			view:      accepted,
			listener:  "http",
			region:    pathAtom("PathPrefix", "/api/v2"),
			wantState: ProviderKnown,
			wantRoute: &routeA,
			wantCert:  CertaintyDefinite,
		},
		{
			name:      "disjoint higher rule falls through to the next rule",
			view:      accepted,
			listener:  "http",
			region:    pathAtom("PathPrefix", "/apix"),
			wantState: ProviderKnown,
			wantRoute: &routeB,
			wantCert:  CertaintyDefinite,
		},
		{
			name:      "region wider than the top rule cannot be attributed",
			view:      accepted,
			listener:  "http",
			region:    pathAtom("PathPrefix", "/"),
			wantState: ProviderUnknown,
			wantCert:  CertaintyUndetermined,
		},
		{
			name:      "unconfirmed winner is reported with undetermined certainty",
			view:      unconfirmedA,
			listener:  "http",
			region:    pathAtom("PathPrefix", "/api"),
			wantState: ProviderKnown,
			wantRoute: &routeA,
			wantCert:  CertaintyUndetermined,
		},
		{
			name:      "listener with no candidates has no provider",
			view:      accepted,
			listener:  "https",
			region:    pathAtom("PathPrefix", "/api"),
			wantState: ProviderNone,
			wantCert:  CertaintyDefinite,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, cert := Winner(tt.view, tt.listener, tt.region)
			if got.State != tt.wantState {
				t.Fatalf("state = %q, want %q", got.State, tt.wantState)
			}
			if cert != tt.wantCert {
				t.Fatalf("certainty = %q, want %q", cert, tt.wantCert)
			}
			if tt.wantRoute == nil {
				if got.Provider != nil {
					t.Fatalf("provider = %+v, want nil", got.Provider)
				}
				return
			}
			if got.Provider == nil || got.Provider.Route != *tt.wantRoute {
				t.Fatalf("provider = %+v, want route %+v", got.Provider, *tt.wantRoute)
			}
		})
	}
}

func TestBuildViewIsDeterministicAcrossInputOrder(t *testing.T) {
	snapshot := loadManifest(t, "simulate-impact.yaml")
	waypoint, err := ResolveWaypoint(snapshot, "waypoint", "mesh")
	if err != nil {
		t.Fatalf("ResolveWaypoint() error = %v", err)
	}
	attachments := EvaluateAttachments(snapshot, waypoint)
	reversed := make([]RouteAttachments, len(attachments))
	for i := range attachments {
		reversed[len(attachments)-1-i] = attachments[i]
	}

	forward := BuildView(snapshot, waypoint, attachments)
	backward := BuildView(snapshot, waypoint, reversed)
	if !reflect.DeepEqual(candidateKeys(forward), candidateKeys(backward)) {
		t.Fatalf("view order depends on input order:\nforward:  %v\nbackward: %v",
			candidateKeys(forward), candidateKeys(backward))
	}
}

func candidateKeys(view View) map[string][]string {
	out := make(map[string][]string, len(view))
	for listener, candidates := range view {
		for _, c := range candidates {
			out[listener] = append(out[listener], c.Route.Namespace+"/"+c.Route.Name)
		}
	}
	return out
}
