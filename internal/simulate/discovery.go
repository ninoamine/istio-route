package simulate

import (
	"fmt"
	"slices"
	"strings"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/ninoamine/istio-route/internal/source"
)

// Selection is the attachment evidence of the HTTPRoute chosen for simulation.
type Selection = RouteAttachments

// EligibleRoutes lists the routes accepted or unconfirmed on at least one applicable listener,
// sorted by namespace and name. Routes rejected everywhere or without applicable listeners are omitted.
func EligibleRoutes(attachments []RouteAttachments) []RouteListing {
	listings := []RouteListing{}
	for _, attachment := range attachments {
		if !isEligible(attachment) {
			continue
		}
		listings = append(listings, RouteListing{
			Route:     attachment.Route,
			Hostnames: slices.Clone(attachment.Hostnames),
			Listeners: slices.Clone(attachment.Listeners),
		})
	}
	slices.SortFunc(listings, func(a, b RouteListing) int {
		return compareRouteRefs(a.Route, b.Route)
	})
	return listings
}

// SelectRoute returns the attachment evidence for the HTTPRoute named by ref. It fails with
// ErrRouteNotFound when the route is absent from the snapshot, and with ErrRouteIneligible when
// the route is not attached to the waypoint or is rejected everywhere.
func SelectRoute(snapshot source.Snapshot, attachments []RouteAttachments, ref RouteRef) (Selection, error) {
	if !routeExists(snapshot, ref) {
		return Selection{}, fmt.Errorf("%s/%s: %w", ref.Namespace, ref.Name, ErrRouteNotFound)
	}
	for _, attachment := range attachments {
		if attachment.Route != ref {
			continue
		}
		if !isEligible(attachment) {
			return Selection{}, fmt.Errorf("%s/%s: %w: %s", ref.Namespace, ref.Name, ErrRouteIneligible, ineligibleReason(attachment))
		}
		return attachment, nil
	}
	return Selection{}, fmt.Errorf("%s/%s: %w: no direct parentRef to this waypoint", ref.Namespace, ref.Name, ErrRouteIneligible)
}

// isEligible reports whether the route is accepted or unconfirmed on at least one applicable listener.
func isEligible(attachment RouteAttachments) bool {
	for _, listener := range attachment.Listeners {
		if listener.State == AttachmentAccepted || listener.State == AttachmentUnconfirmed {
			return true
		}
	}
	return false
}

func ineligibleReason(attachment RouteAttachments) string {
	if len(attachment.Listeners) == 0 {
		return attachment.Reason
	}
	reasons := make([]string, 0, len(attachment.Listeners))
	for _, listener := range attachment.Listeners {
		reasons = append(reasons, fmt.Sprintf("%s: %s", listener.Listener, listener.Reason))
	}
	return "rejected on every applicable listener (" + strings.Join(reasons, "; ") + ")"
}

func routeExists(snapshot source.Snapshot, ref RouteRef) bool {
	for _, res := range snapshot.Resources {
		if _, ok := res.Object.(*gatewayv1.HTTPRoute); ok && routeRef(res) == ref {
			return true
		}
	}
	return false
}

func compareRouteRefs(a, b RouteRef) int {
	if c := strings.Compare(a.Namespace, b.Namespace); c != 0 {
		return c
	}
	return strings.Compare(a.Name, b.Name)
}
