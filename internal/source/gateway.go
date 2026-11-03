package source

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ninoamine/istio-route/internal/kube"
	"github.com/ninoamine/istio-route/internal/model"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

const waypointLabel = "istio.io/use-waypoint"

var ErrGatewayNotFound = errors.New("gateway not found")

// GatewaySnapshot loads the selected Gateway and only the resources linked to it.
func (s *ClusterSource) GatewaySnapshot(ctx context.Context, gatewayName, namespace string) (Snapshot, error) {
	if s.Reader == nil || s.Discovery == nil {
		return Snapshot{}, fmt.Errorf("load Gateway snapshot: reader and discovery client are required")
	}
	if namespace == "" {
		namespace = s.Namespace
	}
	if namespace == "" {
		return Snapshot{}, fmt.Errorf("gateway namespace is required")
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, fmt.Errorf("load Gateway snapshot: %w", err)
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
			Families: []model.ResourceFamilyStatus{{Family: "gateway-api", Available: true}},
		},
	}
	gateway, err := s.Reader.GetGateway(ctx, namespace, gatewayName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return Snapshot{}, fmt.Errorf("%w: %s/%s", ErrGatewayNotFound, namespace, gatewayName)
		}
		return Snapshot{}, fmt.Errorf("get selected Gateway: %w", err)
	}
	gatewayGV := schema.GroupVersion{Group: gatewayv1.GroupVersion.Group, Version: gatewayv1.GroupVersion.Version}
	result.Resources = append(result.Resources, resourceFromObject(gatewayGV, "Gateway", gateway))
	class, err := s.Reader.GetGatewayClass(ctx, string(gateway.Spec.GatewayClassName), metav1.GetOptions{})
	if err != nil {
		return Snapshot{}, fmt.Errorf("get GatewayClass %q: %w", gateway.Spec.GatewayClassName, err)
	}
	result.Resources = append(result.Resources, resourceFromObject(gatewayGV, "GatewayClass", class))

	httpAvailable, httpReason, err := kube.DiscoverGroupVersion(ctx, s.Discovery, gatewayv1.GroupVersion.String())
	if err != nil {
		return Snapshot{}, fmt.Errorf("discover HTTPRoute API: %w", err)
	}
	grpcAvailable, grpcReason, err := kube.DiscoverGroupVersion(ctx, s.Discovery, gatewayv1.GroupVersion.String())
	if err != nil {
		return Snapshot{}, fmt.Errorf("discover GRPCRoute API: %w", err)
	}
	result.Metadata.Families = append(result.Metadata.Families,
		model.ResourceFamilyStatus{Family: "httproute", Available: httpAvailable, Reason: httpReason},
		model.ResourceFamilyStatus{Family: "grpcroute", Available: grpcAvailable, Reason: grpcReason},
	)
	parent := model.ResourceIdentity{
		Group: gatewayv1.GroupVersion.Group, Version: gatewayv1.GroupVersion.Version,
		Kind: "Gateway", Namespace: namespace, Name: gatewayName,
	}
	var backendRefs []backendLookup
	if httpAvailable {
		routes, listErr := s.Reader.ListHTTPRoutes(ctx, metav1.NamespaceAll, metav1.ListOptions{})
		if listErr != nil && !isMissingAPI(listErr) {
			return Snapshot{}, fmt.Errorf("list HTTPRoutes: %w", listErr)
		}
		if listErr != nil {
			result.Metadata.Families[1].Available = false
			result.Metadata.Families[1].Reason = listErr.Error()
		} else {
			for i := range routes.Items {
				route := &routes.Items[i]
				if !httpRouteTargets(route, parent) {
					continue
				}
				result.Resources = append(result.Resources, resourceFromObject(gatewayGV, "HTTPRoute", route))
				backendRefs = append(backendRefs, httpBackendLookups(route)...)
			}
		}
	}
	if grpcAvailable {
		routes, listErr := s.Reader.ListGRPCRoutes(ctx, metav1.NamespaceAll, metav1.ListOptions{})
		if listErr != nil && !isMissingAPI(listErr) {
			return Snapshot{}, fmt.Errorf("list GRPCRoutes: %w", listErr)
		}
		if listErr != nil {
			result.Metadata.Families[2].Available = false
			result.Metadata.Families[2].Reason = listErr.Error()
		} else {
			for i := range routes.Items {
				route := &routes.Items[i]
				if !grpcRouteTargets(route, parent) {
					continue
				}
				result.Resources = append(result.Resources, resourceFromObject(gatewayGV, "GRPCRoute", route))
				backendRefs = append(backendRefs, grpcBackendLookups(route)...)
			}
		}
	}
	backendNamespaces := make(map[string]struct{})
	seenServices := make(map[string]struct{})
	endpointsAvailable, endpointsReason, err := kube.DiscoverGroupVersion(ctx, s.Discovery, "discovery.k8s.io/v1")
	if err != nil {
		return Snapshot{}, fmt.Errorf("discover EndpointSlice API: %w", err)
	}
	result.Metadata.Families = append(result.Metadata.Families, model.ResourceFamilyStatus{
		Family: "endpointslices", Available: endpointsAvailable, Reason: endpointsReason,
	})
	for _, ref := range backendRefs {
		if ref.group == "" && (ref.kind == "" || ref.kind == "Service") {
			serviceKey := ref.namespace + "/" + ref.name
			if _, found := seenServices[serviceKey]; !found {
				seenServices[serviceKey] = struct{}{}
				service, getErr := s.Reader.GetService(ctx, ref.namespace, ref.name, metav1.GetOptions{})
				if getErr == nil {
					result.Resources = append(result.Resources, resourceFromObject(schema.GroupVersion{Version: "v1"}, "Service", service))
					if endpointsAvailable {
						slices, listErr := s.Reader.ListEndpointSlices(ctx, ref.namespace, metav1.ListOptions{
							LabelSelector: "kubernetes.io/service-name=" + ref.name,
						})
						if listErr != nil && !isMissingAPI(listErr) {
							return Snapshot{}, fmt.Errorf("list EndpointSlices for Service %s/%s: %w", ref.namespace, ref.name, listErr)
						}
						if listErr != nil {
							result.Metadata.Families[len(result.Metadata.Families)-1].Available = false
							result.Metadata.Families[len(result.Metadata.Families)-1].Reason = listErr.Error()
						} else {
							for i := range slices.Items {
								result.Resources = append(result.Resources, resourceFromObject(
									schema.GroupVersion{Group: "discovery.k8s.io", Version: "v1"},
									"EndpointSlice", &slices.Items[i],
								))
							}
						}
					}
				} else if !apierrors.IsNotFound(getErr) {
					return Snapshot{}, fmt.Errorf("get backend Service %s/%s: %w", ref.namespace, ref.name, getErr)
				}
			}
		}
		if ref.namespace != "" && ref.namespace != ref.routeNamespace {
			backendNamespaces[ref.namespace] = struct{}{}
		}
	}
	grantAvailable, grantReason, err := kube.DiscoverGroupVersion(ctx, s.Discovery, gatewayv1beta1.GroupVersion.String())
	if err != nil {
		return Snapshot{}, fmt.Errorf("discover ReferenceGrant API: %w", err)
	} else {
		result.Metadata.Families = append(result.Metadata.Families, model.ResourceFamilyStatus{
			Family: "referencegrant", Available: grantAvailable, Reason: grantReason,
		})
		if grantAvailable {
			namespaces := make([]string, 0, len(backendNamespaces))
			for namespace := range backendNamespaces {
				namespaces = append(namespaces, namespace)
			}
			sort.Strings(namespaces)
			for _, namespace := range namespaces {
				grants, listErr := s.Reader.ListReferenceGrants(ctx, namespace, metav1.ListOptions{})
				if listErr != nil && !isMissingAPI(listErr) {
					return Snapshot{}, fmt.Errorf("list ReferenceGrants in %s: %w", namespace, listErr)
				}
				if listErr != nil {
					family := len(result.Metadata.Families) - 1
					result.Metadata.Families[family].Available = false
					result.Metadata.Families[family].Reason = listErr.Error()
				}
				if listErr == nil {
					sort.Slice(grants.Items, func(i, j int) bool {
						return grants.Items[i].Name < grants.Items[j].Name
					})
					for i := range grants.Items {
						result.Resources = append(result.Resources, resourceFromObject(
							schema.GroupVersion{
								Group:   gatewayv1beta1.GroupVersion.Group,
								Version: gatewayv1beta1.GroupVersion.Version,
							},
							"ReferenceGrant", &grants.Items[i],
						))
					}
				}
			}
		}
	}
	if string(gateway.Spec.GatewayClassName) == "istio-waypoint" {
		if err := s.appendWaypointSelectors(ctx, namespace, gatewayName, &result.Resources); err != nil {
			return Snapshot{}, err
		}
	}
	if err := result.Validate(); err != nil {
		return Snapshot{}, fmt.Errorf("validate Gateway snapshot: %w", err)
	}
	return result, nil
}

