package source

import (
	"context"
	"fmt"

	"github.com/ninoamine/istio-route/internal/kube"
	"github.com/ninoamine/istio-route/internal/model"
	istiov1 "istio.io/client-go/pkg/apis/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type ClusterSource struct {
	Reader    kube.Reader
	Discovery discovery.DiscoveryInterface
	Context   string
	Namespace string
}

type ClusterClientFactory func(*rest.Config) (kube.Reader, discovery.DiscoveryInterface, error)

func NewClusterSource(ctx context.Context, kubeconfig, contextName, namespace string) (*ClusterSource, error) {
	return NewClusterSourceWithClientFactory(
		ctx, kubeconfig, contextName, namespace,
		func(config *rest.Config) (kube.Reader, discovery.DiscoveryInterface, error) {
			client, err := kube.NewClient(config)
			if err != nil {
				return nil, nil, err
			}
			return client, client.Discovery(), nil
		},
	)
}

func NewClusterSourceWithClientFactory(
	ctx context.Context,
	kubeconfig, contextName, namespace string,
	newClients ClusterClientFactory,
) (*ClusterSource, error) {
	if newClients == nil {
		return nil, fmt.Errorf("initialize cluster source: client factory is required")
	}
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loadingRules.ExplicitPath = kubeconfig
	}
	overrides := &clientcmd.ConfigOverrides{}
	overrides.CurrentContext = contextName
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load Kubernetes configuration: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("initialize cluster source: %w", err)
	}
	reader, discoveryClient, err := newClients(restConfig)
	if err != nil {
		return nil, fmt.Errorf("initialize cluster source: %w", err)
	}
	if reader == nil || discoveryClient == nil {
		return nil, fmt.Errorf("initialize cluster source: client factory returned incomplete clients")
	}
	rawConfig, err := clientConfig.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("read Kubernetes context: %w", err)
	}
	if contextName == "" {
		contextName = rawConfig.CurrentContext
	}
	return &ClusterSource{
		Reader: reader, Discovery: discoveryClient,
		Context: contextName, Namespace: namespace,
	}, nil
}

func (s *ClusterSource) Snapshot(ctx context.Context) (Snapshot, error) {
	if s.Reader == nil || s.Discovery == nil {
		return Snapshot{}, fmt.Errorf("load cluster source: reader and discovery client are required")
	}
	namespace := s.Namespace
	if namespace == "" {
		namespace = metav1.NamespaceAll
	}
	snapshot := Snapshot{
		Metadata: model.SourceSnapshot{
			Mode: model.SourceModeCluster, Context: s.Context, Namespace: s.Namespace,
		},
	}

	if err := ctx.Err(); err != nil {
		return Snapshot{}, &Error{Mode: ModeCluster, Err: err}
	}
	istioAvailable, istioReason, err := kube.DiscoverGroupVersion(ctx, s.Discovery, "networking.istio.io/v1")
	if err != nil {
		return Snapshot{}, &Error{Mode: ModeCluster, Err: err}
	}
	snapshot.Metadata.Families = append(snapshot.Metadata.Families, model.ResourceFamilyStatus{
		Family: "istio", Available: istioAvailable, Reason: istioReason,
	})
	if istioAvailable {
		if err := s.appendIstioResources(ctx, namespace, &snapshot.Resources); err != nil {
			return Snapshot{}, &Error{Mode: ModeCluster, Err: err}
		}
	}

	gatewayAvailable, gatewayReason, err := kube.DiscoverGroupVersion(ctx, s.Discovery, "gateway.networking.k8s.io/v1")
	if err != nil {
		return Snapshot{}, &Error{Mode: ModeCluster, Err: err}
	}
	snapshot.Metadata.Families = append(snapshot.Metadata.Families, model.ResourceFamilyStatus{
		Family: "gateway-api", Available: gatewayAvailable, Reason: gatewayReason,
	})
	if gatewayAvailable {
		if err := s.appendGatewayResources(ctx, namespace, &snapshot.Resources); err != nil {
			return Snapshot{}, &Error{Mode: ModeCluster, Err: err}
		}
	}

	referenceGrantAvailable, reason, err := kube.DiscoverGroupVersion(ctx, s.Discovery, "gateway.networking.k8s.io/v1beta1")
	if err != nil {
		return Snapshot{}, &Error{Mode: ModeCluster, Err: err}
	}
	snapshot.Metadata.Families = append(snapshot.Metadata.Families, model.ResourceFamilyStatus{
		Family: "gateway-api-referencegrant", Available: referenceGrantAvailable, Reason: reason,
	})
	if referenceGrantAvailable {
		grants, listErr := s.Reader.ListReferenceGrants(ctx, namespace, metav1.ListOptions{})
		if listErr != nil {
			if !isMissingAPI(listErr) {
				return Snapshot{}, &Error{Mode: ModeCluster, Err: fmt.Errorf("list ReferenceGrants: %w", listErr)}
			}
			snapshot.Metadata.Families[len(snapshot.Metadata.Families)-1].Available = false
			snapshot.Metadata.Families[len(snapshot.Metadata.Families)-1].Reason = listErr.Error()
		} else {
			for i := range grants.Items {
				object := &grants.Items[i]
				snapshot.Resources = append(snapshot.Resources, resourceFromObject(
					schema.GroupVersion{
						Group: gatewayv1beta1.GroupVersion.Group, Version: gatewayv1beta1.GroupVersion.Version,
					}, "ReferenceGrant", object,
				))
			}
		}
	}
	if err := snapshot.Validate(); err != nil {
		return Snapshot{}, &Error{Mode: ModeCluster, Err: err}
	}
	return snapshot, nil
}

