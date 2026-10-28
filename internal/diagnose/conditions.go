package diagnose

import (
	"fmt"
	"strings"

	"github.com/ninoamine/istio-route/internal/model"
	"github.com/ninoamine/istio-route/internal/source"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func diagnoseConditions(analysis *model.Analysis, snapshot source.Snapshot, add addFindingFunc) {
	gateways := make(map[string]*gatewayv1.Gateway)
	routes := make(map[string]*gatewayv1.HTTPRoute)
	for _, resource := range snapshot.Resources {
		switch object := resource.Object.(type) {
		case *gatewayv1.Gateway:
			gateways[identityKey(resource.Identity)] = object
		case *gatewayv1.HTTPRoute:
			routes[identityKey(resource.Identity)] = object
		}
	}

	for i := range analysis.EntryPoints {
		entry := &analysis.EntryPoints[i]
		if entry.Origin != model.OriginGatewayAPI {
			continue
		}
		gateway := gateways[identityKey(entry.Resource)]
		if gateway == nil {
			continue
		}
		entry.Conditions = replaceConditionType(entry.Conditions, gatewayModelConditions(gateway.Status.Conditions, "Programmed"))
		if !hasCondition(entry.Conditions, "Programmed") {
			entry.Conditions = append(entry.Conditions, model.Condition{
				Type: "Programmed", Status: model.ConditionStatusUnavailable,
			})
		}
		for _, condition := range entry.Conditions {
			if condition.Type == "Programmed" {
				conditionFinding(add, "gateway-not-programmed", entry.Resource, condition, "Gateway is not programmed")
			}
		}
	}

	for routeIndex := range analysis.Routes {
		route := &analysis.Routes[routeIndex]
		if route.Origin != model.OriginGatewayAPI {
			continue
		}
		object := routes[identityKey(route.Resource)]
		if object == nil {
			continue
		}
		mapped, matchedParents := routeModelConditions(object)
		route.Conditions = replaceConditionType(route.Conditions, mapped)
		if !matchedParents {
			route.Conditions = appendUnavailableRouteConditions(route.Conditions)
		}
		for _, condition := range route.Conditions {
			switch condition.Type {
			case "Accepted":
				conditionFinding(add, "route-not-accepted", route.Resource, condition, "HTTPRoute was not accepted")
			case "ResolvedRefs":
				conditionFinding(add, "route-references-unresolved", route.Resource, condition, "HTTPRoute references could not be resolved")
			}
		}
	}
}

func gatewayModelConditions(conditions []metav1.Condition, conditionType string) []model.Condition {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return []model.Condition{modelCondition(condition)}
		}
	}
	return nil
}

func routeModelConditions(route *gatewayv1.HTTPRoute) ([]model.Condition, bool) {
	var conditions []model.Condition
	matchedParents := false
	for _, status := range route.Status.Parents {
		if !routeHasParentRef(route, status.ParentRef) {
			continue
		}
		matchedParents = true
		for _, condition := range status.Conditions {
			if condition.Type == string(gatewayv1.RouteConditionAccepted) ||
				condition.Type == string(gatewayv1.RouteConditionResolvedRefs) {
				conditions = append(conditions, modelCondition(condition))
			}
		}
	}
	if !matchedParents {
		return nil, false
	}
	for _, conditionType := range []string{
		string(gatewayv1.RouteConditionAccepted), string(gatewayv1.RouteConditionResolvedRefs),
	} {
		if !hasCondition(conditions, conditionType) {
			conditions = append(conditions, model.Condition{
				Type: conditionType, Status: model.ConditionStatusUnavailable,
			})
		}
	}
	return conditions, true
}

func modelCondition(condition metav1.Condition) model.Condition {
	result := model.Condition{
		Type: condition.Type, Status: conditionStatus(condition.Status),
		Reason: condition.Reason, Message: condition.Message,
		ObservedGeneration: int64Pointer(condition.ObservedGeneration),
	}
	if result.Message == "" && result.Status != model.ConditionStatusTrue {
		result.Message = conditionExplanation(result.Type, result.Reason)
	}
	return result
}

