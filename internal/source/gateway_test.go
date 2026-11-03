package source

import (
	"context"
	"testing"

	"github.com/ninoamine/istio-route/internal/kube"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	gatewayfake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"
)

func TestClusterSourceGatewaySnapshotIsTargeted(t *testing.T) {
	coreClient := kubefake.NewClientset(
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "platform", Name: "api"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "other", Name: "unrelated"}},
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "platform", Name: "api-1",
				Labels: map[string]string{"kubernetes.io/service-name": "api"},
			},
		},
	)
	for _, version := range []string{
		"gateway.networking.k8s.io/v1", "gateway.networking.k8s.io/v1beta1", "discovery.k8s.io/v1",
	} {
		coreClient.Resources = append(coreClient.Resources, &metav1.APIResourceList{GroupVersion: version})
	}
	gatewayClient := gatewayfake.NewClientset(
		&gatewayv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{Name: "istio"}},
		targetGateway(),
		targetHTTPRoute(),
		targetGRPCRoute(),
		unrelatedHTTPRoute(),
		&gatewayv1beta1.ReferenceGrant{
			ObjectMeta: metav1.ObjectMeta{Namespace: "platform", Name: "grant"},
			Spec: gatewayv1beta1.ReferenceGrantSpec{
				From: []gatewayv1beta1.ReferenceGrantFrom{{
					Group: gatewayv1beta1.Group(gatewayv1.GroupVersion.Group),
					Kind:  "HTTPRoute", Namespace: "shop",
				}},
				To: []gatewayv1beta1.ReferenceGrantTo{{Kind: "Service"}},
			},
		},
	)
	gatewayClient.PrependReactor("get", "gateways", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, targetGateway(), nil
	})
	gatewayClient.PrependReactor("get", "gatewayclasses", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "istio"},
			Spec:       gatewayv1.GatewayClassSpec{ControllerName: "istio.io/gateway-controller"},
		}, nil
	})
	client := kube.NewClientForInterfaces(
		coreClient, coreClient.Discovery(), nil, gatewayClient.GatewayV1(), gatewayClient.GatewayV1beta1(),
	)
	snapshot, err := (&ClusterSource{
		Reader: client, Discovery: client.Discovery(), Context: "staging", Namespace: "infra",
	}).GatewaySnapshot(context.Background(), "edge", "infra")
	if err != nil {
		t.Fatalf("GatewaySnapshot() error = %v", err)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("Gateway snapshot is invalid: %v", err)
	}
	routeCount := 0
	for _, resource := range snapshot.Resources {
		if resource.Identity.Kind == "HTTPRoute" || resource.Identity.Kind == "GRPCRoute" {
			routeCount++
			if resource.Identity.Name == "unrelated" {
				t.Fatal("snapshot contains a route with no matching parentRef")
			}
		}
	}
	if routeCount != 2 {
		t.Fatalf("snapshot contains %d linked routes, want HTTPRoute and GRPCRoute", routeCount)
	}
	if snapshot.Metadata.Namespace != "infra" || snapshot.Metadata.Context != "staging" {
		t.Errorf("snapshot metadata = %#v, want selected namespace and context", snapshot.Metadata)
	}

	var sawBackendGet, sawEndpointSelector, sawGrantList bool
	for _, action := range coreClient.Actions() {
		if action.GetVerb() == "get" && action.GetResource().Resource == "services" &&
			action.GetNamespace() == "platform" && action.(k8stesting.GetAction).GetName() == "api" {
			sawBackendGet = true
		}
		if action.GetVerb() == "list" && action.GetResource().Resource == "endpointslices" {
			listAction := action.(k8stesting.ListAction)
			sawEndpointSelector = listAction.GetListRestrictions().Labels.String() == "kubernetes.io/service-name=api"
		}
		if action.GetResource().Resource == "services" &&
			(action.GetNamespace() == "other" || action.GetVerb() == "list") {
			t.Errorf("unrelated Service read: %#v", action)
		}
		if action.GetResource().Resource == "pods" || action.GetResource().Resource == "secrets" {
			t.Errorf("out-of-scope core resource read: %#v", action)
		}
	}
	for _, action := range gatewayClient.Actions() {
		if action.GetVerb() == "list" && action.GetResource().Resource == "referencegrants" {
			if action.GetNamespace() != "platform" {
				t.Errorf("ReferenceGrant list namespace = %q, want platform", action.GetNamespace())
			}
			sawGrantList = true
		}
	}
	if !sawBackendGet || !sawEndpointSelector || !sawGrantList {
		t.Errorf("targeted reads: service=%t endpointSelector=%t unnecessaryGrantList=%t",
			sawBackendGet, sawEndpointSelector, sawGrantList)
	}
}

