package kube

import (
	"context"
	"errors"
	"strings"
	"testing"

	istiov1 "istio.io/client-go/pkg/apis/networking/v1"
	istiofake "istio.io/client-go/pkg/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	gatewayfake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"
)

func TestClientReadMethods(t *testing.T) {
	ctx := context.Background()
	core := kubefake.NewClientset(
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api-pod"}},
		&discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api-1"}},
	)
	istioClient := istiofake.NewSimpleClientset(
		&istiov1.VirtualService{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api"}},
		&istiov1.DestinationRule{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api"}},
		&istiov1.ServiceEntry{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "external"}},
	)
	istioClient.PrependReactor("list", "gateways", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &istiov1.GatewayList{Items: []*istiov1.Gateway{{
			ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "edge"},
		}}}, nil
	})
	gatewayClient := gatewayfake.NewClientset(
		&gatewayv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{Name: "edge"}},
		&gatewayv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api"}},
		&gatewayv1beta1.ReferenceGrant{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "grant"}},
	)
	gatewayClient.PrependReactor("list", "gateways", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, &gatewayv1.GatewayList{Items: []gatewayv1.Gateway{{
			ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "edge"},
		}}}, nil
	})
	client := NewClientForInterfaces(
		core, core.Discovery(), istioClient.NetworkingV1(),
		gatewayClient.GatewayV1(), gatewayClient.GatewayV1beta1(),
	)

	checks := []struct {
		name string
		call func() error
	}{
		{name: "Istio Gateways", call: func() error { _, err := client.ListIstioGateways(ctx, "shop", metav1.ListOptions{}); return err }},
		{name: "VirtualServices", call: func() error { _, err := client.ListVirtualServices(ctx, "shop", metav1.ListOptions{}); return err }},
		{name: "DestinationRules", call: func() error { _, err := client.ListDestinationRules(ctx, "shop", metav1.ListOptions{}); return err }},
		{name: "ServiceEntries", call: func() error { _, err := client.ListServiceEntries(ctx, "shop", metav1.ListOptions{}); return err }},
		{name: "GatewayClasses", call: func() error { _, err := client.ListGatewayClasses(ctx, metav1.ListOptions{}); return err }},
		{name: "Gateway API Gateways", call: func() error { _, err := client.ListGateways(ctx, "shop", metav1.ListOptions{}); return err }},
		{name: "HTTPRoutes", call: func() error { _, err := client.ListHTTPRoutes(ctx, "shop", metav1.ListOptions{}); return err }},
		{name: "ReferenceGrants", call: func() error { _, err := client.ListReferenceGrants(ctx, "shop", metav1.ListOptions{}); return err }},
		{name: "GetService", call: func() error { _, err := client.GetService(ctx, "shop", "api", metav1.GetOptions{}); return err }},
		{name: "ListServices", call: func() error { _, err := client.ListServices(ctx, "shop", metav1.ListOptions{}); return err }},
		{name: "EndpointSlices", call: func() error { _, err := client.ListEndpointSlices(ctx, "shop", metav1.ListOptions{}); return err }},
		{name: "GetPod", call: func() error { _, err := client.GetPod(ctx, "shop", "api-pod", metav1.GetOptions{}); return err }},
		{name: "ListPods", call: func() error { _, err := client.ListPods(ctx, "shop", metav1.ListOptions{}); return err }},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.call(); err != nil {
				t.Fatalf("read returned an error: %v", err)
			}
		})
	}
}

func TestClientReadMethodsWrapErrors(t *testing.T) {
	ctx := context.Background()
	core := kubefake.NewClientset()
	core.PrependReactor("*", "*", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("read denied")
	})
	istioClient := istiofake.NewSimpleClientset()
	istioClient.PrependReactor("*", "*", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("read denied")
	})
	gatewayClient := gatewayfake.NewClientset()
	gatewayClient.PrependReactor("*", "*", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("read denied")
	})
	client := NewClientForInterfaces(
		core, core.Discovery(), istioClient.NetworkingV1(),
		gatewayClient.GatewayV1(), gatewayClient.GatewayV1beta1(),
	)
	checks := []struct {
		name string
		call func() error
		want string
	}{
		{name: "Istio Gateways", call: func() error { _, err := client.ListIstioGateways(ctx, "shop", metav1.ListOptions{}); return err }, want: "list Istio Gateways"},
		{name: "VirtualServices", call: func() error { _, err := client.ListVirtualServices(ctx, "shop", metav1.ListOptions{}); return err }, want: "list Istio VirtualServices"},
		{name: "DestinationRules", call: func() error { _, err := client.ListDestinationRules(ctx, "shop", metav1.ListOptions{}); return err }, want: "list Istio DestinationRules"},
		{name: "ServiceEntries", call: func() error { _, err := client.ListServiceEntries(ctx, "shop", metav1.ListOptions{}); return err }, want: "list Istio ServiceEntries"},
		{name: "GatewayClasses", call: func() error { _, err := client.ListGatewayClasses(ctx, metav1.ListOptions{}); return err }, want: "list GatewayClasses"},
		{name: "Gateway API Gateways", call: func() error { _, err := client.ListGateways(ctx, "shop", metav1.ListOptions{}); return err }, want: "list Gateway API Gateways"},
		{name: "HTTPRoutes", call: func() error { _, err := client.ListHTTPRoutes(ctx, "shop", metav1.ListOptions{}); return err }, want: "list HTTPRoutes"},
		{name: "ReferenceGrants", call: func() error { _, err := client.ListReferenceGrants(ctx, "shop", metav1.ListOptions{}); return err }, want: "list ReferenceGrants"},
		{name: "GetService", call: func() error { _, err := client.GetService(ctx, "shop", "api", metav1.GetOptions{}); return err }, want: "get Service shop/api"},
		{name: "ListServices", call: func() error { _, err := client.ListServices(ctx, "shop", metav1.ListOptions{}); return err }, want: "list Services"},
		{name: "EndpointSlices", call: func() error { _, err := client.ListEndpointSlices(ctx, "shop", metav1.ListOptions{}); return err }, want: "list EndpointSlices in shop"},
		{name: "GetPod", call: func() error { _, err := client.GetPod(ctx, "shop", "api-pod", metav1.GetOptions{}); return err }, want: "get Pod shop/api-pod"},
		{name: "ListPods", call: func() error { _, err := client.ListPods(ctx, "shop", metav1.ListOptions{}); return err }, want: "list Pods"},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			err := check.call()
			if err == nil || !strings.Contains(err.Error(), check.want) {
				t.Fatalf("read error = %v, want context %q", err, check.want)
			}
		})
	}
}

func TestNewClientRejectsIncompleteRESTConfig(t *testing.T) {
	client, err := NewClient(&rest.Config{})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client == nil || client.Discovery() == nil {
		t.Fatal("NewClient() returned a client without discovery")
	}
}
