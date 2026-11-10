package simulate

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/ninoamine/istio-route/internal/audit"
	"github.com/ninoamine/istio-route/internal/model"
	"github.com/ninoamine/istio-route/internal/resolve"
	"github.com/ninoamine/istio-route/internal/source"
)

const (
	acceptedConditionType = "Accepted"
	serviceKind           = "Service"
	reasonNoListeners     = "no applicable listeners"
	reasonNoStatus        = "no Accepted status reported for this listener"
	reasonStaleStatus     = "Accepted status is missing or does not match the route generation"
	reasonUnscopedStatus  = "Accepted status is not scoped to this listener"
	reasonHBONEAllowed    = "HBONE listener has no allowedRoutes"
)

// RouteAttachments is the attachment evidence of one HTTPRoute with a direct parentRef to a waypoint.
type RouteAttachments struct {
	Route     RouteRef
	Hostnames []string
	Listeners []ListenerAttachment
	// Reason explains why the route has no applicable listeners.
	Reason string
}

// ServiceParentedExclusion is an HTTPRoute parentRef to a Service, which never attaches to a waypoint.
type ServiceParentedExclusion struct {
	Route   RouteRef
	Service string
}

// EvaluateAttachments returns the HTTPRoutes with a direct parentRef to the waypoint and the
// evidence state of each route on every applicable waypoint listener.
func EvaluateAttachments(snapshot source.Snapshot, waypoint Waypoint) []RouteAttachments {
	gateway := waypointGateway(snapshot, waypoint)
	parent := model.ResourceIdentity{Group: gatewayAPIGroup, Kind: gatewayKind, Namespace: waypoint.Namespace, Name: waypoint.Name}
	controller := ""
	if gateway != nil {
		controller = gatewayClassController(snapshot, string(gateway.Spec.GatewayClassName))
	}

	var out []RouteAttachments
	for _, res := range snapshot.Resources {
		route, ok := res.Object.(*gatewayv1.HTTPRoute)
		if !ok {
			continue
		}
		refs := directParentRefs(route, res.Identity.Namespace, parent)
		if len(refs) == 0 {
			continue
		}
		attachment := RouteAttachments{Route: routeRef(res), Hostnames: routeHostnames(route)}
		if gateway != nil {
			attachment.Listeners = routeListeners(snapshot, gateway, route, res.Identity.Namespace, refs, parent, controller)
		}
		if len(attachment.Listeners) == 0 {
			attachment.Reason = reasonNoListeners
		}
		out = append(out, attachment)
	}
	return out
}

// ServiceParentedExclusions lists HTTPRoute parentRefs that target a Service rather than a Gateway.
func ServiceParentedExclusions(snapshot source.Snapshot) []ServiceParentedExclusion {
	var out []ServiceParentedExclusion
	for _, res := range snapshot.Resources {
		route, ok := res.Object.(*gatewayv1.HTTPRoute)
		if !ok {
			continue
		}
		for _, ref := range route.Spec.ParentRefs {
			if !isServiceParentRef(ref) {
				continue
			}
			out = append(out, ServiceParentedExclusion{Route: routeRef(res), Service: string(ref.Name)})
		}
	}
	return out
}

func waypointGateway(snapshot source.Snapshot, waypoint Waypoint) *gatewayv1.Gateway {
	selected, err := audit.SelectGateway(snapshot, waypoint.Name, waypoint.Namespace)
	if err != nil || selected == nil {
		return nil
	}
	gateway, _ := selected.Object.(*gatewayv1.Gateway)
	return gateway
}

// gatewayClassController returns the controllerName of the named GatewayClass, or "" when absent.
func gatewayClassController(snapshot source.Snapshot, name string) string {
	for _, res := range snapshot.Resources {
		class, ok := res.Object.(*gatewayv1.GatewayClass)
		if ok && res.Identity.Group == gatewayAPIGroup && class.Name == name {
			return string(class.Spec.ControllerName)
		}
	}
	return ""
}

