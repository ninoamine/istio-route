package audit

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ninoamine/istio-route/internal/source"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestAnalyzeGatewayFixtures(t *testing.T) {
	tests := []struct {
		name           string
		fixture        string
		gatewayName    string
		namespace      string
		wantCodes      []string
		wantState      RouteState
		wantSelections int
	}{
		{
			name: "healthy Gateway", fixture: "gateway-audit-healthy.yaml", gatewayName: "edge", namespace: "infra",
			wantState: RouteStateAccepted,
		},
		{
			name: "rejected route", fixture: "gateway-audit-rejected.yaml", gatewayName: "edge", namespace: "infra",
			wantCodes: []string{"rejected-route"}, wantState: RouteStateRejected,
		},
		{
			name: "missing backend", fixture: "gateway-audit-missing-backend.yaml", gatewayName: "edge", namespace: "infra",
			wantCodes: []string{"backend-service-not-found"},
		},
		{
			name: "duplicate route rules", fixture: "gateway-audit-duplicate.yaml", gatewayName: "edge", namespace: "infra",
			wantCodes: []string{"duplicate-route-rule", "shadowed-route-rule", "winning-route-rule"},
		},
		{
			name: "unused ReferenceGrant", fixture: "gateway-audit-unused-grant.yaml", gatewayName: "edge", namespace: "infra",
			wantCodes: []string{"missing-reference-grant", "unused-reference-grant"},
		},
		{
			name: "waypoint without Programmed status", fixture: "gateway-audit-missing-waypoint.yaml", gatewayName: "waypoint", namespace: "app",
			wantCodes: []string{"waypoint-not-programmed"}, wantSelections: 1,
		},
		{
			name: "healthy selected waypoint", fixture: "gateway-audit-waypoint.yaml", gatewayName: "waypoint", namespace: "app",
			wantSelections: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := loadGatewayFixture(t, tt.fixture)
			result, outcome, err := Analyze(context.Background(), Request{
				GatewayName: tt.gatewayName, Namespace: tt.namespace, Source: snapshot.Metadata,
			}, snapshot)
			if err != nil {
				t.Fatalf("Analyze() error = %v", err)
			}
			if outcome != OutcomeTargetFound {
				t.Fatalf("Analyze() outcome = %q, want %q", outcome, OutcomeTargetFound)
			}
			if len(result.Findings) != len(tt.wantCodes) {
				t.Fatalf("Analyze() findings = %#v, want codes %#v", result.Findings, tt.wantCodes)
			}
			for i, code := range tt.wantCodes {
				if result.Findings[i].Code != code {
					t.Errorf("finding %d code = %q, want %q", i, result.Findings[i].Code, code)
				}
			}
			if len(result.Routes) > 0 && tt.wantState != "" && result.Routes[0].Acceptance[0].State != tt.wantState {
				t.Errorf("route acceptance = %q, want %q", result.Routes[0].Acceptance[0].State, tt.wantState)
			}
			if len(result.WaypointSelections) != tt.wantSelections {
				t.Errorf("waypoint selections = %d, want %d", len(result.WaypointSelections), tt.wantSelections)
			}
			if tt.fixture == "gateway-audit-healthy.yaml" && result.EndpointCheck.State != EndpointCheckSkipped {
				t.Errorf("endpoint check = %q, want skipped", result.EndpointCheck.State)
			}
		})
	}
}

func TestAnalyzeTargetOutcomes(t *testing.T) {
	snapshot := loadGatewayFixture(t, "gateway-audit-healthy.yaml")
	result, outcome, err := Analyze(context.Background(), Request{
		GatewayName: "absent", Namespace: "infra", Source: snapshot.Metadata,
	}, snapshot)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if outcome != OutcomeTargetNotFound || len(result.Findings) != 0 {
		t.Fatalf("Analyze() = %#v, %q, want absent target", result, outcome)
	}

	gateway := snapshot.Resources[1]
	gatewayObject := gateway.Object.(*gatewayv1.Gateway).DeepCopy()
	gatewayObject.Namespace = "other"
	gateway.Identity.Namespace = "other"
	gateway.Object = gatewayObject
	snapshot.Resources = append(snapshot.Resources, gateway)
	_, _, err = Analyze(context.Background(), Request{
		GatewayName: "edge", Source: snapshot.Metadata,
	}, snapshot)
	if !errors.Is(err, ErrAmbiguousGateway) {
		t.Fatalf("Analyze() error = %v, want ErrAmbiguousGateway", err)
	}
}

func TestAnalyzeMissingWaypointSelection(t *testing.T) {
	snapshot := loadGatewayFixture(t, "gateway-audit-missing-waypoint-target.yaml")
	result, outcome, err := Analyze(context.Background(), Request{
		GatewayName: "missing-waypoint", Namespace: "app", Source: snapshot.Metadata,
	}, snapshot)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if outcome != OutcomeTargetNotFound {
		t.Fatalf("Analyze() outcome = %q, want %q", outcome, OutcomeTargetNotFound)
	}
	if len(result.Findings) != 1 || result.Findings[0].Code != "waypoint-not-found" {
		t.Fatalf("Analyze() findings = %#v, want one waypoint-not-found finding", result.Findings)
	}
	if len(result.Findings[0].Resources) != 1 ||
		result.Findings[0].Resources[0].Kind != "Namespace" ||
		result.Findings[0].Resources[0].Name != "app" {
		t.Fatalf("waypoint finding resources = %#v, want Namespace app", result.Findings[0].Resources)
	}
}

func TestAnalyzeMissingAcceptedStatusRemainsUnknown(t *testing.T) {
	snapshot := loadGatewayFixture(t, "gateway-audit-healthy.yaml")
	for i := range snapshot.Resources {
		route, ok := snapshot.Resources[i].Object.(*gatewayv1.HTTPRoute)
		if !ok {
			continue
		}
		route.Status.Parents[0].Conditions = nil
	}
	result, _, err := Analyze(context.Background(), Request{
		GatewayName: "edge", Namespace: "infra", Source: snapshot.Metadata,
	}, snapshot)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.Routes[0].Acceptance[0].State != RouteStateUnknown {
		t.Fatalf("route acceptance = %q, want unknown", result.Routes[0].Acceptance[0].State)
	}
	for _, finding := range result.Findings {
		if finding.Code == "rejected-route" {
			t.Fatal("missing Accepted condition was incorrectly reported as rejection")
		}
	}
}

func loadGatewayFixture(t *testing.T, name string) source.Snapshot {
	t.Helper()
	input := filepath.Join("..", "..", "testdata", "manifests", name)
	snapshot, err := (&source.FilesSource{Files: []string{input}}).Snapshot(context.Background())
	if err != nil {
		t.Fatalf("load fixture %q: %v", name, err)
	}
	return snapshot
}
