package audit

import (
	"strings"
	"testing"

	"github.com/ninoamine/istio-route/internal/model"
)

func TestResultValidateIdentityAndCategory(t *testing.T) {
	tests := []struct {
		name    string
		result  Result
		wantErr string
	}{
		{
			name: "valid gateway identity and finding category",
			result: Result{
				Source: model.SourceSnapshot{Mode: model.SourceModeCluster},
				Gateway: Gateway{
					Identity: model.ResourceIdentity{
						Group: "gateway.networking.k8s.io", Version: "v1",
						Kind: "Gateway", Namespace: "infra", Name: "edge",
					},
					Class: model.GatewayClassRef{
						Identity: model.ResourceIdentity{
							Group: "gateway.networking.k8s.io", Version: "v1",
							Kind: "GatewayClass", Name: "istio",
						},
						Controller: "istio.io/gateway-controller",
					},
				},
				Findings: []model.Finding{{
					Code: "rejected-route", Severity: model.FindingSeverityError,
					Message: "route is not accepted", Category: string(CategoryOrphaned),
				}},
				EndpointCheck: EndpointCheck{State: EndpointCheckUnavailable},
			},
		},
		{
			name: "gateway identity is required",
			result: Result{
				Source:        model.SourceSnapshot{Mode: model.SourceModeCluster},
				EndpointCheck: EndpointCheck{State: EndpointCheckUnavailable},
			},
			wantErr: "gateway identity",
		},
		{
			name: "audit finding category is required",
			result: Result{
				Source:  model.SourceSnapshot{Mode: model.SourceModeCluster},
				Gateway: validGateway(),
				Findings: []model.Finding{{
					Code: "rejected-route", Severity: model.FindingSeverityError,
					Message: "route is not accepted",
				}},
				EndpointCheck: EndpointCheck{State: EndpointCheckUnavailable},
			},
			wantErr: "finding category",
		},
		{
			name: "unsupported audit finding category is rejected",
			result: Result{
				Source:  model.SourceSnapshot{Mode: model.SourceModeCluster},
				Gateway: validGateway(),
				Findings: []model.Finding{{
					Code: "rejected-route", Severity: model.FindingSeverityError,
					Message: "route is not accepted", Category: "routing",
				}},
				EndpointCheck: EndpointCheck{State: EndpointCheckUnavailable},
			},
			wantErr: "finding category",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.result.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func validGateway() Gateway {
	return Gateway{
		Identity: model.ResourceIdentity{
			Group: "gateway.networking.k8s.io", Version: "v1",
			Kind: "Gateway", Namespace: "infra", Name: "edge",
		},
		Class: model.GatewayClassRef{
			Identity: model.ResourceIdentity{
				Group: "gateway.networking.k8s.io", Version: "v1",
				Kind: "GatewayClass", Name: "istio",
			},
			Controller: "istio.io/gateway-controller",
		},
	}
}

func TestRouteStateValidate(t *testing.T) {
	tests := []struct {
		name  string
		state RouteState
		valid bool
	}{
		{name: "accepted", state: RouteStateAccepted, valid: true},
		{name: "rejected", state: RouteStateRejected, valid: true},
		{name: "unknown", state: RouteStateUnknown, valid: true},
		{name: "unsupported", state: "pending"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.state.Validate()
			if (err == nil) != tt.valid {
				t.Fatalf("Validate() error = %v, valid %t", err, tt.valid)
			}
		})
	}
}

func TestEndpointCheckStateValidate(t *testing.T) {
	tests := []struct {
		name  string
		state EndpointCheckState
		valid bool
	}{
		{name: "checked", state: EndpointCheckChecked, valid: true},
		{name: "skipped", state: EndpointCheckSkipped, valid: true},
		{name: "unavailable", state: EndpointCheckUnavailable, valid: true},
		{name: "unsupported", state: "not-run"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.state.Validate()
			if (err == nil) != tt.valid {
				t.Fatalf("Validate() error = %v, valid %t", err, tt.valid)
			}
		})
	}
}

func TestOutcomeValidate(t *testing.T) {
	tests := []struct {
		name    string
		outcome Outcome
		wantErr bool
	}{
		{name: "target found", outcome: OutcomeTargetFound},
		{name: "target not found", outcome: OutcomeTargetNotFound},
		{name: "invalid outcome", outcome: "empty", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.outcome.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}

func TestRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		request Request
		wantErr string
	}{
		{
			name: "valid cluster request",
			request: Request{
				GatewayName: "edge", Namespace: "infra",
				Source: model.SourceSnapshot{Mode: model.SourceModeCluster},
			},
		},
		{
			name: "invalid gateway name",
			request: Request{
				GatewayName: "not_a_gateway",
				Source:      model.SourceSnapshot{Mode: model.SourceModeCluster},
			},
			wantErr: "invalid gateway name",
		},
		{
			name: "invalid namespace",
			request: Request{
				GatewayName: "edge", Namespace: "BAD",
				Source: model.SourceSnapshot{Mode: model.SourceModeCluster},
			},
			wantErr: "invalid gateway namespace",
		},
		{
			name: "invalid source",
			request: Request{
				GatewayName: "edge", Source: model.SourceSnapshot{Mode: "other"},
			},
			wantErr: "source",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			assertError(t, err, tt.wantErr)
		})
	}
}