type backendLookup struct {
	routeNamespace string
	namespace      string
	name           string
	group          string
	kind           string
}

func httpBackendLookups(route *gatewayv1.HTTPRoute) []backendLookup {
	var lookups []backendLookup
	for _, rule := range route.Spec.Rules {
		for _, backend := range rule.BackendRefs {
			lookups = append(lookups, backendLookupFromRef(route.Namespace, backend.BackendObjectReference))
		}
	}
	return lookups
}

func grpcBackendLookups(route *gatewayv1.GRPCRoute) []backendLookup {
	var lookups []backendLookup
	for _, rule := range route.Spec.Rules {
		for _, backend := range rule.BackendRefs {
			lookups = append(lookups, backendLookupFromRef(route.Namespace, backend.BackendObjectReference))
		}
	}
	return lookups
}

func backendLookupFromRef(routeNamespace string, ref gatewayv1.BackendObjectReference) backendLookup {
	namespace := routeNamespace
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}
	group := ""
	if ref.Group != nil {
		group = string(*ref.Group)
	}
	kind := ""
	if ref.Kind != nil {
		kind = string(*ref.Kind)
	}
	return backendLookup{
		routeNamespace: routeNamespace, namespace: namespace,
		name: string(ref.Name), group: group, kind: kind,
	}
}

