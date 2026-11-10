package source

import (
	"context"
	"fmt"

	"github.com/ninoamine/istio-route/internal/kube"
	"github.com/ninoamine/istio-route/internal/model"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// WaypointSnapshot loads the minimal read-only state needed to simulate a waypoint:
// the Gateway, its GatewayClass when present, HTTPRoutes directly parented by the
// Gateway plus the selected route (when routeName is set), and Namespaces only when
// a listener selects routes by namespace label.
func (s *ClusterSource) WaypointSnapshot(
	ctx context.Context, name, namespace, routeNamespace, routeName string,
) (Snapshot, error) {
	if s.Reader == nil || s.Discovery == nil {
		return Snapshot{}, fmt.Errorf("load waypoint snapshot: reader and discovery client are required")
	}
	if namespace == "" {
		namespace = s.Namespace
	}
	if namespace == "" {
		return Snapshot{}, fmt.Errorf("waypoint namespace is required")
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, fmt.Errorf("load waypoint snapshot: %w", err)
	}
	available, reason, err := kube.DiscoverGroupVersion(ctx, s.Discovery, gatewayv1.GroupVersion.String())
	if err != nil {
		return Snapshot{}, fmt.Errorf("discover Gateway API: %w", err)
	}
	if !available {
		return Snapshot{}, fmt.Errorf("gateway API is unavailable: %s", reason)
	}
	result := Snapshot{
		Metadata: model.SourceSnapshot{
			Mode: model.SourceModeCluster, Context: s.Context, Namespace: namespace,
			Families: []model.ResourceFamilyStatus{
				{Family: "gateway-api", Available: true},
				{Family: "httproute", Available: true},
			},
		},
	}
	gatewayGV := schema.GroupVersion{Group: gatewayv1.GroupVersion.Group, Version: gatewayv1.GroupVersion.Version}

	gateway, err := s.Reader.GetGateway(ctx, namespace, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return Snapshot{}, fmt.Errorf("%w: %s/%s", ErrGatewayNotFound, namespace, name)
		}
		return Snapshot{}, fmt.Errorf("get waypoint Gateway: %w", err)
	}
	result.Resources = append(result.Resources, resourceFromObject(gatewayGV, "Gateway", gateway))

	class, err := s.Reader.GetGatewayClass(ctx, string(gateway.Spec.GatewayClassName), metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		// A missing class is reported by the simulation as unconfirmed, not as a failure.
	case err != nil:
		return Snapshot{}, fmt.Errorf("get GatewayClass %q: %w", gateway.Spec.GatewayClassName, err)
	default:
		result.Resources = append(result.Resources, resourceFromObject(gatewayGV, "GatewayClass", class))
	}

	parent := model.ResourceIdentity{
		Group: gatewayv1.GroupVersion.Group, Version: gatewayv1.GroupVersion.Version,
		Kind: "Gateway", Namespace: namespace, Name: name,
	}
	routes, err := s.Reader.ListHTTPRoutes(ctx, metav1.NamespaceAll, metav1.ListOptions{})
	if err != nil {
		return Snapshot{}, fmt.Errorf("list HTTPRoutes: %w", err)
	}
	for i := range routes.Items {
		route := &routes.Items[i]
		selected := routeName != "" && route.Namespace == routeNamespace && route.Name == routeName
		if !selected && !httpRouteTargets(route, parent) {
			continue
		}
		result.Resources = append(result.Resources, resourceFromObject(gatewayGV, "HTTPRoute", route))
	}

	if listenersUseNamespaceSelector(gateway) {
		namespaces, listErr := s.Reader.ListNamespaces(ctx, metav1.ListOptions{})
		if listErr != nil {
			return Snapshot{}, fmt.Errorf("list Namespaces: %w", listErr)
		}
		coreGV := schema.GroupVersion{Version: "v1"}
		for i := range namespaces.Items {
			result.Resources = append(result.Resources, resourceFromObject(coreGV, "Namespace", &namespaces.Items[i]))
		}
	}
	return result, nil
}

func listenersUseNamespaceSelector(gateway *gatewayv1.Gateway) bool {
	for _, listener := range gateway.Spec.Listeners {
		if listener.AllowedRoutes == nil || listener.AllowedRoutes.Namespaces == nil {
			continue
		}
		if listener.AllowedRoutes.Namespaces.From != nil &&
			*listener.AllowedRoutes.Namespaces.From == gatewayv1.NamespacesFromSelector {
			return true
		}
	}
	return false
}