func (s *ClusterSource) appendIstioResources(ctx context.Context, namespace string, resources *[]Resource) error {
	gatewayList, err := s.Reader.ListIstioGateways(ctx, namespace, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list Istio Gateways: %w", err)
	}
	for i := range gatewayList.Items {
		*resources = append(*resources, resourceFromObject(istiov1.SchemeGroupVersion, "Gateway", gatewayList.Items[i]))
	}
	virtualServiceList, err := s.Reader.ListVirtualServices(ctx, namespace, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list Istio VirtualServices: %w", err)
	}
	for i := range virtualServiceList.Items {
		*resources = append(*resources, resourceFromObject(istiov1.SchemeGroupVersion, "VirtualService", virtualServiceList.Items[i]))
	}
	destinationRuleList, err := s.Reader.ListDestinationRules(ctx, namespace, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list Istio DestinationRules: %w", err)
	}
	for i := range destinationRuleList.Items {
		*resources = append(*resources, resourceFromObject(istiov1.SchemeGroupVersion, "DestinationRule", destinationRuleList.Items[i]))
	}
	serviceEntryList, err := s.Reader.ListServiceEntries(ctx, namespace, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list Istio ServiceEntries: %w", err)
	}
	for i := range serviceEntryList.Items {
		*resources = append(*resources, resourceFromObject(istiov1.SchemeGroupVersion, "ServiceEntry", serviceEntryList.Items[i]))
	}
	return nil
}

func (s *ClusterSource) appendGatewayResources(ctx context.Context, namespace string, resources *[]Resource) error {
	classes, err := s.Reader.ListGatewayClasses(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list GatewayClasses: %w", err)
	}
	for i := range classes.Items {
		*resources = append(*resources, resourceFromObject(schema.GroupVersion{
			Group: gatewayv1.GroupVersion.Group, Version: gatewayv1.GroupVersion.Version,
		}, "GatewayClass", &classes.Items[i]))
	}
	gateways, err := s.Reader.ListGateways(ctx, namespace, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list Gateway API Gateways: %w", err)
	}
	for i := range gateways.Items {
		*resources = append(*resources, resourceFromObject(schema.GroupVersion{
			Group: gatewayv1.GroupVersion.Group, Version: gatewayv1.GroupVersion.Version,
		}, "Gateway", &gateways.Items[i]))
	}
	routes, err := s.Reader.ListHTTPRoutes(ctx, namespace, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list HTTPRoutes: %w", err)
	}
	for i := range routes.Items {
		*resources = append(*resources, resourceFromObject(schema.GroupVersion{
			Group: gatewayv1.GroupVersion.Group, Version: gatewayv1.GroupVersion.Version,
		}, "HTTPRoute", &routes.Items[i]))
	}
	return nil
}

func isMissingAPI(err error) bool {
	return apierrors.IsNotFound(err) || apierrors.IsMethodNotSupported(err)
}

type apiResourceObject interface {
	metav1.Object
	runtime.Object
}

func resourceFromObject(gv schema.GroupVersion, kind string, object apiResourceObject) Resource {
	version := gv.Version
	return Resource{
		Identity: model.ResourceIdentity{
			Group: gv.Group, Version: version, Kind: kind,
			Namespace: object.GetNamespace(), Name: object.GetName(),
		},
		GVK:    gv.WithKind(kind),
		Object: object,
	}
}
