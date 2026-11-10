package simulate

import "github.com/ninoamine/istio-route/internal/source"

// Without returns a copy of snapshot with the target HTTPRoute removed. The
// original snapshot and its resources are left untouched.
func Without(snapshot source.Snapshot, target RouteRef) source.Snapshot {
	resources := make([]source.Resource, 0, len(snapshot.Resources))
	for _, resource := range snapshot.Resources {
		if isTargetRoute(resource, target) {
			continue
		}
		resources = append(resources, resource)
	}
	snapshot.Resources = resources
	return snapshot
}

func isTargetRoute(resource source.Resource, target RouteRef) bool {
	identity := resource.Identity
	return identity.Group == gatewayAPIGroup &&
		identity.Kind == "HTTPRoute" &&
		identity.Namespace == target.Namespace &&
		identity.Name == target.Name
}
