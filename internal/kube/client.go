package kube

import (
	"context"

	networkingv1 "istio.io/client-go/pkg/apis/networking/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// Reader exposes only the Kubernetes reads required by hostname analysis.
type Reader interface {
	ListIstioGateways(context.Context, string, metav1.ListOptions) (*networkingv1.GatewayList, error)
	ListVirtualServices(context.Context, string, metav1.ListOptions) (*networkingv1.VirtualServiceList, error)
	ListDestinationRules(context.Context, string, metav1.ListOptions) (*networkingv1.DestinationRuleList, error)
	ListServiceEntries(context.Context, string, metav1.ListOptions) (*networkingv1.ServiceEntryList, error)

	ListGatewayClasses(context.Context, metav1.ListOptions) (*gatewayv1.GatewayClassList, error)
	ListGateways(context.Context, string, metav1.ListOptions) (*gatewayv1.GatewayList, error)
	ListHTTPRoutes(context.Context, string, metav1.ListOptions) (*gatewayv1.HTTPRouteList, error)
	ListReferenceGrants(context.Context, string, metav1.ListOptions) (*gatewayv1beta1.ReferenceGrantList, error)

	GetService(context.Context, string, string, metav1.GetOptions) (*corev1.Service, error)
	ListServices(context.Context, string, metav1.ListOptions) (*corev1.ServiceList, error)
	ListEndpointSlices(context.Context, string, metav1.ListOptions) (*discoveryv1.EndpointSliceList, error)
	GetPod(context.Context, string, string, metav1.GetOptions) (*corev1.Pod, error)
	ListPods(context.Context, string, metav1.ListOptions) (*corev1.PodList, error)
}
