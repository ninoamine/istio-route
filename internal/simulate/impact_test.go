package simulate

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"testing"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/ninoamine/istio-route/internal/source"
)

// impactFor runs the removal analysis for target against a manifest fixture on the mesh/waypoint gateway.
func impactFor(t *testing.T, fixture string, target RouteRef, attachmentOrder func([]RouteAttachments) []RouteAttachments) Result {
	t.Helper()
	snapshot := loadManifest(t, fixture)
	waypoint, err := ResolveWaypoint(snapshot, "waypoint", "mesh")
	if err != nil {
		t.Fatalf("ResolveWaypoint() error = %v", err)
	}
	attachments := EvaluateAttachments(snapshot, waypoint)
	if attachmentOrder != nil {
		attachments = attachmentOrder(attachments)
	}
	before := BuildView(snapshot, waypoint, attachments)

	removed := Without(snapshot, target)
	afterAttachments := EvaluateAttachments(removed, waypoint)
	if attachmentOrder != nil {
		afterAttachments = attachmentOrder(afterAttachments)
	}
	after := BuildView(removed, waypoint, afterAttachments)
	return BuildImpact(before, after, target, waypoint, Scope{DirectHTTPRouteOnly: true})
}

func reverseAttachments(in []RouteAttachments) []RouteAttachments {
	out := make([]RouteAttachments, len(in))
	for i := range in {
		out[len(in)-1-i] = in[i]
	}
	return out
}

// outcomeLines renders entries as "listener rule outcome" for compact assertions.
func outcomeLines(entries []Entry) []string {
	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		lines = append(lines, fmt.Sprintf("%s %d %s", e.Listener, e.Rule.Index, e.Outcome))
	}
	return lines
}

func TestBuildImpact(t *testing.T) {
	primary := RouteRef{Namespace: "shop", Name: "primary"}
	tests := []struct {
		name         string
		fixture      string
		target       RouteRef
		wantLines    []string
		wantTotals   Totals
		wantPartial  bool
		wantWarnings int
	}{
		{
			name:    "fallback covers GET but POST /admin becomes unrouted",
			fixture: "simulate-impact.yaml",
			target:  primary,
			wantLines: []string{
				"hbone 1 still-covered", "hbone 2 unrouted",
				"http 1 still-covered", "http 2 unrouted",
				"https 1 still-covered", "https 2 unrouted",
			},
			wantTotals: Totals{Unrouted: 3, StillCovered: 3},
		},
		{
			name:       "sole route leaves all traffic unrouted and warns",
			fixture:    "simulate-sole-route.yaml",
			target:     primary,
			wantTotals: Totals{Unrouted: 6},
			wantLines: []string{
				"hbone 1 unrouted", "hbone 2 unrouted",
				"http 1 unrouted", "http 2 unrouted",
				"https 1 unrouted", "https 2 unrouted",
			},
			wantWarnings: 1,
		},
		{
			name:        "one rule with GET and POST splits into still-covered and unrouted",
			fixture:     "simulate-partial.yaml",
			target:      primary,
			wantPartial: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := impactFor(t, tt.fixture, tt.target, nil)
			if tt.wantLines != nil {
				if got := outcomeLines(result.Entries); !reflect.DeepEqual(got, tt.wantLines) {
					t.Fatalf("entries = %v, want %v", got, tt.wantLines)
				}
			}
			if tt.wantTotals != (Totals{}) && result.Totals != tt.wantTotals {
				t.Fatalf("totals = %+v, want %+v", result.Totals, tt.wantTotals)
			}
			if tt.wantPartial {
				if len(result.RuleSummaries) == 0 || result.RuleSummaries[0].Partial != true {
					t.Fatalf("rule summaries = %+v, want first rule partial", result.RuleSummaries)
				}
			}
			if len(result.Warnings) != tt.wantWarnings {
				t.Fatalf("warnings = %v, want %d", result.Warnings, tt.wantWarnings)
			}
		})
	}
}

