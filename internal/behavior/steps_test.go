package behavior

import (
	"context"
	"strings"
	"testing"

	"github.com/ninoamine/istio-route/internal/audit"
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

func TestScenarioAssertionFailuresIncludeActualData(t *testing.T) {
	port := int32(8080)
	state := &scenarioState{
		traced: true,
		analysis: model.Analysis{
			EntryPoints: []model.EntryPoint{{Name: "https", Resource: model.ResourceIdentity{Name: "edge"}}},
			Routes: []model.Route{{
				Resource: model.ResourceIdentity{Kind: "VirtualService", Namespace: "shop", Name: "shop"},
				Rules: []model.Rule{{
					Matches: []model.Match{{PathType: "Exact", Path: "/api"}},
					Destinations: []model.Destination{{
						Kind: model.DestinationKindService, Namespace: "shop", Name: "web", Port: &port, Subset: "v1",
					}},
				}},
			}},
			Findings: []model.Finding{{Severity: model.FindingSeverityWarning, Message: "degraded"}},
		},
	}

	tests := []struct {
		name string
		run  func() error
		want string
	}{
		{name: "unexpected route", run: func() error { return state.assertRouteMatch(false) }, want: "VirtualService shop/shop"},
		{name: "gateway mismatch", run: func() error { return state.assertGateway("public") }, want: "edge/https"},
		{name: "route mismatch", run: func() error { return state.assertRoute(1, "HTTPRoute", "shop/other") }, want: "VirtualService shop/shop"},
		{name: "path mismatch", run: func() error { return state.assertPathPrefix(1, 1, "/api") }, want: `Exact "/api"`},
		{name: "destination mismatch", run: func() error { return state.assertDestination(1, 1, "api", 8080) }, want: "service shop/web:8080 subset v1"},
		{name: "missing route", run: func() error { return state.assertRoute(2, "VirtualService", "shop/other") }, want: "VirtualService shop/shop"},
		{name: "missing rule", run: func() error { return state.assertPathPrefix(1, 2, "/api") }, want: "rule 2 of route 1 is absent"},
		{name: "traffic split mismatch", run: func() error {
			return state.assertTrafficSplit(1, 1, []weightedDestination{{subset: "v2", percent: 100}})
		}, want: "100% \"v2\""},
		{name: "finding severity mismatch", run: func() error { return state.assertNoFindings(model.FindingSeverityWarning) }, want: "warning: degraded"},
		{name: "missing finding", run: func() error { return state.assertFinding(model.FindingSeverityError, "missing") }, want: "warning: degraded"},
		{name: "invalid field", run: func() error { return state.assertField("trace", "invalid field", "value") }, want: "invalid trace field"},
		{name: "missing expected field", run: func() error { return state.assertField("trace", "missing", "value") }, want: "<absent>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("assertion error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestScenarioStructuredFieldAssertions(t *testing.T) {
	ready := true
	state := &scenarioState{
		traced: true,
		analysis: model.Analysis{
			Hostname:    "shop.example.com",
			EntryPoints: []model.EntryPoint{{Protocol: "HTTPS"}},
			Routes: []model.Route{{
				Resource: model.ResourceIdentity{Kind: "VirtualService"},
				Rules: []model.Rule{{
					Matches: []model.Match{{Path: "/api"}},
					Destinations: []model.Destination{{
						Subset:  "v1",
						Service: &model.Service{Endpoints: []model.Endpoint{{Ready: &ready}}},
					}},
				}},
			}},
			Findings: []model.Finding{{Severity: model.FindingSeverityWarning}},
		},
	}

	tests := []struct {
		name string
		run  func() error
	}{
		{name: "trace field", run: func() error { return state.assertField("trace", "hostname", "shop.example.com") }},
		{name: "entry point field", run: func() error { return state.assertEntryPointField(1, "protocol", "HTTPS") }},
		{name: "route field", run: func() error { return state.assertRouteField(1, "resource.kind", "VirtualService") }},
		{name: "rule field", run: func() error { return state.assertRuleField(1, 1, "matches[0].path", "/api") }},
		{name: "destination field", run: func() error { return state.assertDestinationField(1, 1, 1, "subset", "v1") }},
		{name: "endpoint field", run: func() error { return state.assertEndpointField(1, 1, 1, 1, "ready", "true") }},
		{name: "finding field", run: func() error { return state.assertFindingField(1, "severity", "warning") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err != nil {
				t.Fatalf("field assertion failed: %v", err)
			}
		})
	}

	for _, run := range []func() error{
		func() error { return state.assertEntryPointField(0, "protocol", "HTTPS") },
		func() error { return state.assertRouteField(0, "resource.kind", "VirtualService") },
		func() error { return state.assertFindingField(0, "severity", "warning") },
		func() error { return state.assertRuleField(0, 1, "matches[0].path", "/api") },
		func() error { return state.assertDestinationField(0, 1, 1, "subset", "v1") },
		func() error { return state.assertEndpointField(0, 1, 1, 1, "ready", "true") },
	} {
		if err := run(); err == nil {
			t.Error("field assertion with an invalid index unexpectedly passed")
		}
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

func TestScenarioAuditAssertions(t *testing.T) {
	gateway := model.ResourceIdentity{Kind: "Gateway", Namespace: "infra", Name: "edge"}
	route := model.ResourceIdentity{Kind: "HTTPRoute", Namespace: "shop", Name: "old-route"}
	service := model.ResourceIdentity{Kind: "Service", Namespace: "shop", Name: "api"}
	result := audit.Result{
		Gateway: audit.Gateway{Identity: gateway},
		Routes: []audit.Route{{
			Identity: route,
			Rules:    []audit.Rule{{Index: 1, Precedence: audit.PrecedenceShadowed}},
		}, {
			Identity: model.ResourceIdentity{Kind: "HTTPRoute", Namespace: "shop", Name: "general"},
			Rules:    []audit.Rule{{Index: 1, Precedence: audit.PrecedencePartialOverlap}},
		}, {
			Identity: model.ResourceIdentity{Kind: "HTTPRoute", Namespace: "shop", Name: "specific"},
			Rules:    []audit.Rule{{Index: 1, Precedence: audit.PrecedenceWinner}},
		}},
		Findings: []model.Finding{{
			Code: "backend-service-not-found", Severity: model.FindingSeverityError,
			Category: string(audit.CategoryOrphaned), Resources: []model.ResourceIdentity{route, service},
		}},
	}
	state := &scenarioState{
		auditFunc: func(_ context.Context, namespace, name string) (audit.Result, audit.Outcome, error) {
			if namespace != "infra" || name != "edge" {
				t.Fatalf("audit callback identity = %s/%s, want infra/edge", namespace, name)
			}
			return result, audit.OutcomeTargetFound, nil
		},
		analysis: model.Analysis{Hostname: "untouched.example.com"},
		traced:   true,
	}
	if err := state.auditGateway(context.Background(), "infra/edge"); err != nil {
		t.Fatalf("auditGateway() error = %v", err)
	}
	if state.analysis.Hostname != "untouched.example.com" || !state.traced {
		t.Fatal("gateway audit changed hostname trace state")
	}

	tests := []struct {
		name string
		run  func() error
	}{
		{name: "gateway found", run: func() error { return state.assertGatewayFound(true) }},
		{name: "finding fields", run: func() error {
			return state.assertAuditFinding(
				"backend-service-not-found", model.FindingSeverityError, audit.CategoryOrphaned,
			)
		}},
		{name: "finding resource", run: func() error {
			return state.assertAuditFindingResource("backend-service-not-found", "Service", "shop/api")
		}},
		{name: "finding absence", run: func() error { return state.assertNoAuditFinding("duplicate-route-rule") }},
		{name: "shadowed rule", run: func() error {
			return state.assertRoutePrecedence("shop/old-route", 1, audit.PrecedenceShadowed)
		}},
		{name: "partial-overlap rule", run: func() error {
			return state.assertPartialOverlap("shop/general", 1, "shop/specific", 1)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err != nil {
				t.Fatalf("audit assertion failed: %v", err)
			}
		})
	}
}

func TestScenarioAuditNotFoundAndFailure(t *testing.T) {
	state := &scenarioState{
		auditFunc: func(context.Context, string, string) (audit.Result, audit.Outcome, error) {
			return audit.Result{}, audit.OutcomeTargetNotFound, nil
		},
	}
	if err := state.auditGateway(context.Background(), "infra/absent"); err != nil {
		t.Fatalf("auditGateway() error = %v", err)
	}
	if err := state.assertGatewayFound(false); err != nil {
		t.Fatalf("assertGatewayFound(false) error = %v", err)
	}
	if err := state.assertNoAuditFinding("waypoint-not-found"); err != nil {
		t.Fatalf("assertNoAuditFinding() error = %v", err)
	}

	if err := state.assertGatewayFound(true); err == nil || !strings.Contains(err.Error(), "gateway found") {
		t.Fatalf("assertGatewayFound(true) error = %v, want assertion failure", err)
	}
}
