package diagnose

import (
	"fmt"
	"strings"

	"github.com/ninoamine/istio-route/internal/model"
	"github.com/ninoamine/istio-route/internal/resolve"
	"github.com/ninoamine/istio-route/internal/source"
	corev1 "k8s.io/api/core/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func diagnoseGatewayRouteReferences(analysis *model.Analysis, snapshot source.Snapshot, add addFindingFunc) {
	gateways := make(map[string]*gatewayv1.Gateway)
	services := make(map[string]struct{})
	var grants []*gatewayv1beta1.ReferenceGrant
	var routes []*gatewayv1.HTTPRoute
	for _, resource := range snapshot.Resources {
		switch object := resource.Object.(type) {
		case *gatewayv1.Gateway:
			gateways[identityKey(resource.Identity)] = object
		case *gatewayv1.HTTPRoute:
			routes = append(routes, object)
		case *corev1.Service:
			services[resource.Identity.Namespace+"/"+resource.Identity.Name] = struct{}{}
		case *gatewayv1beta1.ReferenceGrant:
			grants = append(grants, object)
		}
	}

	for _, route := range routes {
		relevant := routeRelevantToHostname(route, analysis.Hostname, gateways)
		if !relevant {
			continue
		}
		routeIdentity := model.ResourceIdentity{
			Group: gatewayGroup, Version: "v1", Kind: "HTTPRoute",
			Namespace: route.Namespace, Name: route.Name,
		}
		parentIdentities := make([]model.ResourceIdentity, 0, len(route.Spec.ParentRefs))
		for _, parent := range route.Spec.ParentRefs {
			parentIdentity := gatewayParentIdentity(parent, route.Namespace)
			parentIdentities = append(parentIdentities, parentIdentity)
			gateway := gateways[identityKey(parentIdentity)]
			if gateway == nil {
				addWarning(
					add, "missing-parent-gateway",
					fmt.Sprintf("parent Gateway %s/%s does not exist", parentIdentity.Namespace, parentIdentity.Name),
					routeIdentity, parentIdentity,
				)
				continue
			}
			listeners := matchingParentListeners(gateway, parent)
			if parent.SectionName != nil && len(listeners) == 0 {
				addWarning(
					add, "missing-parent-listener",
					fmt.Sprintf("listener %s does not exist on parent Gateway %s/%s", *parent.SectionName, gateway.Namespace, gateway.Name),
					routeIdentity, parentIdentity,
				)
				continue
			}
			if parent.SectionName == nil && len(gateway.Spec.Listeners) == 0 {
				addWarning(
					add, "missing-parent-listener",
					fmt.Sprintf("parent Gateway %s/%s has no listeners", gateway.Namespace, gateway.Name),
					routeIdentity, parentIdentity,
				)
				continue
			}
			for _, listener := range listeners {
				if len(route.Spec.Hostnames) == 0 || listener.Hostname == nil {
					continue
				}
				matched := false
				for _, hostname := range route.Spec.Hostnames {
					if resolve.HostnamesIntersect(string(hostname), string(*listener.Hostname)) {
						matched = true
						break
					}
				}
				if !matched {
					addWarning(
						add, "hostname-intersection-failed",
						fmt.Sprintf(
							"route hostname %s does not intersect with listener %s on Gateway %s/%s",
							strings.Join(routeHostnames(route), ", "), listener.Name, gateway.Namespace, gateway.Name,
						),
						routeIdentity, parentIdentity,
					)
				}
			}
		}

		appendRouteIfMissing(analysis, route, routeIdentity, parentIdentities)
		diagnoseRouteBackends(route, routeIdentity, analysis, services, grants, add)
	}
}

func routeRelevantToHostname(
	route *gatewayv1.HTTPRoute,
	hostname string,
	gateways map[string]*gatewayv1.Gateway,
) bool {
	if len(route.Spec.Hostnames) == 0 {
		for _, parent := range route.Spec.ParentRefs {
			gateway := gateways[identityKey(gatewayParentIdentity(parent, route.Namespace))]
			if gatewayHasMatchingListener(gateway, hostname, parent) {
				return true
			}
		}
		return false
	}
	for _, routeHostname := range route.Spec.Hostnames {
		if resolve.MatchesHostname(hostname, string(routeHostname)) {
			return true
		}
	}
	for _, parent := range route.Spec.ParentRefs {
		gateway := gateways[identityKey(gatewayParentIdentity(parent, route.Namespace))]
		if gatewayHasMatchingListener(gateway, hostname, parent) {
			return true
		}
	}
	return false
}

func gatewayHasMatchingListener(gateway *gatewayv1.Gateway, hostname string, parent gatewayv1.ParentReference) bool {
	if gateway == nil {
		return false
	}
	for _, listener := range matchingParentListeners(gateway, parent) {
		if listener.Hostname == nil || resolve.MatchesHostname(hostname, string(*listener.Hostname)) {
			return true
		}
	}
	return false
}