func directParentRefs(route *gatewayv1.HTTPRoute, routeNamespace string, parent model.ResourceIdentity) []gatewayv1.ParentReference {
	var refs []gatewayv1.ParentReference
	for _, ref := range route.Spec.ParentRefs {
		if audit.ParentTargets(ref, routeNamespace, parent) {
			refs = append(refs, ref)
		}
	}
	return refs
}

// routeListeners evaluates each applicable waypoint listener that a direct parentRef selects.
func routeListeners(snapshot source.Snapshot, gateway *gatewayv1.Gateway, route *gatewayv1.HTTPRoute,
	routeNamespace string, refs []gatewayv1.ParentReference, parent model.ResourceIdentity, controller string,
) []ListenerAttachment {
	var out []ListenerAttachment
	for _, listener := range gateway.Spec.Listeners {
		if !isApplicableProtocol(string(listener.Protocol)) {
			continue
		}
		ref, ok := targetingRef(refs, listener)
		if !ok || !hostnameApplies(route, listener) {
			continue
		}
		applicable, unconfirmed := routeAllowed(snapshot, gateway, listener, routeNamespace)
		if !applicable {
			continue
		}
		state, reason := acceptanceState(route, listener, routeNamespace, parent, controller)
		if unconfirmed != "" && state == AttachmentAccepted {
			state, reason = AttachmentUnconfirmed, unconfirmed
		}
		attachment := ListenerAttachment{
			Route:    RouteRef{Namespace: routeNamespace, Name: route.Name},
			Listener: string(listener.Name),
			State:    state,
			Reason:   reason,
		}
		if ref.SectionName != nil {
			attachment.SectionName = string(*ref.SectionName)
		}
		if ref.Port != nil {
			port := *ref.Port
			attachment.Port = &port
		}
		out = append(out, attachment)
	}
	return out
}

func targetingRef(refs []gatewayv1.ParentReference, listener gatewayv1.Listener) (gatewayv1.ParentReference, bool) {
	for _, ref := range refs {
		if ref.SectionName != nil && string(*ref.SectionName) != string(listener.Name) {
			continue
		}
		if ref.Port != nil && *ref.Port != listener.Port {
			continue
		}
		return ref, true
	}
	return gatewayv1.ParentReference{}, false
}

func isApplicableProtocol(protocol string) bool {
	switch protocol {
	case "HBONE", "HTTP", "HTTPS":
		return true
	default:
		return false
	}
}

// hostnameApplies reports whether the route's hostnames intersect the listener hostname.
// An empty set on either side matches everything.
func hostnameApplies(route *gatewayv1.HTTPRoute, listener gatewayv1.Listener) bool {
	if listener.Hostname == nil || len(route.Spec.Hostnames) == 0 {
		return true
	}
	for _, hostname := range route.Spec.Hostnames {
		if resolve.HostnamesIntersect(string(hostname), string(*listener.Hostname)) {
			return true
		}
	}
	return false
}

// routeAllowed applies the listener's allowedRoutes policy. It returns applicable=false when the
// policy rejects the route, and a non-empty unconfirmed reason when applicability cannot be established.
func routeAllowed(snapshot source.Snapshot, gateway *gatewayv1.Gateway, listener gatewayv1.Listener, routeNamespace string) (bool, string) {
	allowed := listener.AllowedRoutes
	if allowed == nil {
		if string(listener.Protocol) == "HBONE" {
			return true, reasonHBONEAllowed
		}
		allowed = &gatewayv1.AllowedRoutes{}
	}
	if len(allowed.Kinds) > 0 && !allowsHTTPRoute(allowed.Kinds) {
		return false, ""
	}

	from := gatewayv1.NamespacesFromSame
	if allowed.Namespaces != nil && allowed.Namespaces.From != nil {
		from = *allowed.Namespaces.From
	}
	switch from {
	case gatewayv1.NamespacesFromAll:
		return true, ""
	case gatewayv1.NamespacesFromSelector:
		return selectorAllows(snapshot, allowed.Namespaces, routeNamespace)
	default:
		return routeNamespace == gateway.Namespace, ""
	}
}

