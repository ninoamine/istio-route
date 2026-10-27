package resolve

import (
	"context"
	"strings"
	"testing"

	"github.com/ninoamine/istio-route/internal/model"
	"github.com/ninoamine/istio-route/internal/source"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	istioclientv1 "istio.io/client-go/pkg/apis/networking/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestResolveMixedRoutingTree(t *testing.T) {
	istioGateway := &istioclientv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Namespace: "istio-system", Name: "public-gw"},
		Spec: istiov1alpha3.Gateway{Servers: []*istiov1alpha3.Server{{
			Name:  "https",
			Port:  &istiov1alpha3.Port{Number: 443, Name: "https", Protocol: "HTTPS"},
			Hosts: []string{"shop.example.com"},
		}}},
	}
	virtualService := &istioclientv1.VirtualService{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "shop-vs"},
		Spec: istiov1alpha3.VirtualService{
			Hosts:    []string{"shop.example.com"},
			Gateways: []string{"istio-system/public-gw"},
			Http: []*istiov1alpha3.HTTPRoute{{
				Match: []*istiov1alpha3.HTTPMatchRequest{{
					Uri:     &istiov1alpha3.StringMatch{MatchType: &istiov1alpha3.StringMatch_Prefix{Prefix: "/api"}},
					Headers: map[string]*istiov1alpha3.StringMatch{"x-canary": {MatchType: &istiov1alpha3.StringMatch_Exact{Exact: "true"}}},
				}},
				Rewrite: &istiov1alpha3.HTTPRewrite{Uri: "/"},
				Timeout: nil,
				Route: []*istiov1alpha3.HTTPRouteDestination{{
					Destination: &istiov1alpha3.Destination{
						Host: "api.shop.svc.cluster.local", Subset: "v1",
						Port: &istiov1alpha3.PortSelector{Number: 8080},
					},
					Weight: 100,
				}},
			}},
		},
	}
	class := &gatewayv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "istio"},
		Spec:       gatewayv1.GatewayClassSpec{ControllerName: "istio.io/gateway-controller"},
	}
	apiGateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Namespace: "infra", Name: "edge-gw"},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "istio",
			Listeners: []gatewayv1.Listener{{
				Name: "https", Hostname: hostnamePointer("shop.example.com"),
				Port: 443, Protocol: gatewayv1.HTTPSProtocolType,
				AllowedRoutes: &gatewayv1.AllowedRoutes{Namespaces: &gatewayv1.RouteNamespaces{From: routeNamespaceAll()}},
			}},
		},
		Status: gatewayv1.GatewayStatus{Conditions: []metav1.Condition{{
			Type: "Programmed", Status: metav1.ConditionTrue, Reason: "Programmed",
		}}},
	}
	httpRoute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "shop-route"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{ParentRefs: []gatewayv1.ParentReference{{
				Name: "edge-gw", Namespace: namespacePointer("infra"), SectionName: sectionPointer("https"),
			}}},
			Hostnames: []gatewayv1.Hostname{"shop.example.com"},
			Rules: []gatewayv1.HTTPRouteRule{{
				Matches: []gatewayv1.HTTPRouteMatch{{
					Path: &gatewayv1.HTTPPathMatch{Type: pathTypePointer(gatewayv1.PathMatchPathPrefix), Value: stringPointer("/v2")},
					Headers: []gatewayv1.HTTPHeaderMatch{{
						Type: headerMatchPointer(gatewayv1.HeaderMatchExact), Name: "x-canary", Value: "true",
					}},
				}},
				BackendRefs: []gatewayv1.HTTPBackendRef{{
					BackendRef: gatewayv1.BackendRef{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name: "api-v2", Namespace: namespacePointer("shop"), Port: portPointer(8080),
						},
						Weight: testInt32Pointer(100),
					},
				}},
			}},
		},
		Status: gatewayv1.HTTPRouteStatus{RouteStatus: gatewayv1.RouteStatus{Parents: []gatewayv1.RouteParentStatus{{
			ParentRef: gatewayv1.ParentReference{Name: "edge-gw", Namespace: namespacePointer("infra"), SectionName: sectionPointer("https")},
			Conditions: []metav1.Condition{
				{Type: "Accepted", Status: metav1.ConditionTrue},
				{Type: "ResolvedRefs", Status: metav1.ConditionTrue},
			},
		}}}},
	}

	snapshot := source.Snapshot{
		Metadata: model.SourceSnapshot{Mode: model.SourceModeCluster},
		Resources: []source.Resource{
			testResource("networking.istio.io", "v1", "Gateway", "istio-system", "public-gw", istioGateway),
			testResource("networking.istio.io", "v1", "VirtualService", "shop", "shop-vs", virtualService),
			testResource("networking.istio.io", "v1", "DestinationRule", "shop", "api", &istioclientv1.DestinationRule{
				ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api"},
				Spec: istiov1alpha3.DestinationRule{
					Host:    "api.shop.svc.cluster.local",
					Subsets: []*istiov1alpha3.Subset{{Name: "v1", Labels: map[string]string{"version": "v1"}}},
				},
			}),
			testResource("gateway.networking.k8s.io", "v1", "GatewayClass", "", "istio", class),
			testResource("gateway.networking.k8s.io", "v1", "Gateway", "infra", "edge-gw", apiGateway),
			testResource("gateway.networking.k8s.io", "v1", "HTTPRoute", "shop", "shop-route", httpRoute),
		},
	}
	reader := fixtureEndpointReader{
		services: map[string]*corev1.Service{
			"shop/api": {
				ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api"},
				Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 8080}}},
			},
			"shop/api-v2": {
				ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api-v2"},
				Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 8080}}},
			},
		},
		slices: map[string][]discoveryv1.EndpointSlice{
			"shop/api": {{
				Endpoints: []discoveryv1.Endpoint{{
					Addresses:  []string{"10.0.0.2"},
					Conditions: discoveryv1.EndpointConditions{Ready: boolPointer(true), Serving: boolPointer(true)},
					TargetRef:  &corev1.ObjectReference{Kind: "Pod", Namespace: "shop", Name: "api-abc"},
				}},
			}},
		},
		pods: map[string]*corev1.Pod{
			"shop/api-abc": {
				ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api-abc"},
				Status: corev1.PodStatus{
					Phase:      corev1.PodRunning,
					Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
				},
			},
		},
	}

	got, err := Resolve(context.Background(), "shop.example.com", snapshot, reader)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(got.EntryPoints) != 2 {
		t.Fatalf("got %d entry points, want 2", len(got.EntryPoints))
	}
	if len(got.Routes) != 2 {
		t.Fatalf("got %d routes, want 2", len(got.Routes))
	}
	istioRoute := findRoute(t, got.Routes, "shop-vs")
	if len(istioRoute.Rules) != 1 || istioRoute.Rules[0].Index != 1 {
		t.Fatalf("Istio rules = %#v, want one ordered rule", istioRoute.Rules)
	}
	if got := istioRoute.Rules[0].Matches[0].Path; got != "/api" {
		t.Errorf("Istio URI prefix = %q, want /api", got)
	}
	if got := istioRoute.Rules[0].Destinations[0].Subset; got != "v1" {
		t.Errorf("Istio destination subset = %q, want v1", got)
	}
	if got := istioRoute.Rules[0].Destinations[0].DestinationRule; got == nil || got.Name != "api" {
		t.Errorf("Istio destination rule = %#v, want api", got)
	}
	apiRoute := findRoute(t, got.Routes, "shop-route")
	if len(apiRoute.Rules) != 1 || apiRoute.Rules[0].Index != 1 {
		t.Fatalf("Gateway API rules = %#v, want one ordered rule", apiRoute.Rules)
	}

	if got := apiRoute.Rules[0].Matches[0].Headers["x-canary"]; got != "true" {
		t.Errorf("Gateway API header match = %q, want true", got)
	}
	if len(apiRoute.Rules[0].Destinations) != 1 || apiRoute.Rules[0].Destinations[0].Weight == nil || *apiRoute.Rules[0].Destinations[0].Weight != 100 {
		t.Fatalf("Gateway API destinations = %#v, want api-v2 at weight 100", apiRoute.Rules[0].Destinations)
	}
	apiDestination := apiRoute.Rules[0].Destinations[0]
	if apiDestination.Service == nil || len(apiDestination.Service.Endpoints) != 0 {
		t.Errorf("api-v2 service = %#v, want service data and no endpoints", apiDestination.Service)
	}
	istioDestination := istioRoute.Rules[0].Destinations[0]
	if istioDestination.Service == nil || len(istioDestination.Service.Endpoints) != 1 {
		t.Fatalf("Istio service = %#v, want one actual endpoint", istioDestination.Service)
	}
	if istioDestination.Service.Endpoints[0].Pod == nil || istioDestination.Service.Endpoints[0].Pod.Ready == nil || !*istioDestination.Service.Endpoints[0].Pod.Ready {
		t.Errorf("Istio endpoint pod = %#v, want ready pod", istioDestination.Service.Endpoints[0].Pod)
	}
}