func conditionStatus(status metav1.ConditionStatus) model.ConditionStatus {
	switch status {
	case metav1.ConditionTrue:
		return model.ConditionStatusTrue
	case metav1.ConditionFalse:
		return model.ConditionStatusFalse
	case metav1.ConditionUnknown:
		return model.ConditionStatusUnknown
	default:
		return model.ConditionStatusUnavailable
	}
}

func conditionExplanation(conditionType, reason string) string {
	switch {
	case conditionType == "Accepted" && reason == "NotAllowedByListeners":
		return "listener does not allow this HTTPRoute"
	case conditionType == "Accepted" && reason == "NoMatchingParent":
		return "HTTPRoute has no matching parent Gateway or listener"
	case conditionType == "ResolvedRefs" && reason == "BackendNotFound":
		return "backend reference could not be resolved"
	case conditionType == "ResolvedRefs" && reason == "RefNotPermitted":
		return "cross-namespace reference is not authorized by a ReferenceGrant"
	case conditionType == "Programmed":
		return "Gateway is not programmed"
	case conditionType == "Accepted":
		return "HTTPRoute was not accepted"
	case conditionType == "ResolvedRefs":
		return "HTTPRoute references could not be resolved"
	default:
		if reason != "" {
			return fmt.Sprintf("%s condition is not true (%s)", conditionType, reason)
		}
		return fmt.Sprintf("%s condition is not true", conditionType)
	}
}

func conditionFinding(
	add addFindingFunc,
	code string,
	resource model.ResourceIdentity,
	condition model.Condition,
	prefix string,
) {
	if condition.Status != model.ConditionStatusFalse && condition.Status != model.ConditionStatusUnknown {
		return
	}
	explanation := strings.TrimSpace(condition.Message)
	if explanation == "" {
		explanation = conditionExplanation(condition.Type, condition.Reason)
	}
	message := prefix + ": " + explanation
	addWarning(add, code, message, resource)
}

func routeHasParentRef(route *gatewayv1.HTTPRoute, parent gatewayv1.ParentReference) bool {
	for _, ref := range route.Spec.ParentRefs {
		if string(ref.Name) != string(parent.Name) ||
			parentNamespace(ref, route.Namespace) != parentNamespace(parent, route.Namespace) ||
			parentGroup(ref) != parentGroup(parent) ||
			parentKind(ref) != parentKind(parent) ||
			parentSection(ref) != parentSection(parent) {
			continue
		}
		return true
	}
	return false
}

func parentNamespace(parent gatewayv1.ParentReference, routeNamespace string) string {
	if parent.Namespace == nil {
		return routeNamespace
	}
	return string(*parent.Namespace)
}

func parentGroup(parent gatewayv1.ParentReference) string {
	if parent.Group == nil {
		return gatewayGroup
	}
	return string(*parent.Group)
}

func parentKind(parent gatewayv1.ParentReference) string {
	if parent.Kind == nil {
		return "Gateway"
	}
	return string(*parent.Kind)
}

func parentSection(parent gatewayv1.ParentReference) string {
	if parent.SectionName == nil {
		return ""
	}
	return string(*parent.SectionName)
}

func hasCondition(conditions []model.Condition, conditionType string) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return true
		}
	}
	return false
}

func replaceConditionType(existing, replacement []model.Condition) []model.Condition {
	if len(replacement) == 0 {
		return existing
	}
	conditionTypes := make(map[string]struct{}, len(replacement))
	for _, condition := range replacement {
		conditionTypes[condition.Type] = struct{}{}
	}
	result := make([]model.Condition, 0, len(existing)+len(replacement))
	for _, condition := range existing {
		if _, replaced := conditionTypes[condition.Type]; !replaced {
			result = append(result, condition)
		}
	}
	return append(result, replacement...)
}

func appendUnavailableRouteConditions(conditions []model.Condition) []model.Condition {
	for _, conditionType := range []string{"Accepted", "ResolvedRefs"} {
		if !hasCondition(conditions, conditionType) {
			conditions = append(conditions, model.Condition{
				Type: conditionType, Status: model.ConditionStatusUnavailable,
			})
		}
	}
	return conditions
}

func int64Pointer(value int64) *int64 {
	return &value
}
