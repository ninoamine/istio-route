package resolve

import (
	"testing"

	"github.com/ninoamine/istio-route/internal/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func TestRouteConditionsMatchEffectiveParentReference(t *testing.T) {
	section := gatewayv1.SectionName("https")
	namespace := gatewayv1.Namespace("infra")
	parent := gatewayv1.ParentReference{Name: "edge", Namespace: &namespace, SectionName: &section}
	statuses := []gatewayv1.RouteParentStatus{
		{
			ParentRef: gatewayv1.ParentReference{Name: "edge", Namespace: &namespace, SectionName: &section},
			Conditions: []metav1.Condition{
				{Type: "Accepted", Status: metav1.ConditionTrue},
				{Type: "ResolvedRefs", Status: metav1.ConditionFalse},
			},
		},
		{
			ParentRef: gatewayv1.ParentReference{
				Name: "edge", Namespace: &namespace, Kind: kindPointer("Service"), SectionName: &section,
			},
			Conditions: []metav1.Condition{{Type: "Accepted", Status: metav1.ConditionFalse}},
		},
	}

	got := routeConditions(statuses, []gatewayv1.ParentReference{parent}, "shop")
	if len(got) != 2 {
		t.Fatalf("routeConditions() = %#v, want only the two matching conditions", got)
	}
	if got[0].Type != "Accepted" || got[0].Status != model.ConditionStatusTrue {
		t.Errorf("Accepted condition = %#v, want true", got[0])
	}
	if got[1].Type != "ResolvedRefs" || got[1].Status != model.ConditionStatusFalse {
		t.Errorf("ResolvedRefs condition = %#v, want false", got[1])
	}

	implicitNamespace := gatewayv1.ParentReference{Name: "edge"}
	implicitStatus := []gatewayv1.RouteParentStatus{{
		ParentRef:  gatewayv1.ParentReference{Name: "edge", Namespace: namespacePointer("shop")},
		Conditions: []metav1.Condition{{Type: "Accepted", Status: metav1.ConditionTrue}},
	}}
	if conditions := routeConditions(implicitStatus, []gatewayv1.ParentReference{implicitNamespace}, "shop"); len(conditions) != 1 {
		t.Errorf("implicit same-namespace ParentRef conditions = %#v, want one condition", conditions)
	}
}

func TestFindReferenceGrant(t *testing.T) {
	namespace := gatewayv1.Namespace("shop")
	grants := []*gatewayv1beta1.ReferenceGrant{{
		ObjectMeta: metav1.ObjectMeta{Namespace: "infra", Name: "allow-shop"},
		Spec: gatewayv1beta1.ReferenceGrantSpec{
			From: []gatewayv1beta1.ReferenceGrantFrom{{
				Group: gatewayv1.Group(gatewayGroup), Kind: "HTTPRoute", Namespace: namespace,
			}},
			To: []gatewayv1beta1.ReferenceGrantTo{{
				Group: "", Kind: "Service", Name: objectNamePointer("api"),
			}},
		},
	}}
	tests := []struct {
		name        string
		fromNS      string
		backendKind string
		backendName string
		wantAllowed bool
	}{
		{name: "matching grant", fromNS: "shop", backendKind: "Service", backendName: "api", wantAllowed: true},
		{name: "wrong source namespace", fromNS: "other", backendKind: "Service", backendName: "api"},
		{name: "wrong backend kind", fromNS: "shop", backendKind: "Secret", backendName: "api"},
		{name: "wrong backend name", fromNS: "shop", backendKind: "Service", backendName: "web"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grant, allowed := findReferenceGrant(grants, tt.fromNS, "route", "", tt.backendKind, "infra", tt.backendName)
			if allowed != tt.wantAllowed {
				t.Fatalf("allowed = %t, want %t", allowed, tt.wantAllowed)
			}
			if (grant != nil) != tt.wantAllowed {
				t.Errorf("grant = %v, want present = %t", grant, tt.wantAllowed)
			}
		})
	}
}

