package diagnose

import (
	"strings"
	"testing"

	"github.com/ninoamine/istio-route/internal/model"
	"github.com/ninoamine/istio-route/internal/source"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func TestDiagnoseGatewayAndHTTPRouteConditions(t *testing.T) {
	tests := []struct {
		name        string
		resources   []source.Resource
		entryPoints []model.EntryPoint
		routes      []model.Route
		wantStatus  model.ConditionStatus
		wantType    string
		wantReason  string
		wantCode    string
		wantMessage string
	}{
		{
			name: "HTTPRoute rejected",
			resources: []source.Resource{
				testGatewayResource(testGatewayWithListener("infra", "edge", "shop.example.com", "https")),
				testHTTPRouteResource(testRouteWithConditions(string(gatewayv1.RouteConditionAccepted), metav1.ConditionFalse, "NotAllowedByListeners", "")),
			},
			wantStatus:  model.ConditionStatusFalse,
			wantType:    string(gatewayv1.RouteConditionAccepted),
			wantReason:  "NotAllowedByListeners",
			wantCode:    "route-not-accepted",
			wantMessage: "listener does not allow this HTTPRoute",
		},
		{
			name: "HTTPRoute references unresolved",
			resources: []source.Resource{
				testGatewayResource(testGatewayWithListener("infra", "edge", "shop.example.com", "https")),
				testHTTPRouteResource(testRouteWithConditions(string(gatewayv1.RouteConditionResolvedRefs), metav1.ConditionFalse, "BackendNotFound", "")),
			},
			wantStatus:  model.ConditionStatusFalse,
			wantType:    string(gatewayv1.RouteConditionResolvedRefs),
			wantReason:  "BackendNotFound",
			wantCode:    "route-references-unresolved",
			wantMessage: "backend reference could not be resolved",
		},
		{
			name: "Gateway not programmed",
			resources: []source.Resource{
				testGatewayResource(testGatewayWithCondition("infra", "edge", "shop.example.com", metav1.Condition{
					Type: string(gatewayv1.GatewayConditionProgrammed), Status: metav1.ConditionFalse,
					Reason: "Invalid", Message: "listener configuration is invalid",
				})),
				testHTTPRouteResource(testRoute()),
			},
			entryPoints: []model.EntryPoint{testGatewayEntryPoint()},
			wantStatus:  model.ConditionStatusFalse,
			wantType:    "Programmed",
			wantReason:  "Invalid",
			wantCode:    "gateway-not-programmed",
			wantMessage: "Gateway is not programmed: listener configuration is invalid",
		},
		{
			name: "missing condition status is unavailable",
			resources: []source.Resource{
				testGatewayResource(testGatewayWithListener("infra", "edge", "shop.example.com", "https")),
				testHTTPRouteResource(testRoute()),
			},
			wantStatus: model.ConditionStatusUnavailable,
			wantType:   string(gatewayv1.RouteConditionAccepted),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := model.Analysis{
				Hostname:    "shop.example.com",
				EntryPoints: tt.entryPoints,
			}
			if err := Diagnose(&analysis, source.Snapshot{Resources: tt.resources}); err != nil {
				t.Fatalf("Diagnose() error = %v", err)
			}

			var conditions []model.Condition
			if len(tt.entryPoints) > 0 {
				conditions = analysis.EntryPoints[0].Conditions
			} else if len(analysis.Routes) > 0 {
				conditions = analysis.Routes[0].Conditions
			}
			foundCondition := false
			for _, condition := range conditions {
				if condition.Type == tt.wantType && condition.Status == tt.wantStatus {
					foundCondition = true
					if condition.Reason != tt.wantReason {
						t.Errorf("condition reason = %q, want %q", condition.Reason, tt.wantReason)
					}
					break
				}
			}
			if !foundCondition {
				t.Fatalf("conditions = %#v, want %s status %q", conditions, tt.wantType, tt.wantStatus)
			}

			if tt.wantCode == "" {
				if len(analysis.Findings) != 0 {
					t.Fatalf("findings = %#v, want none for unavailable status", analysis.Findings)
				}
				return
			}
			finding := findFinding(analysis.Findings, tt.wantCode)
			if finding == nil {
				t.Fatalf("findings = %#v, want code %q", analysis.Findings, tt.wantCode)
			}
			if !strings.Contains(finding.Message, tt.wantMessage) {
				t.Errorf("finding message = %q, want substring %q", finding.Message, tt.wantMessage)
			}
			if finding.Severity != model.FindingSeverityWarning {
				t.Errorf("finding severity = %q, want warning", finding.Severity)
			}
			if len(finding.Resources) == 0 {
				t.Error("condition finding has no related resource")
			}
		})
	}
}