func matchingParentListeners(gateway *gatewayv1.Gateway, parent gatewayv1.ParentReference) []gatewayv1.Listener {
	listeners := make([]gatewayv1.Listener, 0, len(gateway.Spec.Listeners))
	for _, listener := range gateway.Spec.Listeners {
		if parent.SectionName != nil && string(listener.Name) != string(*parent.SectionName) {
			continue
		}
		listeners = append(listeners, listener)
	}
	return listeners
}

func gatewayParentIdentity(parent gatewayv1.ParentReference, routeNamespace string) model.ResourceIdentity {
	return model.ResourceIdentity{
		Group: gatewayGroup, Version: "v1", Kind: "Gateway",
		Namespace: parentNamespace(parent, routeNamespace), Name: string(parent.Name),
	}
}

func appendRouteIfMissing(
	analysis *model.Analysis,
	route *gatewayv1.HTTPRoute,
	identity model.ResourceIdentity,
	parents []model.ResourceIdentity,
) {
	for _, existing := range analysis.Routes {
		if sameResource(existing.Resource, identity) {
			return
		}
	}
	hosts := make([]string, len(route.Spec.Hostnames))
	for i, hostname := range route.Spec.Hostnames {
		hosts[i] = string(hostname)
	}
	analysis.Routes = append(analysis.Routes, model.Route{
		Origin: model.OriginGatewayAPI, Resource: identity,
		Hostnames: hosts, ParentRefs: parents,
	})
}

func diagnoseRouteBackends(
	route *gatewayv1.HTTPRoute,
	routeIdentity model.ResourceIdentity,
	analysis *model.Analysis,
	services map[string]struct{},
	grants []*gatewayv1beta1.ReferenceGrant,
	add addFindingFunc,
) {
	for _, rule := range route.Spec.Rules {
		for _, backend := range rule.BackendRefs {
			ref := backend.BackendObjectReference
			namespace := route.Namespace
			if ref.Namespace != nil {
				namespace = string(*ref.Namespace)
			}
			group := ""
			if ref.Group != nil {
				group = string(*ref.Group)
			}
			kind := "Service"
			if ref.Kind != nil {
				kind = string(*ref.Kind)
			}
			name := string(ref.Name)
			if group != "" || kind != "Service" {
				addWarning(
					add, "unresolved-backend-reference",
					fmt.Sprintf("backend reference %s/%s (%s, group %q) is not a supported Service", namespace, name, kind, group),
					routeIdentity,
				)
			} else if _, exists := services[namespace+"/"+name]; !exists &&
				missingBackendCanBeConfirmed(analysis, routeIdentity, namespace, name) {
				addWarning(
					add, "unresolved-backend-reference",
					fmt.Sprintf("backend Service %s/%s does not exist", namespace, name),
					routeIdentity,
				)
			}
			if namespace != route.Namespace && !referenceGrantAllows(
				grants, route.Namespace, group, kind, namespace, name,
			) {
				addWarning(
					add, "missing-reference-grant",
					fmt.Sprintf("no ReferenceGrant in namespace %s authorizes backend %s/%s", namespace, namespace, name),
					routeIdentity,
				)
			}
		}
	}
}

func missingBackendCanBeConfirmed(
	analysis *model.Analysis,
	route model.ResourceIdentity,
	namespace, name string,
) bool {
	if analysis.Source.Mode != model.SourceModeCluster {
		return true
	}
	for _, analyzedRoute := range analysis.Routes {
		if !sameResource(analyzedRoute.Resource, route) {
			continue
		}
		for _, rule := range analyzedRoute.Rules {
			for _, destination := range rule.Destinations {
				if destination.Namespace != namespace || destination.Name != name || destination.Service == nil {
					continue
				}
				return destination.Service.Availability == model.AvailabilityNotFound
			}
		}
	}
	return false
}

func referenceGrantAllows(
	grants []*gatewayv1beta1.ReferenceGrant,
	fromNamespace, group, kind, toNamespace, name string,
) bool {
	for _, grant := range grants {
		if grant.Namespace != toNamespace {
			continue
		}
		fromAllowed := false
		for _, from := range grant.Spec.From {
			if string(from.Group) == gatewayGroup &&
				string(from.Kind) == "HTTPRoute" &&
				string(from.Namespace) == fromNamespace {
				fromAllowed = true
				break
			}
		}
		if !fromAllowed {
			continue
		}
		for _, to := range grant.Spec.To {
			if string(to.Group) != group || string(to.Kind) != kind {
				continue
			}
			if to.Name == nil || string(*to.Name) == name {
				return true
			}
		}
	}
	return false
}

func routeHostnames(route *gatewayv1.HTTPRoute) []string {
	hosts := make([]string, len(route.Spec.Hostnames))
	for i, hostname := range route.Spec.Hostnames {
		hosts[i] = string(hostname)
	}
	return hosts
}