func TestBuildImpactShadowedRuleIsNoChange(t *testing.T) {
	result := impactFor(t, "simulate-shadowed.yaml", RouteRef{Namespace: "shop", Name: "primary"}, nil)

	var noChange, providers []string
	for _, e := range result.Entries {
		if e.Outcome == ClassificationNoChange {
			noChange = append(noChange, e.Listener)
			if e.Before.Provider == nil {
				t.Fatalf("no-change entry on %s has no provider", e.Listener)
			}
			providers = append(providers, e.Before.Provider.Route.Namespace+"/"+e.Before.Provider.Route.Name)
		}
	}
	if len(noChange) == 0 {
		t.Fatalf("expected no-change entries for shadowed traffic, got %v", outcomeLines(result.Entries))
	}
	for _, p := range providers {
		if p != "other/shadow" {
			t.Fatalf("shadowing provider = %q, want other/shadow", p)
		}
	}
	if result.Totals.NoChange != len(noChange) {
		t.Fatalf("totals.NoChange = %d, want %d", result.Totals.NoChange, len(noChange))
	}
}

func TestBuildImpactIsDeterministicAcrossInputOrder(t *testing.T) {
	target := RouteRef{Namespace: "shop", Name: "primary"}
	for _, fixture := range []string{"simulate-impact.yaml", "simulate-shadowed.yaml"} {
		t.Run(fixture, func(t *testing.T) {
			forward := impactFor(t, fixture, target, nil)
			backward := impactFor(t, fixture, target, reverseAttachments)
			if !reflect.DeepEqual(outcomeLines(forward.Entries), outcomeLines(backward.Entries)) {
				t.Fatalf("entries depend on input order:\nforward:  %v\nbackward: %v",
					outcomeLines(forward.Entries), outcomeLines(backward.Entries))
			}
			if !reflect.DeepEqual(forward.Totals, backward.Totals) {
				t.Fatalf("totals depend on input order: %+v vs %+v", forward.Totals, backward.Totals)
			}
		})
	}
}

func TestWithoutLeavesImpactFixtureUnmodified(t *testing.T) {
	snapshot := loadManifest(t, "simulate-impact.yaml")
	before := resourceKeys(snapshot)
	_ = impactFor(t, "simulate-impact.yaml", RouteRef{Namespace: "shop", Name: "primary"}, nil)
	if !reflect.DeepEqual(resourceKeys(snapshot), before) {
		t.Fatal("impact analysis mutated the source snapshot")
	}
}

// impactIn runs the removal analysis for target against snapshot using a resolved waypoint.
func impactIn(t *testing.T, snapshot source.Snapshot, waypoint Waypoint, target RouteRef) Result {
	t.Helper()
	before := BuildView(snapshot, waypoint, EvaluateAttachments(snapshot, waypoint))
	removed := Without(snapshot, target)
	after := BuildView(removed, waypoint, EvaluateAttachments(removed, waypoint))
	return BuildImpact(before, after, target, waypoint, Scope{DirectHTTPRouteOnly: true})
}

func providerName(view ProviderView) string {
	if view.Provider == nil {
		return ""
	}
	return view.Provider.Route.Namespace + "/" + view.Provider.Route.Name
}

func TestBuildImpactScopedToSelectedWaypoint(t *testing.T) {
	primary := RouteRef{Namespace: "shop", Name: "primary"}
	tests := []struct {
		name         string
		waypoint     string
		namespace    string
		wantProvider string
	}{
		{name: "wp-a is covered only by only-a", waypoint: "wp-a", namespace: "mesh", wantProvider: "shop/only-a"},
		{name: "wp-b is covered only by only-b", waypoint: "wp-b", namespace: "mesh", wantProvider: "shop/only-b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := loadManifest(t, "simulate-multi-waypoint.yaml")
			waypoint, err := ResolveWaypoint(snapshot, tt.waypoint, tt.namespace)
			if err != nil {
				t.Fatalf("ResolveWaypoint() error = %v", err)
			}
			result := impactIn(t, snapshot, waypoint, primary)
			if got, want := outcomeLines(result.Entries), []string{"hbone 1 no-change"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("entries = %v, want %v", got, want)
			}
			for _, e := range result.Entries {
				if got := providerName(e.After); got != tt.wantProvider {
					t.Fatalf("after provider = %q, want %q", got, tt.wantProvider)
				}
			}
		})
	}
}