func testResource(group, version, kind, namespace, name string, object runtime.Object) source.Resource {
	return source.Resource{
		Identity: model.ResourceIdentity{Group: group, Version: version, Kind: kind, Namespace: namespace, Name: name},
		GVK:      schema.GroupVersionKind{Group: group, Version: version, Kind: kind},
		Object:   object,
	}
}

type fixtureEndpointReader struct {
	services map[string]*corev1.Service
	slices   map[string][]discoveryv1.EndpointSlice
	pods     map[string]*corev1.Pod
}

func (f fixtureEndpointReader) GetService(_ context.Context, namespace, name string, _ metav1.GetOptions) (*corev1.Service, error) {
	if service := f.services[namespace+"/"+name]; service != nil {
		return service.DeepCopy(), nil
	}
	return nil, nil
}

func (f fixtureEndpointReader) ListEndpointSlices(_ context.Context, namespace string, options metav1.ListOptions) (*discoveryv1.EndpointSliceList, error) {
	result := &discoveryv1.EndpointSliceList{}
	serviceName := strings.TrimPrefix(options.LabelSelector, "kubernetes.io/service-name=")
	if slice := f.slices[namespace+"/"+serviceName]; slice != nil {
		result.Items = append(result.Items, slice...)
	}
	return result, nil
}