func TestGatewayValidate(t *testing.T) {
	programmed := &model.Condition{Type: "Programmed", Status: model.ConditionStatusTrue}
	tests := []struct {
		name    string
		gateway Gateway
		wantErr string
	}{
		{name: "valid", gateway: validGateway()},
		{
			name: "valid programmed condition",
			gateway: func() Gateway {
				gateway := validGateway()
				gateway.Programmed = programmed
				return gateway
			}(),
		},
		{
			name:    "missing identity",
			gateway: Gateway{},
			wantErr: "resource version",
		},
		{
			name: "wrong resource kind",
			gateway: func() Gateway {
				gateway := validGateway()
				gateway.Identity.Kind = "Service"
				return gateway
			}(),
			wantErr: "namespaced Gateway",
		},
		{
			name: "missing namespace",
			gateway: func() Gateway {
				gateway := validGateway()
				gateway.Identity.Namespace = ""
				return gateway
			}(),
			wantErr: "namespaced Gateway",
		},
		{
			name: "invalid class identity",
			gateway: Gateway{
				Identity: validGateway().Identity,
			},
			wantErr: "GatewayClass",
		},
		{
			name: "wrong class kind",
			gateway: func() Gateway {
				gateway := validGateway()
				gateway.Class.Identity.Kind = "Gateway"
				return gateway
			}(),
			wantErr: "kind must be GatewayClass",
		},
		{
			name: "missing class controller",
			gateway: func() Gateway {
				gateway := validGateway()
				gateway.Class.Controller = ""
				return gateway
			}(),
			wantErr: "controller",
		},
		{
			name: "invalid programmed condition",
			gateway: func() Gateway {
				gateway := validGateway()
				gateway.Programmed = &model.Condition{Type: "Accepted", Status: "invalid"}
				return gateway
			}(),
			wantErr: "programmed condition",
		},
		{
			name: "wrong programmed condition type",
			gateway: func() Gateway {
				gateway := validGateway()
				gateway.Programmed = &model.Condition{Type: "Accepted", Status: model.ConditionStatusTrue}
				return gateway
			}(),
			wantErr: "condition type must be Programmed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.gateway.Validate()
			assertError(t, err, tt.wantErr)
		})
	}
}

func TestListenerValidate(t *testing.T) {
	tests := []struct {
		name     string
		listener Listener
		wantErr  string
	}{
		{
			name: "valid",
			listener: Listener{
				Name: "https", Port: 443, Protocol: "HTTPS",
				Conditions: []model.Condition{{Type: "Accepted", Status: model.ConditionStatusTrue}},
			},
		},
		{name: "missing name", listener: Listener{Port: 443, Protocol: "HTTPS"}, wantErr: "name"},
		{name: "invalid port", listener: Listener{Name: "https", Port: 0, Protocol: "HTTPS"}, wantErr: "port"},
		{name: "missing protocol", listener: Listener{Name: "https", Port: 443}, wantErr: "protocol"},
		{
			name: "invalid condition",
			listener: Listener{
				Name: "https", Port: 443, Protocol: "HTTPS",
				Conditions: []model.Condition{{Type: "Accepted", Status: "invalid"}},
			},
			wantErr: "condition",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.listener.Validate()
			assertError(t, err, tt.wantErr)
		})
	}
}

func TestRouteAcceptanceValidate(t *testing.T) {
	tests := []struct {
		name       string
		acceptance RouteAcceptance
		wantErr    string
	}{
		{
			name: "valid accepted state",
			acceptance: RouteAcceptance{
				Parent: ParentReference{Identity: validGateway().Identity},
				State:  RouteStateAccepted,
			},
		},
		{
			name: "invalid parent identity",
			acceptance: RouteAcceptance{
				State: RouteStateAccepted,
			},
			wantErr: "parent reference",
		},
		{
			name: "invalid route state",
			acceptance: RouteAcceptance{
				Parent: ParentReference{Identity: validGateway().Identity},
				State:  "pending",
			},
			wantErr: "route state",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.acceptance.Validate()
			assertError(t, err, tt.wantErr)
		})
	}
}

