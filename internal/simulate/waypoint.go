package simulate

import (
	"fmt"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/ninoamine/istio-route/internal/audit"
	"github.com/ninoamine/istio-route/internal/source"
)

// ResolveWaypoint selects the ambient waypoint Gateway and collects its applicable listeners.
// A missing istio-waypoint GatewayClass leaves the waypoint unconfirmed rather than failing.
func ResolveWaypoint(snapshot source.Snapshot, name, namespace string) (Waypoint, error) {
	selected, err := audit.SelectGateway(snapshot, name, namespace)
	if err != nil {
		return Waypoint{}, fmt.Errorf("resolve waypoint %q: %w", name, err)
	}
	if selected == nil {
		return Waypoint{}, fmt.Errorf("waypoint %q: %w", name, ErrWaypointNotFound)
	}
	gateway, ok := selected.Object.(*gatewayv1.Gateway)
	if !ok {
		return Waypoint{}, fmt.Errorf("decode Gateway %q: unexpected object type %T", name, selected.Object)
	}
	if !audit.IsWaypointGateway(gateway) {
		return Waypoint{}, fmt.Errorf("gateway %s/%s uses class %q: %w",
			gateway.Namespace, gateway.Name, gateway.Spec.GatewayClassName, ErrNotWaypoint)
	}

	waypoint := Waypoint{
		Namespace:  gateway.Namespace,
		Name:       gateway.Name,
		Class:      string(gateway.Spec.GatewayClassName),
		Validation: ValidationConfirmed,
		Listeners:  applicableListeners(gateway),
	}
	if !hasGatewayClass(snapshot, waypointClassName) {
		waypoint.Validation = ValidationUnconfirmed
		waypoint.ValidationReason = fmt.Sprintf("GatewayClass %q not found in snapshot", waypointClassName)
	}
	return waypoint, nil
}

// applicableListeners returns the HBONE, HTTP, and HTTPS listeners of a waypoint Gateway.
func applicableListeners(gateway *gatewayv1.Gateway) []WaypointListener {
	var listeners []WaypointListener
	for _, listener := range gateway.Spec.Listeners {
		switch string(listener.Protocol) {
		case "HBONE", "HTTP", "HTTPS":
		default:
			continue
		}
		item := WaypointListener{
			Name:     string(listener.Name),
			Port:     int32(listener.Port),
			Protocol: string(listener.Protocol),
		}
		if listener.Hostname != nil {
			item.Hostname = string(*listener.Hostname)
		}
		listeners = append(listeners, item)
	}
	return listeners
}

func hasGatewayClass(snapshot source.Snapshot, name string) bool {
	for _, res := range snapshot.Resources {
		if res.Identity.Group == gatewayAPIGroup && res.Identity.Kind == "GatewayClass" && res.Identity.Name == name {
			return true
		}
	}
	return false
}
