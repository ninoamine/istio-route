package simulate

import (
	"errors"
	"fmt"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/ninoamine/istio-route/internal/source"
)

const (
	gatewayAPIGroup = "gateway.networking.k8s.io"
	gatewayKind     = "Gateway"
	httpRouteKind   = "HTTPRoute"
)

// ValidateLocalNamespaces rejects local manifests whose waypoint Gateway, selected
// HTTPRoute, or waypoint-attached HTTPRoutes omit metadata.namespace. The simulation
// never guesses a namespace for these resources.
func ValidateLocalNamespaces(snapshot source.Snapshot, waypointName string, route *RouteRef) error {
	var errs []error
	for _, res := range snapshot.Resources {
		id := res.Identity
		if id.Namespace != "" || id.Group != gatewayAPIGroup {
			continue
		}
		switch {
		case id.Kind == gatewayKind && id.Name == waypointName:
			errs = append(errs, missingNamespace(id.Kind, id.Name))
		case id.Kind == httpRouteKind && routeSelected(id.Name, route):
			errs = append(errs, missingNamespace(id.Kind, id.Name))
		case id.Kind == httpRouteKind && routeAttachedTo(res, waypointName):
			errs = append(errs, missingNamespace(id.Kind, id.Name))
		}
	}
	return errors.Join(errs...)
}

// RequireGatewayAPI fails when the snapshot contains no Gateway API resources, so
// Istio-only manifests are not reported as a missing waypoint.
func RequireGatewayAPI(snapshot source.Snapshot) error {
	for _, res := range snapshot.Resources {
		if res.Identity.Group == gatewayAPIGroup {
			return nil
		}
	}
	return errors.New("no Gateway API resources were supplied")
}

func missingNamespace(kind, name string) error {
	return fmt.Errorf("%s %q: missing namespace", kind, name)
}

func routeSelected(name string, route *RouteRef) bool {
	return route != nil && name == route.Name
}

// routeAttachedTo reports whether an HTTPRoute has a parentRef naming the waypoint Gateway.
// Group and kind default to the Gateway API Gateway, as in audit.ParentTargets.
func routeAttachedTo(res source.Resource, waypointName string) bool {
	route, ok := res.Object.(*gatewayv1.HTTPRoute)
	if !ok {
		return false
	}
	for _, ref := range route.Spec.ParentRefs {
		if string(ref.Name) != waypointName {
			continue
		}
		if ref.Group != nil && string(*ref.Group) != gatewayAPIGroup {
			continue
		}
		if ref.Kind != nil && string(*ref.Kind) != gatewayKind {
			continue
		}
		return true
	}
	return false
}
