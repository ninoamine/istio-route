package diagnose

import (
	"fmt"
	"strings"

	"github.com/ninoamine/istio-route/internal/model"
	"github.com/ninoamine/istio-route/internal/resolve"
	"github.com/ninoamine/istio-route/internal/source"
	istioclientv1 "istio.io/client-go/pkg/apis/networking/v1"
)

func diagnoseIstioBindings(analysis *model.Analysis, snapshot source.Snapshot, add addFindingFunc) {
	gateways := make(map[string]*istioclientv1.Gateway)
	for _, resource := range snapshot.Resources {
		switch object := resource.Object.(type) {
		case *istioclientv1.Gateway:
			gateways[resource.Identity.Namespace+"/"+resource.Identity.Name] = object
		}
	}

	for _, resource := range snapshot.Resources {
		virtualService, ok := resource.Object.(*istioclientv1.VirtualService)
		if !ok || !istioVirtualServiceMatches(analysis.Hostname, virtualService) {
			continue
		}
		if len(virtualService.Spec.Gateways) == 0 {
			continue
		}
		virtualServiceIdentity := resource.Identity
		for _, reference := range virtualService.Spec.Gateways {
			if reference == "mesh" {
				continue
			}
			namespace, name := istioGatewayReference(virtualService.Namespace, reference)
			gateway := gateways[namespace+"/"+name]
			if gateway == nil {
				addWarning(
					add, "unbound-virtualservice",
					fmt.Sprintf("VirtualService %s/%s references missing Istio Gateway %s/%s", virtualService.Namespace, virtualService.Name, namespace, name),
					virtualServiceIdentity,
					model.ResourceIdentity{
						Group: "networking.istio.io", Version: "v1", Kind: "Gateway",
						Namespace: namespace, Name: name,
					},
				)
				continue
			}
			if !istioGatewayMatchesVirtualService(analysis.Hostname, gateway, virtualService) {
				addWarning(
					add, "unbound-virtualservice",
					fmt.Sprintf("VirtualService %s/%s is not bound: Gateway %s/%s has no server for hostname %s", virtualService.Namespace, virtualService.Name, namespace, name, analysis.Hostname),
					virtualServiceIdentity,
					model.ResourceIdentity{
						Group: "networking.istio.io", Version: "v1", Kind: "Gateway",
						Namespace: namespace, Name: name,
					},
				)
			}
		}
	}

	for _, route := range analysis.Routes {
		if route.Origin != model.OriginIstio {
			continue
		}
		for _, rule := range route.Rules {
			for _, destination := range rule.Destinations {
				if destination.Subset == "" || destination.DestinationRule != nil {
					continue
				}
				addWarning(
					add, "missing-destination-rule-subset",
					fmt.Sprintf(
						"destination %s/%s selects subset %s but no matching DestinationRule defines it",
						destination.Namespace, destination.Name, destination.Subset,
					),
					route.Resource,
				)
			}
		}
	}
}

func istioVirtualServiceMatches(hostname string, virtualService *istioclientv1.VirtualService) bool {
	for _, host := range virtualService.Spec.Hosts {
		if resolve.MatchesIstioHost(hostname, virtualService.Namespace, host) {
			return true
		}
	}
	return false
}

func istioGatewayReference(namespace, reference string) (string, string) {
	parts := strings.SplitN(reference, "/", 2)
	if len(parts) == 1 {
		return namespace, parts[0]
	}
	return parts[0], parts[1]
}

func istioGatewayMatchesVirtualService(
	hostname string,
	gateway *istioclientv1.Gateway,
	virtualService *istioclientv1.VirtualService,
) bool {
	for _, server := range gateway.Spec.Servers {
		if server == nil {
			continue
		}
		for _, serverHost := range server.Hosts {
			if !resolve.MatchesIstioHost(hostname, gateway.Namespace, serverHost) {
				continue
			}
			for _, routeHost := range virtualService.Spec.Hosts {
				if resolve.MatchesIstioHost(hostname, virtualService.Namespace, routeHost) {
					return true
				}
			}
		}
	}
	return false
}
