package audit

import (
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/ninoamine/istio-route/internal/model"
	"github.com/ninoamine/istio-route/internal/source"
)

// IsWaypointGateway reports whether gateway uses the istio-waypoint GatewayClass.
func IsWaypointGateway(gateway *gatewayv1.Gateway) bool {
	return isWaypoint(gateway)
}

// ParentTargets reports whether ref points at the gateway from a route in routeNamespace.
func ParentTargets(ref gatewayv1.ParentReference, routeNamespace string, gateway model.ResourceIdentity) bool {
	return parentTargets(ref, routeNamespace, gateway)
}

// HTTPMatchFrom converts a Gateway API HTTPRouteMatch into the audit Match model.
func HTTPMatchFrom(match gatewayv1.HTTPRouteMatch) Match {
	return httpMatch(match)
}

// SelectGateway returns the Gateway with the given name, or nil when none matches.
// Duplicates across namespaces return an error wrapping ErrAmbiguousGateway.
func SelectGateway(snapshot source.Snapshot, name, namespace string) (*source.Resource, error) {
	return selectGateway(snapshot, name, namespace)
}

// CompareRules applies the 004 precedence comparator to two rules.
// It returns 1 when the left rule wins, -1 when the right rule wins, and 0 when undecided,
// along with the criterion that decided the comparison.
func CompareRules(leftRoute Route, leftRule Rule, rightRoute Route, rightRule Rule) (int, string) {
	outcome, criterion := compareRulePrecedence(
		ruleRef{route: leftRoute, rule: &leftRule, ruleIndex: leftRule.Index},
		ruleRef{route: rightRoute, rule: &rightRule, ruleIndex: rightRule.Index},
	)
	switch outcome {
	case outcomeLeftWins:
		return 1, criterion
	case outcomeRightWins:
		return -1, criterion
	default:
		return 0, criterion
	}
}