func TestClusterSourceGatewaySnapshotRequiresNamespace(t *testing.T) {
	coreClient := kubefake.NewClientset()
	client := kube.NewClientForInterfaces(
		coreClient, coreClient.Discovery(), nil, nil, nil,
	)
	_, err := (&ClusterSource{Reader: client, Discovery: client.Discovery()}).
		GatewaySnapshot(context.Background(), "edge", "")
	if err == nil || err.Error() != "gateway namespace is required" {
		t.Fatalf("GatewaySnapshot() error = %v, want namespace requirement", err)
	}
}

func TestClusterSourceGatewaySnapshotRecordsUnavailableFamilies(t *testing.T) {
	coreClient := kubefake.NewClientset()
	coreClient.Resources = append(coreClient.Resources, &metav1.APIResourceList{
		GroupVersion: "gateway.networking.k8s.io/v1",
	})
	gatewayClient := gatewayfake.NewClientset()
	gatewayClient.PrependReactor("get", "gateways", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, targetGateway(), nil
	})
	gatewayClient.PrependReactor("get", "gatewayclasses", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "istio"},
			Spec:       gatewayv1.GatewayClassSpec{ControllerName: "istio.io/gateway-controller"},
		}, nil
	})
	for _, resource := range []string{"httproutes", "grpcroutes"} {
		gatewayClient.PrependReactor("list", resource, func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewNotFound(schema.GroupResource{Group: "gateway.networking.k8s.io", Resource: action.GetResource().Resource}, "routes")
		})
	}
	client := kube.NewClientForInterfaces(
		coreClient, coreClient.Discovery(), nil, gatewayClient.GatewayV1(), gatewayClient.GatewayV1beta1(),
	)
	snapshot, err := (&ClusterSource{Reader: client, Discovery: client.Discovery()}).
		GatewaySnapshot(context.Background(), "edge", "infra")
	if err != nil {
		t.Fatalf("GatewaySnapshot() error = %v", err)
	}
	families := make(map[string]bool)
	for _, family := range snapshot.Metadata.Families {
		families[family.Family] = family.Available
	}
	if families["httproute"] || families["grpcroute"] {
		t.Errorf("missing route API families were marked available: %#v", families)
	}
	if families["gateway-api"] != true {
		t.Errorf("Gateway API family availability = %#v, want available", families)
	}
}

func targetGateway() *gatewayv1.Gateway {
	return &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Namespace: "infra", Name: "edge"},
		Spec:       gatewayv1.GatewaySpec{GatewayClassName: "istio"},
	}
}

func targetHTTPRoute() *gatewayv1.HTTPRoute {
	return &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "http"},
		Spec: gatewayv1.HTTPRouteSpec{CommonRouteSpec: gatewayv1.CommonRouteSpec{
			ParentRefs: []gatewayv1.ParentReference{{Name: "edge", Namespace: namespaceRef("infra")}},
		}, Rules: []gatewayv1.HTTPRouteRule{{BackendRefs: []gatewayv1.HTTPBackendRef{{
			BackendRef: gatewayv1.BackendRef{BackendObjectReference: gatewayv1.BackendObjectReference{
				Name: "api", Namespace: namespaceRef("platform"), Port: int32Ref(8080),
			}},
		}}}}},
	}
}

func targetGRPCRoute() *gatewayv1.GRPCRoute {
	return &gatewayv1.GRPCRoute{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "grpc"},
		Spec: gatewayv1.GRPCRouteSpec{CommonRouteSpec: gatewayv1.CommonRouteSpec{
			ParentRefs: []gatewayv1.ParentReference{{Name: "edge", Namespace: namespaceRef("infra")}},
		}, Rules: []gatewayv1.GRPCRouteRule{{BackendRefs: []gatewayv1.GRPCBackendRef{{
			BackendRef: gatewayv1.BackendRef{BackendObjectReference: gatewayv1.BackendObjectReference{
				Name: "api", Namespace: namespaceRef("platform"), Port: int32Ref(8080),
			}},
		}}}}},
	}
}

func unrelatedHTTPRoute() *gatewayv1.HTTPRoute {
	return &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "unrelated"},
		Spec: gatewayv1.HTTPRouteSpec{CommonRouteSpec: gatewayv1.CommonRouteSpec{
			ParentRefs: []gatewayv1.ParentReference{{Name: "elsewhere", Namespace: namespaceRef("infra")}},
		}},
	}
}

func namespaceRef(value string) *gatewayv1.Namespace {
	namespace := gatewayv1.Namespace(value)
	return &namespace
}

func int32Ref(value int32) *int32 {
	return &value
}