func TestDiagnoseHTTPRouteBindingFailures(t *testing.T) {
	tests := []struct {
		name      string
		route     *gatewayv1.HTTPRoute
		gateways  []*gatewayv1.Gateway
		services  []*corev1.Service
		wantCode  string
		wantCause string
	}{
		{
			name:      "missing parent Gateway",
			route:     testRoute(),
			wantCode:  "missing-parent-gateway",
			wantCause: "parent Gateway infra/edge does not exist",
		},
		{
			name:      "missing listener",
			route:     testRoute(),
			gateways:  []*gatewayv1.Gateway{testGateway("infra", "edge", "shop.example.com")},
			wantCode:  "missing-parent-listener",
			wantCause: "listener https does not exist",
		},
		{
			name:      "hostname does not intersect",
			route:     testRouteWithHost("other.example.com"),
			gateways:  []*gatewayv1.Gateway{testGatewayWithListener("infra", "edge", "shop.example.com", "https")},
			wantCode:  "hostname-intersection-failed",
			wantCause: "route hostname other.example.com does not intersect",
		},
		{
			name: "backend Service is missing",
			route: testRouteWithBackend(gatewayv1.BackendObjectReference{
				Name: gatewayv1.ObjectName("missing"),
				Port: gatewayPortPointer(8080),
			}),
			gateways:  []*gatewayv1.Gateway{testGatewayWithListener("infra", "edge", "shop.example.com", "https")},
			wantCode:  "unresolved-backend-reference",
			wantCause: "backend Service shop/missing does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resources := []source.Resource{testHTTPRouteResource(tt.route)}
			for _, gateway := range tt.gateways {
				resources = append(resources, testGatewayResource(gateway))
			}
			for _, service := range tt.services {
				resources = append(resources, testServiceResource(service))
			}
			analysis := model.Analysis{Hostname: "shop.example.com"}
			if err := Diagnose(&analysis, source.Snapshot{Resources: resources}); err != nil {
				t.Fatalf("Diagnose() error = %v", err)
			}
			finding := findFinding(analysis.Findings, tt.wantCode)
			if finding == nil {
				t.Fatalf("findings = %#v, want code %q", analysis.Findings, tt.wantCode)
			}
			if !strings.Contains(finding.Message, tt.wantCause) {
				t.Errorf("finding message = %q, want substring %q", finding.Message, tt.wantCause)
			}
			if len(finding.Resources) == 0 {
				t.Error("binding finding has no related resource")
			}
		})
	}
}

func TestDiagnoseReportsUnauthorizedCrossNamespaceBackend(t *testing.T) {
	route := testRouteWithBackend(gatewayv1.BackendObjectReference{
		Name: gatewayv1.ObjectName("api"), Namespace: gatewayNamespacePointer("backend"),
		Port: gatewayPortPointer(8080),
	})
	grant := &gatewayv1beta1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{Namespace: "backend", Name: "limited"},
		Spec: gatewayv1beta1.ReferenceGrantSpec{
			From: []gatewayv1beta1.ReferenceGrantFrom{{
				Group: gatewayGroup, Kind: "HTTPRoute", Namespace: "shop",
			}},
			To: []gatewayv1beta1.ReferenceGrantTo{{
				Group: "", Kind: "Service", Name: gatewayObjectNamePointer("other"),
			}},
		},
	}
	analysis := model.Analysis{Hostname: "shop.example.com"}
	snapshot := source.Snapshot{Resources: []source.Resource{
		testHTTPRouteResource(route),
		testResource(grant, gatewayGroup, "v1beta1", "ReferenceGrant", "backend", "limited"),
	}}
	if err := Diagnose(&analysis, snapshot); err != nil {
		t.Fatalf("Diagnose() error = %v", err)
	}
	finding := findFinding(analysis.Findings, "missing-reference-grant")
	if finding == nil {
		t.Fatalf("findings = %#v, want missing-reference-grant", analysis.Findings)
	}
	if !strings.Contains(finding.Message, "no ReferenceGrant") {
		t.Errorf("finding message = %q, want an authorization explanation", finding.Message)
	}
}

func TestDiagnoseDoesNotInferMissingClusterBackendsFromSnapshotAbsence(t *testing.T) {
	route := testRouteWithBackend(gatewayv1.BackendObjectReference{
		Name: gatewayv1.ObjectName("api"),
		Port: gatewayPortPointer(8080),
	})
	routeIdentity := model.ResourceIdentity{
		Group: gatewayGroup, Version: "v1", Kind: "HTTPRoute", Namespace: "shop", Name: "route",
	}
	analysis := model.Analysis{
		Hostname: "shop.example.com",
		Source:   model.SourceSnapshot{Mode: model.SourceModeCluster},
		Routes: []model.Route{{
			Origin:   model.OriginGatewayAPI,
			Resource: routeIdentity,
			Rules: []model.Rule{{Index: 1, Destinations: []model.Destination{{
				Kind: model.DestinationKindService, Namespace: "shop", Name: "api",
				Service: &model.Service{
					Identity:     model.ResourceIdentity{Version: "v1", Kind: "Service", Namespace: "shop", Name: "api"},
					Availability: model.AvailabilityPresent,
				},
			}}}},
		}},
	}
	snapshot := source.Snapshot{
		Metadata: model.SourceSnapshot{Mode: model.SourceModeCluster},
		Resources: []source.Resource{
			testHTTPRouteResource(route),
			testGatewayResource(testGatewayWithListener("infra", "edge", "shop.example.com", "https")),
		},
	}
	if err := Diagnose(&analysis, snapshot); err != nil {
		t.Fatalf("Diagnose() error = %v", err)
	}
	if finding := findFinding(analysis.Findings, "unresolved-backend-reference"); finding != nil {
		t.Fatalf("found backend warning despite resolved Service: %#v", *finding)
	}
}