func TestRouteValidate(t *testing.T) {
	tests := []struct {
		name    string
		route   Route
		wantErr string
	}{
		{
			name: "valid HTTP route",
			route: Route{
				Identity: model.ResourceIdentity{
					Group: "gateway.networking.k8s.io", Version: "v1",
					Kind: "HTTPRoute", Namespace: "shop", Name: "api",
				},
			},
		},
		{
			name: "valid GRPC route",
			route: Route{
				Identity: model.ResourceIdentity{
					Group: "gateway.networking.k8s.io", Version: "v1",
					Kind: "GRPCRoute", Namespace: "shop", Name: "api",
				},
			},
		},
		{name: "invalid identity", route: Route{}, wantErr: "version"},
		{
			name: "unsupported route kind",
			route: Route{
				Identity: model.ResourceIdentity{Version: "v1", Kind: "Service", Name: "api"},
			},
			wantErr: "unsupported route kind",
		},
		{
			name: "invalid parent reference",
			route: Route{
				Identity:   model.ResourceIdentity{Version: "v1", Kind: "HTTPRoute", Name: "api"},
				ParentRefs: []ParentReference{{}},
			},
			wantErr: "parent reference",
		},
		{
			name: "invalid acceptance",
			route: Route{
				Identity: model.ResourceIdentity{Version: "v1", Kind: "HTTPRoute", Name: "api"},
				Acceptance: []RouteAcceptance{{
					Parent: ParentReference{Identity: validGateway().Identity}, State: "pending",
				}},
			},
			wantErr: "acceptance",
		},
		{
			name: "invalid rule",
			route: Route{
				Identity: model.ResourceIdentity{Version: "v1", Kind: "HTTPRoute", Name: "api"},
				Rules:    []Rule{{}},
			},
			wantErr: "rule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.route.Validate()
			assertError(t, err, tt.wantErr)
		})
	}
}

func TestRuleValidate(t *testing.T) {
	tests := []struct {
		name    string
		rule    Rule
		wantErr string
	}{
		{name: "valid", rule: Rule{Index: 1, Precedence: PrecedenceWinner}},
		{name: "non-positive index", rule: Rule{Precedence: PrecedenceWinner}, wantErr: "rule index"},
		{name: "invalid precedence", rule: Rule{Index: 1}, wantErr: "precedence state"},
		{
			name: "invalid backend",
			rule: Rule{
				Index: 1, Precedence: PrecedenceWinner,
				Backends: []Backend{{}},
			},
			wantErr: "backend",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rule.Validate()
			assertError(t, err, tt.wantErr)
		})
	}
}

func TestBackendValidate(t *testing.T) {
	service := &BackendService{
		Identity: model.ResourceIdentity{
			Version: "v1", Kind: "Service", Namespace: "shop", Name: "api",
		},
		Availability: model.AvailabilityPresent,
	}
	port := int32(8080)
	tests := []struct {
		name    string
		backend Backend
		wantErr string
	}{
		{
			name: "valid reference and resolved service",
			backend: Backend{
				Reference: BackendReference{Namespace: "shop", Name: "api", Port: &port},
				Service:   service,
			},
		},
		{name: "missing name", backend: Backend{Reference: BackendReference{Namespace: "shop"}}, wantErr: "name"},
		{name: "missing namespace", backend: Backend{Reference: BackendReference{Name: "api"}}, wantErr: "namespace"},
		{
			name: "invalid name",
			backend: Backend{
				Reference: BackendReference{Namespace: "shop", Name: "not_valid"},
			},
			wantErr: "invalid backend name",
		},
		{
			name: "invalid namespace",
			backend: Backend{
				Reference: BackendReference{Namespace: "BAD", Name: "api"},
			},
			wantErr: "invalid backend namespace",
		},
		{
			name: "invalid backend port",
			backend: Backend{
				Reference: BackendReference{Namespace: "shop", Name: "api", Port: int32Pointer(0)},
			},
			wantErr: "backend port",
		},
		{
			name: "invalid resolved service",
			backend: Backend{
				Reference: BackendReference{Namespace: "shop", Name: "api"},
				Service:   &BackendService{},
			},
			wantErr: "service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.backend.Validate()
			assertError(t, err, tt.wantErr)
		})
	}
}

