package audit

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/ninoamine/istio-route/internal/model"
	"github.com/ninoamine/istio-route/internal/source"
	"k8s.io/apimachinery/pkg/runtime"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func AnalyzeFiles(
	ctx context.Context,
	gatewayName, namespace string,
	files []string,
	stdin io.Reader,
) (Result, Outcome, error) {
	filesSource := &source.FilesSource{Files: files, Stdin: stdin}
	snapshot, err := filesSource.Snapshot(ctx)
	if err != nil {
		return Result{}, "", fmt.Errorf("read Gateway audit manifests: %w", err)
	}
	return Analyze(ctx, Request{
		GatewayName: gatewayName, Namespace: namespace, Source: snapshot.Metadata,
	}, snapshot)
}

func selectGateway(snapshot source.Snapshot, name, namespace string) (*source.Resource, error) {
	var candidates []*source.Resource
	for i := range snapshot.Resources {
		resource := &snapshot.Resources[i]
		if resource.Identity.Group == gatewayAPIGroup &&
			resource.Identity.Kind == "Gateway" && resource.Identity.Name == name &&
			(namespace == "" || resource.Identity.Namespace == namespace) {
			candidates = append(candidates, resource)
		}
	}
	if len(candidates) > 1 {
		namespaces := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			namespaces = append(namespaces, candidate.Identity.Namespace)
		}
		sort.Strings(namespaces)
		return nil, fmt.Errorf("%w %q across namespaces: %s", ErrAmbiguousGateway, name, strings.Join(namespaces, ", "))
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	return candidates[0], nil
}

func linkedRoutes(snapshot source.Snapshot, gateway model.ResourceIdentity) []source.Resource {
	var routes []source.Resource
	for _, resource := range snapshot.Resources {
		if resource.Identity.Group != gatewayAPIGroup ||
			(resource.Identity.Kind != "HTTPRoute" && resource.Identity.Kind != "GRPCRoute") {
			continue
		}
		if routeTargets(resource.Object, resource.Identity.Namespace, gateway) {
			routes = append(routes, resource)
		}
	}
	sort.Slice(routes, func(i, j int) bool {
		return identityKey(routes[i].Identity) < identityKey(routes[j].Identity)
	})
	return routes
}

func routeTargets(object runtime.Object, routeNamespace string, gateway model.ResourceIdentity) bool {
	var refs []gatewayv1.ParentReference
	switch route := object.(type) {
	case *gatewayv1.HTTPRoute:
		refs = route.Spec.ParentRefs
	case *gatewayv1.GRPCRoute:
		refs = route.Spec.ParentRefs
	default:
		return false
	}
	for _, ref := range refs {
		if parentReferenceTargets(ref, routeNamespace, gateway) {
			return true
		}
	}
	return false
}

func parentReferenceTargets(ref gatewayv1.ParentReference, routeNamespace string, parent model.ResourceIdentity) bool {
	group := gatewayv1.Group(gatewayAPIGroup)
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