func httpRouteTargets(route *gatewayv1.HTTPRoute, parent model.ResourceIdentity) bool {
	for _, ref := range route.Spec.ParentRefs {
		if parentReferenceTargets(ref, route.Namespace, parent) {
			return true
		}
	}
	return false
}

func grpcRouteTargets(route *gatewayv1.GRPCRoute, parent model.ResourceIdentity) bool {
	for _, ref := range route.Spec.ParentRefs {
		if parentReferenceTargets(ref, route.Namespace, parent) {
			return true
		}
	}
	return false
}

func parentReferenceTargets(ref gatewayv1.ParentReference, routeNamespace string, parent model.ResourceIdentity) bool {
	group := gatewayv1.Group(gatewayv1.GroupVersion.Group)
	if ref.Group != nil {
		group = *ref.Group
	}
	kind := gatewayv1.Kind("Gateway")
	if ref.Kind != nil {
		kind = *ref.Kind
	}
	namespace := routeNamespace
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}
	return string(group) == parent.Group && string(kind) == parent.Kind &&
		namespace == parent.Namespace && string(ref.Name) == parent.Name
}

func (s *ClusterSource) appendWaypointSelectors(
	ctx context.Context,
	namespace, name string,
	resources *[]Resource,
) error {
	values := []string{name, namespace + "/" + name}
	selector := waypointLabel + " in (" + strings.Join(values, ",") + ")"
	namespaces, err := s.Reader.ListNamespaces(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return fmt.Errorf("list waypoint-selecting Namespaces: %w", err)
	}
	for i := range namespaces.Items {
		*resources = append(*resources, resourceFromObject(schema.GroupVersion{Version: "v1"}, "Namespace", &namespaces.Items[i]))
	}
	services, err := s.Reader.ListServices(ctx, metav1.NamespaceAll, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return fmt.Errorf("list waypoint-selecting Services: %w", err)
	}
	for i := range services.Items {
		*resources = append(*resources, resourceFromObject(schema.GroupVersion{Version: "v1"}, "Service", &services.Items[i]))
	}
	return nil
}