func allowsHTTPRoute(kinds []gatewayv1.RouteGroupKind) bool {
	for _, kind := range kinds {
		group := gatewayAPIGroup
		if kind.Group != nil {
			group = string(*kind.Group)
		}
		if group == gatewayAPIGroup && string(kind.Kind) == httpRouteKind {
			return true
		}
	}
	return false
}

// selectorAllows matches the route namespace labels against the listener's namespace selector.
// A selector whose namespace object is missing from the snapshot is unconfirmed.
func selectorAllows(snapshot source.Snapshot, namespaces *gatewayv1.RouteNamespaces, routeNamespace string) (bool, string) {
	if namespaces == nil || namespaces.Selector == nil {
		return true, "namespace selector is missing"
	}
	nsLabels, found := namespaceLabels(snapshot, routeNamespace)
	if !found {
		return true, fmt.Sprintf("namespace %q not found in snapshot for selector", routeNamespace)
	}
	selector, err := metav1.LabelSelectorAsSelector(namespaces.Selector)
	if err != nil {
		return true, fmt.Sprintf("invalid namespace selector: %v", err)
	}
	return selector.Matches(labels.Set(nsLabels)), ""
}

func namespaceLabels(snapshot source.Snapshot, name string) (map[string]string, bool) {
	for _, res := range snapshot.Resources {
		if res.Identity.Kind != "Namespace" || res.Identity.Name != name {
			continue
		}
		obj, ok := res.Object.(metav1.Object)
		if !ok {
			return nil, false
		}
		return obj.GetLabels(), true
	}
	return nil, false
}

// acceptanceState derives the Accepted state reported for one listener of the route.
func acceptanceState(route *gatewayv1.HTTPRoute, listener gatewayv1.Listener, routeNamespace string,
	parent model.ResourceIdentity, controller string,
) (AttachmentState, string) {
	var scoped *gatewayv1.RouteParentStatus
	unscoped := false
	for i := range route.Status.Parents {
		status := &route.Status.Parents[i]
		if !audit.ParentTargets(status.ParentRef, routeNamespace, parent) {
			continue
		}
		if status.ControllerName != "" && controller != "" && string(status.ControllerName) != controller {
			continue
		}
		if status.ParentRef.SectionName == nil {
			unscoped = true
			continue
		}
		if string(*status.ParentRef.SectionName) == string(listener.Name) {
			scoped = status
			break
		}
	}
	switch {
	case scoped == nil && unscoped:
		return AttachmentUnconfirmed, reasonUnscopedStatus
	case scoped == nil:
		return AttachmentUnconfirmed, reasonNoStatus
	}

	condition := findCondition(scoped.Conditions, acceptedConditionType)
	if condition == nil || condition.ObservedGeneration == 0 || route.Generation == 0 ||
		condition.ObservedGeneration != route.Generation {
		return AttachmentUnconfirmed, reasonStaleStatus
	}
	switch condition.Status {
	case metav1.ConditionTrue:
		return AttachmentAccepted, ""
	case metav1.ConditionFalse:
		return AttachmentRejected, condition.Message
	default:
		return AttachmentUnconfirmed, reasonStaleStatus
	}
}

func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

func isServiceParentRef(ref gatewayv1.ParentReference) bool {
	if ref.Kind == nil || string(*ref.Kind) != serviceKind {
		return false
	}
	return ref.Group == nil || string(*ref.Group) == ""
}

func routeRef(res source.Resource) RouteRef {
	return RouteRef{Namespace: res.Identity.Namespace, Name: res.Identity.Name}
}

func routeHostnames(route *gatewayv1.HTTPRoute) []string {
	hostnames := make([]string, len(route.Spec.Hostnames))
	for i, hostname := range route.Spec.Hostnames {
		hostnames[i] = string(hostname)
	}
	return hostnames
}
