package audit

import (
	"testing"

	"github.com/ninoamine/istio-route/internal/model"
)

func TestOrphanedFindings(t *testing.T) {
	tests := []struct {
		name       string
		result     Result
		wantCodes  []string
		wantStates []model.FindingSeverity
	}{
		{
			name: "explicit rejection is an error",
			result: Result{Routes: []Route{testRoute("HTTPRoute", "rejected", RouteAcceptance{
				Parent: ParentReference{Identity: testGatewayIdentity()},
				State:  RouteStateRejected, Reason: "NotAllowedByListeners",
			})}},
			wantCodes: []string{"rejected-route"}, wantStates: []model.FindingSeverity{model.FindingSeverityError},
		},
		{
			name: "missing Accepted status stays unknown",
			result: Result{Routes: []Route{testRoute("HTTPRoute", "pending", RouteAcceptance{
				Parent: ParentReference{Identity: testGatewayIdentity()}, State: RouteStateUnknown,
			})}},
		},
		{
			name: "missing backend Service",
			result: Result{Routes: []Route{testRouteWithBackend(Backend{
				Reference: BackendReference{Namespace: "shop", Name: "missing", Kind: "Service"},
				Service: &BackendService{
					Identity:     testIdentity("Service", "shop", "missing"),
					Availability: model.AvailabilityNotFound,
				},
			})}},
			wantCodes:  []string{"backend-service-not-found"},
			wantStates: []model.FindingSeverity{model.FindingSeverityError},
		},
		{
			name: "missing backend port",
			result: Result{Routes: []Route{testRouteWithBackend(Backend{
				Reference: BackendReference{Namespace: "shop", Name: "api", Kind: "Service", Port: int32Ref(9090)},
				Service: &BackendService{
					Identity:     testIdentity("Service", "shop", "api"),
					Availability: model.AvailabilityPresent,
					PortPresent:  boolRef(false),
				},
			})}},
			wantCodes:  []string{"backend-port-not-found"},
			wantStates: []model.FindingSeverity{model.FindingSeverityError},
		},
		{
			name: "missing cross-namespace grant",
			result: Result{Routes: []Route{testRouteWithBackend(Backend{
				Reference: BackendReference{Namespace: "platform", Name: "api", Kind: "Service"},
				Grant:     &ReferenceGrantUse{},
			})}},
			wantCodes:  []string{"missing-reference-grant"},
			wantStates: []model.FindingSeverity{model.FindingSeverityWarning},
		},
		{
			name: "existing grant that does not authorize the reference",
			result: Result{Routes: []Route{testRouteWithBackend(Backend{
				Reference: BackendReference{Namespace: "platform", Name: "api", Kind: "Service"},
				Grant: &ReferenceGrantUse{Grants: []ReferenceGrant{{
					Identity: testIdentity("ReferenceGrant", "platform", "deny"),
				}}},
			})}},
			wantCodes:  []string{"missing-reference-grant"},
			wantStates: []model.FindingSeverity{model.FindingSeverityWarning},
		},
		{
			name: "unused matching ReferenceGrant",
			result: Result{Routes: []Route{testRouteWithBackend(Backend{
				Reference: BackendReference{Namespace: "platform", Name: "api", Kind: "Service"},
				Grant: &ReferenceGrantUse{Grants: []ReferenceGrant{{
					Identity: testIdentity("ReferenceGrant", "platform", "unused"),
					From:     []GrantConstraint{{Group: testGatewayGroup, Kind: "HTTPRoute", Namespace: "shop"}},
					To:       []GrantConstraint{{Kind: "Service", Name: "other"}},
				}}},
			})}},
			wantCodes:  []string{"missing-reference-grant", "unused-reference-grant"},
			wantStates: []model.FindingSeverity{model.FindingSeverityWarning, model.FindingSeverityWarning},
		},
		{
			name: "missing waypoint",
			result: Result{WaypointSelections: []WaypointSelection{{
				Resource: testIdentity("Namespace", "", "shop"), LabelValue: "mesh/waypoint",
				WaypointNamespace: "mesh", WaypointName: "waypoint",
			}}},
			wantCodes:  []string{"waypoint-not-found"},
			wantStates: []model.FindingSeverity{model.FindingSeverityError},
		},
		{
			name: "unprogrammed waypoint",
			result: Result{WaypointSelections: []WaypointSelection{{
				Resource: testIdentity("Namespace", "", "shop"), LabelValue: "waypoint",
				WaypointNamespace: "shop", WaypointName: "waypoint", TargetFound: true,
				Programmed: &model.Condition{Type: "Programmed", Status: model.ConditionStatusFalse},
			}}},
			wantCodes:  []string{"waypoint-not-programmed"},
			wantStates: []model.FindingSeverity{model.FindingSeverityError},
		},
		{
			name: "programmed waypoint has no orphan finding",
			result: Result{WaypointSelections: []WaypointSelection{{
				Resource: testIdentity("Namespace", "", "shop"), LabelValue: "waypoint",
				WaypointNamespace: "shop", WaypointName: "waypoint", TargetFound: true,
				Programmed: &model.Condition{Type: "Programmed", Status: model.ConditionStatusTrue},
			}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := orphanedFindings(tt.result)
			if len(findings) != len(tt.wantCodes) {
				t.Fatalf("orphanedFindings() returned %d findings, want %d: %#v", len(findings), len(tt.wantCodes), findings)
			}
			for i, finding := range findings {
				if finding.Code != tt.wantCodes[i] {
					t.Errorf("finding %d code = %q, want %q", i, finding.Code, tt.wantCodes[i])
				}
				if finding.Category != string(CategoryOrphaned) {
					t.Errorf("finding %d category = %q, want %q", i, finding.Category, CategoryOrphaned)
				}
				if len(tt.wantStates) > i && finding.Severity != tt.wantStates[i] {
					t.Errorf("finding %d severity = %q, want %q", i, finding.Severity, tt.wantStates[i])
				}
				if finding.Message == "" || finding.SuggestedFix == "" {
					t.Errorf("finding %d must include explanation and suggested fix: %#v", i, finding)
				}
				if err := finding.Validate(); err != nil {
					t.Errorf("finding %d is invalid: %v", i, err)
				}
			}
		})
	}
}

func testRoute(kind, name string, acceptances ...RouteAcceptance) Route {
	return Route{
		Identity:   testIdentity(kind, "shop", name),
		Acceptance: acceptances,
	}
}

func testRouteWithBackend(backend Backend) Route {
	return Route{
		Identity: testIdentity("HTTPRoute", "shop", "api"),
		Rules:    []Rule{{Index: 1, Precedence: PrecedenceWinner, Backends: []Backend{backend}}},
	}
}

func testGatewayIdentity() model.ResourceIdentity {
	return model.ResourceIdentity{Group: testGatewayGroup, Version: "v1", Kind: "Gateway", Namespace: "infra", Name: "edge"}
}

func testIdentity(kind, namespace, name string) model.ResourceIdentity {
	group, version := "", "v1"
	if kind == "Gateway" || kind == "GatewayClass" || kind == "HTTPRoute" || kind == "GRPCRoute" {
		group = testGatewayGroup
	}
	if kind == "ReferenceGrant" {
		group, version = testGatewayGroup, "v1beta1"
	}
	return model.ResourceIdentity{Group: group, Version: version, Kind: kind, Namespace: namespace, Name: name}
}

const testGatewayGroup = "gateway.networking.k8s.io"

func int32Ref(value int32) *int32 {
	return &value
}

func boolRef(value bool) *bool {
	return &value
}
