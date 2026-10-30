package behavior

import (
	"strings"
	"testing"

	"github.com/ninoamine/istio-route/internal/model"
)

func TestScenarioAssertions(t *testing.T) {
	port := int32(8080)
	weight90, weight10 := int32(90), int32(10)
	analysis := model.Analysis{
		Hostname: "shop.example.com",
		EntryPoints: []model.EntryPoint{{
			Name: "https",
			Resource: model.ResourceIdentity{
				Kind: "Gateway", Namespace: "shop", Name: "public-gateway",
			},
			Protocol: "HTTPS",
		}},
		Routes: []model.Route{{
			Resource: model.ResourceIdentity{
				Kind: "VirtualService", Namespace: "shop", Name: "shop",
			},
			Rules: []model.Rule{{
				Index:   1,
				Matches: []model.Match{{PathType: "Prefix", Path: "/api"}},
				Destinations: []model.Destination{
					{Kind: model.DestinationKindService, Name: "api", Namespace: "shop", Subset: "v1", Port: &port, Weight: &weight90},
					{Kind: model.DestinationKindService, Name: "api", Namespace: "shop", Subset: "v2", Port: &port, Weight: &weight10},
				},
			}},
		}},
		Findings: []model.Finding{{
			Severity: model.FindingSeverityWarning,
			Message:  "destination subset is not defined",
		}},
	}
	state := &scenarioState{analysis: analysis, traced: true}

	tests := []struct {
		name string
		run  func() error
	}{
		{name: "hostname found", run: func() error { return state.assertHostnameFound(true) }},
		{name: "route matches", run: func() error { return state.assertRouteMatch(true) }},
		{name: "gateway identity", run: func() error { return state.assertGateway("public-gateway") }},
		{name: "route identity", run: func() error { return state.assertRoute(1, "VirtualService", "shop/shop") }},
		{name: "ordered rule path match", run: func() error { return state.assertPathPrefix(1, 1, "/api") }},
		{name: "destination service and port", run: func() error { return state.assertDestination(1, 1, "api", 8080) }},
		{name: "weighted traffic split", run: func() error {
			return state.assertTrafficSplit(1, 1, []weightedDestination{{subset: "v1", percent: 90}, {subset: "v2", percent: 10}})
		}},
		{name: "finding severity and message", run: func() error { return state.assertFinding(model.FindingSeverityWarning, "destination subset") }},
		{name: "absence of error findings", run: func() error { return state.assertNoFindings(model.FindingSeverityError) }},
		{name: "hostname not found", run: func() error {
			state.analysis.Routes = nil
			return state.assertHostnameFound(false)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err != nil {
				t.Fatalf("assertion failed: %v", err)
			}
		})
	}
}

func TestScenarioAssertionFailureIncludesExpectedAndActual(t *testing.T) {
	state := &scenarioState{traced: true}
	err := state.assertGateway("expected-gateway")
	if err == nil || err.Error() != `expected gateway "expected-gateway", got no matching entry point` {
		t.Fatalf("assertGateway() error = %v, want expected/actual diagnostic", err)
	}
}

func TestScenarioAssertionRequiresTrace(t *testing.T) {
	state := &scenarioState{}
	if err := state.assertRouteMatch(true); err == nil || !strings.Contains(err.Error(), "before this assertion") {
		t.Fatalf("assertRouteMatch() error = %v, want missing trace diagnostic", err)
	}
}

func TestTrafficSplitRoundsPercentages(t *testing.T) {
	firstWeight, secondWeight := int32(2), int32(1)
	state := &scenarioState{
		traced: true,
		analysis: model.Analysis{
			Routes: []model.Route{{
				Rules: []model.Rule{{
					Destinations: []model.Destination{
						{Subset: "v1", Weight: &firstWeight},
						{Subset: "v2", Weight: &secondWeight},
					},
				}},
			}},
		},
	}
	if err := state.assertTrafficSplit(1, 1, []weightedDestination{
		{subset: "v1", percent: 67},
		{subset: "v2", percent: 33},
	}); err != nil {
		t.Fatalf("assertTrafficSplit() error = %v", err)
	}
}

func TestSuggestStep(t *testing.T) {
	got := suggestStep(`When I trace the host "shop.example.com"`)
	if got != `When I trace the hostname "shop.example.com"` {
		t.Fatalf("suggestStep() = %q, want closest supported form", got)
	}
}