func TestGatewayAttachmentHelpers(t *testing.T) {
	listener := gatewayv1.Listener{
		Name: "https",
		AllowedRoutes: &gatewayv1.AllowedRoutes{
			Namespaces: &gatewayv1.RouteNamespaces{From: fromNamespacesPointer(gatewayv1.NamespacesFromAll)},
			Kinds:      []gatewayv1.RouteGroupKind{{Kind: "HTTPRoute"}},
		},
	}
	gateway := &gatewayv1.Gateway{ObjectMeta: metav1.ObjectMeta{Namespace: "infra", Name: "edge"}}
	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop"},
		Spec: gatewayv1.HTTPRouteSpec{CommonRouteSpec: gatewayv1.CommonRouteSpec{
			ParentRefs: []gatewayv1.ParentReference{{Name: "edge", Namespace: namespacePointer("infra"), SectionName: sectionPointer("https")}},
		}},
	}
	if !routeAttaches(route, gateway, listener) {
		t.Fatal("routeAttaches() = false, want matching parent")
	}
	if allowed := allowedRouteNamespace(listener.AllowedRoutes, route.Namespace, gateway.Namespace); !allowed {
		t.Fatal("allowedRouteNamespace() = false, want all namespaces allowed")
	}
	if !allowedRouteKind(listener.AllowedRoutes) {
		t.Fatal("allowedRouteKind() = false, want HTTPRoute allowed")
	}
	if !routeHostnameIntersects([]string{"shop.example.com"}, hostnamePointer("*.example.com")) {
		t.Fatal("routeHostnameIntersects() = false, want wildcard intersection")
	}
	if routeHostnameIntersects([]string{"other.example.net"}, hostnamePointer("*.example.com")) {
		t.Fatal("routeHostnameIntersects() = true, want disjoint hostnames")
	}

	wrongKind := *route
	wrongKind.Spec.ParentRefs = []gatewayv1.ParentReference{{
		Name: "edge", Namespace: namespacePointer("infra"), Kind: kindPointer("Service"),
	}}
	if routeAttaches(&wrongKind, gateway, listener) {
		t.Fatal("routeAttaches() = true for non-Gateway parent")
	}
	if allowedRouteNamespace(nil, "shop", "infra") {
		t.Fatal("default namespace policy allowed a cross-namespace route")
	}
	if allowedRouteKind(&gatewayv1.AllowedRoutes{Kinds: []gatewayv1.RouteGroupKind{{Kind: "GRPCRoute"}}}) {
		t.Fatal("allowedRouteKind() accepted a non-HTTPRoute kind")
	}
}

func TestGatewayMatchAndTimeoutHelpers(t *testing.T) {
	match := gatewayMatch(gatewayv1.HTTPRouteMatch{
		Method: methodPointer(gatewayv1.HTTPMethodGet),
		Headers: []gatewayv1.HTTPHeaderMatch{{
			Name: "x-version", Value: "v2",
		}},
		QueryParams: []gatewayv1.HTTPQueryParamMatch{{
			Name: "region", Value: "west",
		}},
	})
	if match.Method != "GET" || match.Headers["x-version"] != "v2" || match.QueryParams["region"] != "west" {
		t.Fatalf("gatewayMatch() = %#v, want method, header, and query parameter", match)
	}
	if gatewayMatch(gatewayv1.HTTPRouteMatch{}).Headers != nil {
		t.Fatal("empty gateway match maps should be nil")
	}
	if got := durationValue(nil); got != "" {
		t.Errorf("durationValue(nil) = %q, want empty", got)
	}
	timeout := gatewayv1.Duration("3s")
	if got := durationValue(&timeout); got != "3s" {
		t.Errorf("durationValue() = %q, want 3s", got)
	}
}

func kindPointer(value string) *gatewayv1.Kind {
	kind := gatewayv1.Kind(value)
	return &kind
}

func fromNamespacesPointer(value gatewayv1.FromNamespaces) *gatewayv1.FromNamespaces {
	return &value
}

func methodPointer(value gatewayv1.HTTPMethod) *gatewayv1.HTTPMethod {
	return &value
}

func objectNamePointer(value string) *gatewayv1.ObjectName {
	name := gatewayv1.ObjectName(value)
	return &name
}