func TestBackendServiceValidate(t *testing.T) {
	valid := BackendService{
		Identity: model.ResourceIdentity{
			Version: "v1", Kind: "Service", Namespace: "shop", Name: "api",
		},
		Availability: model.AvailabilityPresent,
		Ports:        []int32{8080},
	}
	tests := []struct {
		name    string
		service BackendService
		wantErr string
	}{
		{name: "valid", service: valid},
		{name: "invalid identity", service: BackendService{}, wantErr: "version"},
		{
			name: "wrong identity kind",
			service: BackendService{
				Identity:     model.ResourceIdentity{Version: "v1", Kind: "Pod", Name: "api"},
				Availability: model.AvailabilityPresent,
			},
			wantErr: "kind must be Service",
		},
		{
			name: "invalid availability",
			service: BackendService{
				Identity: valid.Identity, Availability: "unknown",
			},
			wantErr: "availability",
		},
		{
			name: "negative ready count",
			service: BackendService{
				Identity: valid.Identity, Availability: model.AvailabilityPresent,
				ReadyAddressCount: -1,
			},
			wantErr: "ready address count",
		},
		{
			name: "invalid port",
			service: BackendService{
				Identity: valid.Identity, Availability: model.AvailabilityPresent,
				Ports: []int32{0},
			},
			wantErr: "service port",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.service.Validate()
			assertError(t, err, tt.wantErr)
		})
	}
}

func TestWaypointSelectionValidate(t *testing.T) {
	valid := WaypointSelection{
		Resource: model.ResourceIdentity{
			Version: "v1", Kind: "Namespace", Name: "shop",
		},
		LabelValue:        "waypoint",
		WaypointNamespace: "infra",
		WaypointName:      "waypoint",
		TargetFound:       true,
		Programmed:        &model.Condition{Type: "Programmed", Status: model.ConditionStatusTrue},
	}
	tests := []struct {
		name      string
		selection WaypointSelection
		wantErr   string
	}{
		{name: "valid namespace selection", selection: valid},
		{
			name: "valid service selection",
			selection: func() WaypointSelection {
				selection := valid
				selection.Resource = model.ResourceIdentity{
					Version: "v1", Kind: "Service", Namespace: "shop", Name: "api",
				}
				return selection
			}(),
		},
		{name: "invalid selecting resource", selection: WaypointSelection{}, wantErr: "selecting resource"},
		{
			name: "unsupported selecting resource kind",
			selection: WaypointSelection{
				Resource: model.ResourceIdentity{Version: "v1", Kind: "Pod", Name: "api"},
			},
			wantErr: "selector resource kind",
		},
		{
			name: "missing target identity",
			selection: WaypointSelection{
				Resource: model.ResourceIdentity{Version: "v1", Kind: "Namespace", Name: "shop"},
			},
			wantErr: "waypoint namespace and name",
		},
		{
			name: "invalid target namespace",
			selection: WaypointSelection{
				Resource:          model.ResourceIdentity{Version: "v1", Kind: "Namespace", Name: "shop"},
				WaypointNamespace: "BAD", WaypointName: "waypoint",
			},
			wantErr: "invalid waypoint namespace",
		},
		{
			name: "invalid target name",
			selection: WaypointSelection{
				Resource:          model.ResourceIdentity{Version: "v1", Kind: "Namespace", Name: "shop"},
				WaypointNamespace: "infra", WaypointName: "not_valid",
			},
			wantErr: "invalid waypoint name",
		},
		{
			name: "invalid programmed condition",
			selection: WaypointSelection{
				Resource:          model.ResourceIdentity{Version: "v1", Kind: "Namespace", Name: "shop"},
				WaypointNamespace: "infra", WaypointName: "waypoint",
				Programmed: &model.Condition{Type: "Programmed", Status: "invalid"},
			},
			wantErr: "programmed condition",
		},
		{
			name: "wrong programmed condition type",
			selection: WaypointSelection{
				Resource:          model.ResourceIdentity{Version: "v1", Kind: "Namespace", Name: "shop"},
				WaypointNamespace: "infra", WaypointName: "waypoint",
				Programmed: &model.Condition{Type: "Accepted", Status: model.ConditionStatusTrue},
			},
			wantErr: "condition type must be Programmed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.selection.Validate()
			assertError(t, err, tt.wantErr)
		})
	}
}

func TestEndpointCheckValidate(t *testing.T) {
	tests := []struct {
		name  string
		check EndpointCheck
		valid bool
	}{
		{name: "checked", check: EndpointCheck{State: EndpointCheckChecked}, valid: true},
		{name: "skipped", check: EndpointCheck{State: EndpointCheckSkipped}, valid: true},
		{name: "unavailable", check: EndpointCheck{State: EndpointCheckUnavailable}, valid: true},
		{name: "invalid", check: EndpointCheck{State: "not-run"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.check.Validate()
			if (err == nil) != tt.valid {
				t.Fatalf("Validate() error = %v, valid %t", err, tt.valid)
			}
		})
	}
}

func assertError(t *testing.T, err error, wantSubstring string) {
	t.Helper()
	if wantSubstring == "" {
		if err != nil {
			t.Fatalf("Validate() error = %v, want nil", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), wantSubstring) {
		t.Fatalf("Validate() error = %v, want substring %q", err, wantSubstring)
	}
}

func int32Pointer(value int32) *int32 {
	return &value
}