func (f fixtureEndpointReader) GetPod(_ context.Context, namespace, name string, _ metav1.GetOptions) (*corev1.Pod, error) {
	if pod := f.pods[namespace+"/"+name]; pod != nil {
		return pod.DeepCopy(), nil
	}
	return nil, nil
}

func findRoute(t *testing.T, routes []model.Route, name string) model.Route {
	t.Helper()
	for _, route := range routes {
		if route.Resource.Name == name {
			return route
		}
	}
	t.Fatalf("route %q not found in %#v", name, routes)
	return model.Route{}
}

func boolPointer(value bool) *bool { return &value }

func hostnamePointer(value string) *gatewayv1.Hostname {
	hostname := gatewayv1.Hostname(value)
	return &hostname
}

func namespacePointer(value string) *gatewayv1.Namespace {
	namespace := gatewayv1.Namespace(value)
	return &namespace
}

func sectionPointer(value string) *gatewayv1.SectionName {
	section := gatewayv1.SectionName(value)
	return &section
}

func pathTypePointer(value gatewayv1.PathMatchType) *gatewayv1.PathMatchType {
	return &value
}

func headerMatchPointer(value gatewayv1.HeaderMatchType) *gatewayv1.HeaderMatchType {
	return &value
}

func stringPointer(value string) *string { return &value }

func portPointer(value gatewayv1.PortNumber) *gatewayv1.PortNumber {
	return &value
}

func testInt32Pointer(value int32) *int32 { return &value }

func routeNamespaceAll() *gatewayv1.FromNamespaces {
	value := gatewayv1.NamespacesFromAll
	return &value
}

func TestResolveServiceEntryDestination(t *testing.T) {
	entry := &istioclientv1.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "external"},
		Spec: istiov1alpha3.ServiceEntry{
			Hosts:      []string{"*.external.example"},
			Location:   istiov1alpha3.ServiceEntry_MESH_EXTERNAL,
			Resolution: istiov1alpha3.ServiceEntry_DNS,
			Ports:      []*istiov1alpha3.ServicePort{{Name: "https", Number: 443, Protocol: "HTTPS"}},
		},
	}
	snapshot := source.Snapshot{
		Metadata: model.SourceSnapshot{Mode: model.SourceModeCluster},
		Resources: []source.Resource{
			testResource("networking.istio.io", "v1", "ServiceEntry", "shop", "external", entry),
		},
	}

	got, err := Resolve(context.Background(), "api.external.example", snapshot, nil)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(got.Routes) != 1 || got.Routes[0].Resource.Name != "external" {
		t.Fatalf("routes = %#v, want one ServiceEntry route", got.Routes)
	}
	destination := got.Routes[0].Rules[0].Destinations[0]
	if destination.Kind != model.DestinationKindExternal || destination.External == nil {
		t.Fatalf("destination = %#v, want external ServiceEntry destination", destination)
	}
	if len(destination.External.Ports) != 1 || destination.External.Ports[0].Number != 443 {
		t.Errorf("external ports = %#v, want port 443", destination.External.Ports)
	}
}
