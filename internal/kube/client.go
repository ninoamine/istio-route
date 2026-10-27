package kube

import (
	"context"
	"fmt"

	networkingv1 "istio.io/client-go/pkg/apis/networking/v1"
	istio "istio.io/client-go/pkg/clientset/versioned"
	istionetworking "istio.io/client-go/pkg/clientset/versioned/typed/networking/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	gatewayclient "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
	gatewayv1client "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/typed/apis/v1"
	gatewayv1beta1client "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/typed/apis/v1beta1"
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

type EndpointReader interface {
	GetService(context.Context, string, string, metav1.GetOptions) (*corev1.Service, error)
	ListEndpointSlices(context.Context, string, metav1.ListOptions) (*discoveryv1.EndpointSliceList, error)
	GetPod(context.Context, string, string, metav1.GetOptions) (*corev1.Pod, error)
}

type Client struct {
	core        kubernetes.Interface
	discovery   discovery.DiscoveryInterface
	istio       istionetworking.NetworkingV1Interface
	gateway     gatewayv1client.GatewayV1Interface
	gatewayBeta gatewayv1beta1client.GatewayV1beta1Interface
}

func NewClient(config *rest.Config) (*Client, error) {
	core, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes client: %w", err)
	}
	istioClient, err := istio.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create Istio client: %w", err)
	}
	gatewayClient, err := gatewayclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create Gateway API client: %w", err)
	}
	return &Client{
		core:        core,
		discovery:   core.Discovery(),
		istio:       istioClient.NetworkingV1(),
		gateway:     gatewayClient.GatewayV1(),
		gatewayBeta: gatewayClient.GatewayV1beta1(),
	}, nil
}

func NewClientForInterfaces(
	core kubernetes.Interface,
	discoveryClient discovery.DiscoveryInterface,
	istioClient istionetworking.NetworkingV1Interface,
	gatewayClient gatewayv1client.GatewayV1Interface,
	gatewayBetaClient gatewayv1beta1client.GatewayV1beta1Interface,
) *Client {
	return &Client{
		core:        core,
		discovery:   discoveryClient,
		istio:       istioClient,
		gateway:     gatewayClient,
		gatewayBeta: gatewayBetaClient,
	}
}

func (c *Client) Discovery() discovery.DiscoveryInterface {
	return c.discovery
}

func (c *Client) ListIstioGateways(ctx context.Context, namespace string, options metav1.ListOptions) (*networkingv1.GatewayList, error) {
	result, err := c.istio.Gateways(namespace).List(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("list Istio Gateways: %w", err)
	}
	return result, nil
}

func (c *Client) ListVirtualServices(ctx context.Context, namespace string, options metav1.ListOptions) (*networkingv1.VirtualServiceList, error) {
	result, err := c.istio.VirtualServices(namespace).List(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("list Istio VirtualServices: %w", err)
	}
	return result, nil
}

func (c *Client) ListDestinationRules(ctx context.Context, namespace string, options metav1.ListOptions) (*networkingv1.DestinationRuleList, error) {
	result, err := c.istio.DestinationRules(namespace).List(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("list Istio DestinationRules: %w", err)
	}
	return result, nil
}

func (c *Client) ListServiceEntries(ctx context.Context, namespace string, options metav1.ListOptions) (*networkingv1.ServiceEntryList, error) {
	result, err := c.istio.ServiceEntries(namespace).List(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("list Istio ServiceEntries: %w", err)
	}
	return result, nil
}

func (c *Client) ListGatewayClasses(ctx context.Context, options metav1.ListOptions) (*gatewayv1.GatewayClassList, error) {
	result, err := c.gateway.GatewayClasses().List(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("list GatewayClasses: %w", err)
	}
	return result, nil
}

func (c *Client) ListGateways(ctx context.Context, namespace string, options metav1.ListOptions) (*gatewayv1.GatewayList, error) {
	result, err := c.gateway.Gateways(namespace).List(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("list Gateway API Gateways: %w", err)
	}
	return result, nil
}

func (c *Client) ListHTTPRoutes(ctx context.Context, namespace string, options metav1.ListOptions) (*gatewayv1.HTTPRouteList, error) {
	result, err := c.gateway.HTTPRoutes(namespace).List(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("list HTTPRoutes: %w", err)
	}
	return result, nil
}

func (c *Client) ListReferenceGrants(ctx context.Context, namespace string, options metav1.ListOptions) (*gatewayv1beta1.ReferenceGrantList, error) {
	result, err := c.gatewayBeta.ReferenceGrants(namespace).List(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("list ReferenceGrants: %w", err)
	}
	return result, nil
}

func (c *Client) GetService(ctx context.Context, namespace, name string, options metav1.GetOptions) (*corev1.Service, error) {
	result, err := c.core.CoreV1().Services(namespace).Get(ctx, name, options)
	if err != nil {
		return nil, fmt.Errorf("get Service %s/%s: %w", namespace, name, err)
	}
	return result, nil
}

func (c *Client) ListServices(ctx context.Context, namespace string, options metav1.ListOptions) (*corev1.ServiceList, error) {
	result, err := c.core.CoreV1().Services(namespace).List(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("list Services: %w", err)
	}
	return result, nil
}

func (c *Client) ListEndpointSlices(ctx context.Context, namespace string, options metav1.ListOptions) (*discoveryv1.EndpointSliceList, error) {
	result, err := c.core.DiscoveryV1().EndpointSlices(namespace).List(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("list EndpointSlices in %s: %w", namespace, err)
	}
	return result, nil
}

func (c *Client) GetPod(ctx context.Context, namespace, name string, options metav1.GetOptions) (*corev1.Pod, error) {
	result, err := c.core.CoreV1().Pods(namespace).Get(ctx, name, options)
	if err != nil {
		return nil, fmt.Errorf("get Pod %s/%s: %w", namespace, name, err)
	}
	return result, nil
}

func (c *Client) ListPods(ctx context.Context, namespace string, options metav1.ListOptions) (*corev1.PodList, error) {
	result, err := c.core.CoreV1().Pods(namespace).List(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("list Pods: %w", err)
	}
	return result, nil
}
