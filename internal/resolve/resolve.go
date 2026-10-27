package resolve

import (
	"context"
	"fmt"
	"sort"

	"github.com/ninoamine/istio-route/internal/kube"
	"github.com/ninoamine/istio-route/internal/model"
	"github.com/ninoamine/istio-route/internal/source"
	istioclientv1 "istio.io/client-go/pkg/apis/networking/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type catalog struct {
	istioGateways    []*istioclientv1.Gateway
	virtualServices  []*istioclientv1.VirtualService
	destinationRules []*istioclientv1.DestinationRule
	serviceEntries   []*istioclientv1.ServiceEntry
	gatewayClasses   []*gatewayv1.GatewayClass
	gateways         []*gatewayv1.Gateway
	httpRoutes       []*gatewayv1.HTTPRoute
	referenceGrants  []*gatewayv1beta1.ReferenceGrant
}

func Resolve(ctx context.Context, hostname string, snapshot source.Snapshot, reader kube.EndpointReader) (model.Analysis, error) {
	if hostname == "" {
		return model.Analysis{}, fmt.Errorf("resolve hostname: hostname is required")
	}
	if err := snapshot.Validate(); err != nil {
		return model.Analysis{}, fmt.Errorf("resolve hostname %q: %w", hostname, err)
	}
	analysis := model.Analysis{
		Hostname: hostname,
		Source:   snapshot.Metadata,
	}
	resources := catalog{}
	for _, resource := range snapshot.Resources {
		switch object := resource.Object.(type) {
		case *istioclientv1.Gateway:
			resources.istioGateways = append(resources.istioGateways, object)
		case *istioclientv1.VirtualService:
			resources.virtualServices = append(resources.virtualServices, object)
		case *istioclientv1.DestinationRule:
			resources.destinationRules = append(resources.destinationRules, object)
		case *istioclientv1.ServiceEntry:
			resources.serviceEntries = append(resources.serviceEntries, object)
		case *gatewayv1.GatewayClass:
			resources.gatewayClasses = append(resources.gatewayClasses, object)
		case *gatewayv1.Gateway:
			resources.gateways = append(resources.gateways, object)
		case *gatewayv1.HTTPRoute:
			resources.httpRoutes = append(resources.httpRoutes, object)
		case *gatewayv1beta1.ReferenceGrant:
			resources.referenceGrants = append(resources.referenceGrants, object)
		}
	}
	sortCatalog(&resources)
	endpoints := newEndpointResolver(reader)

	if err := resolveIstio(ctx, hostname, resources, endpoints, &analysis); err != nil {
		return model.Analysis{}, err
	}
	if err := resolveGatewayAPI(ctx, hostname, resources, endpoints, &analysis); err != nil {
		return model.Analysis{}, err
	}

	sort.SliceStable(analysis.EntryPoints, func(i, j int) bool {
		first, second := analysis.EntryPoints[i], analysis.EntryPoints[j]
		if first.Origin != second.Origin {
			return first.Origin == model.OriginIstio
		}
		if first.Resource.Namespace != second.Resource.Namespace {
			return first.Resource.Namespace < second.Resource.Namespace
		}
		if first.Resource.Name != second.Resource.Name {
			return first.Resource.Name < second.Resource.Name
		}
		return first.Name < second.Name
	})
	sort.SliceStable(analysis.Routes, func(i, j int) bool {
		first, second := analysis.Routes[i], analysis.Routes[j]
		if first.Origin != second.Origin {
			return first.Origin == model.OriginIstio
		}
		if first.Resource.Namespace != second.Resource.Namespace {
			return first.Resource.Namespace < second.Resource.Namespace
		}
		return first.Resource.Name < second.Resource.Name
	})
	return analysis, nil
}

func sortCatalog(resources *catalog) {
	sort.Slice(resources.istioGateways, func(i, j int) bool {
		return namespacedName(resources.istioGateways[i].Namespace, resources.istioGateways[i].Name) <
			namespacedName(resources.istioGateways[j].Namespace, resources.istioGateways[j].Name)
	})
	sort.Slice(resources.virtualServices, func(i, j int) bool {
		return namespacedName(resources.virtualServices[i].Namespace, resources.virtualServices[i].Name) <
			namespacedName(resources.virtualServices[j].Namespace, resources.virtualServices[j].Name)
	})
	sort.Slice(resources.destinationRules, func(i, j int) bool {
		return namespacedName(resources.destinationRules[i].Namespace, resources.destinationRules[i].Name) <
			namespacedName(resources.destinationRules[j].Namespace, resources.destinationRules[j].Name)
	})
	sort.Slice(resources.serviceEntries, func(i, j int) bool {
		return namespacedName(resources.serviceEntries[i].Namespace, resources.serviceEntries[i].Name) <
			namespacedName(resources.serviceEntries[j].Namespace, resources.serviceEntries[j].Name)
	})
	sort.Slice(resources.gatewayClasses, func(i, j int) bool {
		return resources.gatewayClasses[i].Name < resources.gatewayClasses[j].Name
	})
	sort.Slice(resources.gateways, func(i, j int) bool {
		return namespacedName(resources.gateways[i].Namespace, resources.gateways[i].Name) <
			namespacedName(resources.gateways[j].Namespace, resources.gateways[j].Name)
	})
	sort.Slice(resources.httpRoutes, func(i, j int) bool {
		return namespacedName(resources.httpRoutes[i].Namespace, resources.httpRoutes[i].Name) <
			namespacedName(resources.httpRoutes[j].Namespace, resources.httpRoutes[j].Name)
	})
	sort.Slice(resources.referenceGrants, func(i, j int) bool {
		return namespacedName(resources.referenceGrants[i].Namespace, resources.referenceGrants[i].Name) <
			namespacedName(resources.referenceGrants[j].Namespace, resources.referenceGrants[j].Name)
	})
}

func namespacedName(namespace, name string) string {
	return namespace + "/" + name
}

func identity(group, version, kind, namespace, name string) model.ResourceIdentity {
	return model.ResourceIdentity{
		Group: group, Version: version, Kind: kind, Namespace: namespace, Name: name,
	}
}