func testHTTPRouteResource(route *gatewayv1.HTTPRoute) source.Resource {
	return testResource(route, gatewayGroup, "v1", "HTTPRoute", route.Namespace, route.Name)
}

func testGatewayResource(gateway *gatewayv1.Gateway) source.Resource {
	return testResource(gateway, gatewayGroup, "v1", "Gateway", gateway.Namespace, gateway.Name)
}

func testServiceResource(service *corev1.Service) source.Resource {
	return testResource(service, "", "v1", "Service", service.Namespace, service.Name)
}

func testResource(object runtime.Object, group, version, kind, namespace, name string) source.Resource {
	return source.Resource{
		Identity: model.ResourceIdentity{
			Group: group, Version: version, Kind: kind, Namespace: namespace, Name: name,
		},
		GVK:    schema.GroupVersionKind{Group: group, Version: version, Kind: kind},
		Object: object,
	}
}

func testGateway(namespace, name, hostname string) *gatewayv1.Gateway {
	return testGatewayWithListener(namespace, name, hostname, "web")
}

func testGatewayWithListener(namespace, name, hostname, listenerName string) *gatewayv1.Gateway {
	host := gatewayv1.Hostname(hostname)
	return &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec: gatewayv1.GatewaySpec{
			Listeners: []gatewayv1.Listener{{Name: gatewayv1.SectionName(listenerName), Hostname: &host}},
		},
	}
}

func testGatewayWithCondition(namespace, name, hostname string, condition metav1.Condition) *gatewayv1.Gateway {
	gateway := testGatewayWithListener(namespace, name, hostname, "https")
	gateway.Status.Conditions = []metav1.Condition{condition}
	return gateway
}

func testGatewayEntryPoint() model.EntryPoint {
	return model.EntryPoint{
		Origin: model.OriginGatewayAPI,
		Resource: model.ResourceIdentity{
			Group: gatewayGroup, Version: "v1", Kind: "Gateway", Namespace: "infra", Name: "edge",
		},
		Name: "https",
	}
}

func testRoute() *gatewayv1.HTTPRoute {
	return testRouteWithHost("shop.example.com")
}

func testRouteWithHost(hostname string) *gatewayv1.HTTPRoute {
	parent := gatewayv1.ParentReference{
		Name: "edge", Namespace: gatewayNamespacePointer("infra"),
		SectionName: sectionNamePointer("https"),
	}
	host := gatewayv1.Hostname(hostname)
	return &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "route"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{ParentRefs: []gatewayv1.ParentReference{parent}},
			Hostnames:       []gatewayv1.Hostname{host},
		},
	}
}

func testRouteWithConditions(conditionType string, status metav1.ConditionStatus, reason, message string) *gatewayv1.HTTPRoute {
	route := testRoute()
	parent := route.Spec.ParentRefs[0]
	route.Status.Parents = []gatewayv1.RouteParentStatus{{
		ParentRef: parent,
		Conditions: []metav1.Condition{{
			Type: conditionType, Status: status, Reason: reason, Message: message,
			LastTransitionTime: metav1.Now(), ObservedGeneration: 1,
		}},
	}}
	return route
}

func testRouteWithBackend(backend gatewayv1.BackendObjectReference) *gatewayv1.HTTPRoute {
	route := testRoute()
	route.Spec.Rules = []gatewayv1.HTTPRouteRule{{
		BackendRefs: []gatewayv1.HTTPBackendRef{{BackendObjectReference: backend}},
	}}
	return route
}

func gatewayNamespacePointer(namespace string) *gatewayv1.Namespace {
	value := gatewayv1.Namespace(namespace)
	return &value
}

func sectionNamePointer(name string) *gatewayv1.SectionName {
	value := gatewayv1.SectionName(name)
	return &value
}

func gatewayPortPointer(port int32) *gatewayv1.PortNumber {
	value := gatewayv1.PortNumber(port)
	return &value
}

func gatewayObjectNamePointer(value string) *gatewayv1.ObjectName {
	name := gatewayv1.ObjectName(value)
	return &name
}

func findFinding(findings []model.Finding, code string) *model.Finding {
	for i := range findings {
		if findings[i].Code == code {
			return &findings[i]
		}
	}
	return nil
}