func TestOtherParentNotesNameUnselectedGateways(t *testing.T) {
	snapshot := loadManifest(t, "simulate-multi-waypoint.yaml")
	wpA, err := ResolveWaypoint(snapshot, "wp-a", "mesh")
	if err != nil {
		t.Fatalf("ResolveWaypoint() error = %v", err)
	}
	primary := RouteRef{Namespace: "shop", Name: "primary"}
	want := []string{"also attached to mesh/wp-b; impact not included"}
	if got := otherParentNotes(snapshot, primary, wpA); !reflect.DeepEqual(got, want) {
		t.Fatalf("otherParentNotes() = %v, want %v", got, want)
	}
	if got := otherParentNotes(snapshot, RouteRef{Namespace: "shop", Name: "only-a"}, wpA); len(got) != 0 {
		t.Fatalf("otherParentNotes(only-a) = %v, want none", got)
	}

	result, status, err := Run(context.Background(), Request{WaypointName: "wp-a", Namespace: "mesh", Route: &primary}, snapshot)
	if err != nil || status != StatusFound {
		t.Fatalf("Run() = %v, %v; want found", status, err)
	}
	if !slices.Contains(result.Warnings, want[0]) || !slices.Equal(result.Scope.OtherParentNotes, want) {
		t.Fatalf("warnings = %v, scope notes = %v; want %q", result.Warnings, result.Scope.OtherParentNotes, want[0])
	}
}

func TestMissingGatewayClassDetectedFromManifest(t *testing.T) {
	snapshot := loadManifest(t, "simulate-no-class.yaml")
	waypoint, err := ResolveWaypoint(snapshot, "wp-a", "mesh")
	if err != nil {
		t.Fatalf("ResolveWaypoint() error = %v", err)
	}
	if waypoint.Validation != ValidationUnconfirmed {
		t.Fatalf("waypoint validation = %q, want unconfirmed", waypoint.Validation)
	}
}

func TestMissingGatewayClassMarksRemovalUndetermined(t *testing.T) {
	primary := RouteRef{Namespace: "shop", Name: "primary"}
	snapshot := loadManifest(t, "simulate-impact.yaml")
	waypoint, err := ResolveWaypoint(snapshot, "waypoint", "mesh")
	if err != nil {
		t.Fatalf("ResolveWaypoint() error = %v", err)
	}
	waypoint.Validation = ValidationUnconfirmed
	waypoint.ValidationReason = gatewayClassUnavailableReason

	result := impactIn(t, snapshot, waypoint, primary)
	if len(result.Entries) == 0 {
		t.Fatal("expected entries for the removed route")
	}
	for _, e := range result.Entries {
		if e.Outcome != ClassificationUndetermined {
			t.Fatalf("entry outcome = %q, want undetermined", e.Outcome)
		}
		if !slices.Contains(e.Reasons, gatewayClassUnavailableReason) {
			t.Fatalf("reasons = %v, want %q", e.Reasons, gatewayClassUnavailableReason)
		}
	}
}

func TestMissingGatewayClassKeepsNoChangeEntries(t *testing.T) {
	snapshot := loadManifest(t, "simulate-shadowed.yaml")
	waypoint, err := ResolveWaypoint(snapshot, "waypoint", "mesh")
	if err != nil {
		t.Fatalf("ResolveWaypoint() error = %v", err)
	}
	waypoint.Validation = ValidationUnconfirmed
	waypoint.ValidationReason = gatewayClassUnavailableReason

	result := impactIn(t, snapshot, waypoint, RouteRef{Namespace: "shop", Name: "primary"})
	noChange := 0
	for _, e := range result.Entries {
		if e.Outcome != ClassificationNoChange {
			continue
		}
		noChange++
		if e.Provisional != nil {
			t.Fatalf("no-change entry gained provisional state: %+v", e.Provisional)
		}
	}
	if noChange == 0 {
		t.Fatalf("expected no-change entries, got %v", outcomeLines(result.Entries))
	}
}

func TestUnconfirmedTargetAttachmentIsUndetermined(t *testing.T) {
	primary := RouteRef{Namespace: "shop", Name: "primary"}
	snapshot := loadManifest(t, "simulate-impact.yaml")
	for _, res := range snapshot.Resources {
		if route, ok := res.Object.(*gatewayv1.HTTPRoute); ok && res.Identity.Name == primary.Name {
			route.Status = gatewayv1.HTTPRouteStatus{}
		}
	}
	waypoint, err := ResolveWaypoint(snapshot, "waypoint", "mesh")
	if err != nil {
		t.Fatalf("ResolveWaypoint() error = %v", err)
	}
	result := impactIn(t, snapshot, waypoint, primary)
	if len(result.Entries) == 0 {
		t.Fatal("expected entries for the removed route")
	}
	for _, e := range result.Entries {
		if e.Outcome != ClassificationUndetermined || e.Provisional == nil {
			t.Fatalf("entry = %+v, want undetermined with provisional outcome", e)
		}
	}
}
