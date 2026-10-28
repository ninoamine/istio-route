package source_test

import (
	"context"
	"testing"

	"github.com/ninoamine/istio-route/internal/kube"
	"github.com/ninoamine/istio-route/internal/model"
	"github.com/ninoamine/istio-route/internal/resolve"
	"github.com/ninoamine/istio-route/internal/source"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayfake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"
)

func TestClusterResolutionReadsOnlyReferencedBackendWorkloads(t *testing.T) {
	coreClient := kubefake.NewClientset(
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "unrelated"}},
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "shop", Name: "api-endpoints",
				Labels: map[string]string{discoveryv1.LabelServiceName: "api"},
			},
			Endpoints: []discoveryv1.Endpoint{{
				Addresses: []string{"10.0.0.1"},
				TargetRef: &corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "shop", Name: "api-1"},
			}},
		},
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "shop", Name: "unrelated-endpoints",
				Labels: map[string]string{discoveryv1.LabelServiceName: "unrelated"},
			},
			Endpoints: []discoveryv1.Endpoint{{
				Addresses: []string{"10.0.0.2"},
				TargetRef: &corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "shop", Name: "unrelated-1"},
			}},
		},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api-1"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "unrelated-1"}},
	)
	coreClient.Resources = []*metav1.APIResourceList{{
		GroupVersion: "gateway.networking.k8s.io/v1",
	}}
	gateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "gateway"},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "edge",
			Listeners: []gatewayv1.Listener{{
				Name: "http", Port: 80, Protocol: gatewayv1.HTTPProtocolType,
				Hostname: hostnamePointer("shop.example.com"),
			}},
		},
	}
	gatewayClient := gatewayfake.NewClientset(
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "edge"},
			Spec:       gatewayv1.GatewayClassSpec{ControllerName: "example.test/controller"},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "route"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{Name: "gateway"}},
				},
				Hostnames: []gatewayv1.Hostname{"shop.example.com"},
				Rules: []gatewayv1.HTTPRouteRule{{
					BackendRefs: []gatewayv1.HTTPBackendRef{{
						BackendRef: gatewayv1.BackendRef{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name: "api", Port: portPointer(8080),
							},
							Weight: weightPointer(100),
						},
					}},
				}},
			},
		},
	)
	gatewayClient.PrependReactor("list", "gateways", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &gatewayv1.GatewayList{Items: []gatewayv1.Gateway{*gateway}}, nil
	})
	istioClient := istiofake.NewSimpleClientset()
	client := kube.NewClientForInterfaces(
		coreClient, coreClient.Discovery(), istioClient.NetworkingV1(),
		gatewayClient.GatewayV1(), gatewayClient.GatewayV1beta1(),
	)
	cluster := &source.ClusterSource{Reader: client, Discovery: coreClient.Discovery()}

	snapshot, err := cluster.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	analysis, err := resolve.Resolve(context.Background(), "shop.example.com", snapshot, client)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(analysis.Routes) != 1 || len(analysis.Routes[0].Rules[0].Destinations) != 1 {
		t.Fatalf("resolved routes = %#v, entries = %#v, snapshot resources = %#v, want one backend", analysis.Routes, analysis.EntryPoints, snapshot.Resources)
	}
	if service := analysis.Routes[0].Rules[0].Destinations[0].Service; service == nil ||
		service.Availability != model.AvailabilityPresent || len(service.Endpoints) != 1 ||
		service.Endpoints[0].Target == nil || service.Endpoints[0].Target.Name != "api-1" {
		t.Fatalf("resolved backend Service = %#v, want api and its endpoint Pod", service)
	}

	var serviceGets, endpointLists, podGets, serviceLists, podLists int
	for _, action := range coreClient.Actions() {
		switch action.GetVerb() + ":" + action.GetResource().Resource {
		case "get:services":
			serviceGets++
			if action.GetNamespace() != "shop" || action.(k8stesting.GetAction).GetName() != "api" {
				t.Errorf("unexpected Service get: %#v", action)
			}
		case "list:endpointslices":
			endpointLists++
			if action.GetNamespace() != "shop" {
				t.Errorf("EndpointSlice list namespace = %q, want shop", action.GetNamespace())
			}
			listAction, ok := action.(k8stesting.ListAction)
			if !ok || listAction.GetListRestrictions().Labels.String() != discoveryv1.LabelServiceName+"=api" {
				t.Errorf("EndpointSlice list selector = %#v, want only api endpoints", action)
			}
		case "get:pods":
			podGets++
			if action.GetNamespace() != "shop" || action.(k8stesting.GetAction).GetName() != "api-1" {
				t.Errorf("unexpected Pod get: %#v", action)
			}
		case "list:services":
			serviceLists++
		case "list:pods":
			podLists++
		}
	}
	if serviceGets != 1 || endpointLists != 1 || podGets != 1 {
		t.Errorf("backend read counts: Service=%d EndpointSlices=%d Pod=%d, want one each", serviceGets, endpointLists, podGets)
	}
	if serviceLists != 0 || podLists != 0 {
		t.Errorf("unrelated inventory reads: Services=%d Pods=%d, want none", serviceLists, podLists)
	}
	for _, action := range coreClient.Actions() {
		if action.GetVerb() != "get" && action.GetVerb() != "list" {
			t.Errorf("core client used non-read action %q", action.GetVerb())
		}
	}
}

func hostnamePointer(hostname string) *gatewayv1.Hostname {
	value := gatewayv1.Hostname(hostname)
	return &value
}

func portPointer(port int32) *gatewayv1.PortNumber {
	value := gatewayv1.PortNumber(port)
	return &value
}

func weightPointer(weight int32) *int32 {
	return &weight
}
