package resolve

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/ninoamine/istio-route/internal/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

const gatewayGroup = "gateway.networking.k8s.io"

func resolveGatewayAPI(
	ctx context.Context,
	hostname string,
	resources catalog,
	endpoints *endpointResolver,
	analysis *model.Analysis,
) error {
	classes := make(map[string]*gatewayv1.GatewayClass, len(resources.gatewayClasses))
	for _, class := range resources.gatewayClasses {
		classes[class.Name] = class
	}

	type matchingListener struct {
		gateway  *gatewayv1.Gateway
		listener gatewayv1.Listener
		index    int
	}
	listeners := make([]matchingListener, 0)
	for _, gateway := range resources.gateways {
		for index, listener := range gateway.Spec.Listeners {
			listenerHostname := ""
			if listener.Hostname != nil {
				listenerHostname = string(*listener.Hostname)
				if !MatchesHostname(hostname, listenerHostname) {
					continue
				}
			}
			class := classes[string(gateway.Spec.GatewayClassName)]
			entry := model.EntryPoint{
				Origin:     model.OriginGatewayAPI,
				Resource:   identity(gatewayGroup, "v1", "Gateway", gateway.Namespace, gateway.Name),
				Name:       string(listener.Name),
				Hostname:   listenerHostname,
				Port:       int32(listener.Port),
				Protocol:   string(listener.Protocol),
				Conditions: gatewayConditions(gateway.Status.Conditions, "Programmed"),
			}
			if class != nil {
				entry.GatewayClass = &model.GatewayClassRef{
					Identity:   identity(gatewayGroup, "v1", "GatewayClass", "", class.Name),
					Controller: string(class.Spec.ControllerName),
				}
			}
			if listener.TLS != nil {
				entry.TLS = &model.TLSConfig{}
				if listener.TLS.Mode != nil {
					entry.TLS.Mode = string(*listener.TLS.Mode)
				}
				for _, certificate := range listener.TLS.CertificateRefs {
					namespace := gateway.Namespace
					if certificate.Namespace != nil {
						namespace = string(*certificate.Namespace)
					}
					group := ""
					if certificate.Group != nil {
						group = string(*certificate.Group)
					}
					kind := "Secret"
					if certificate.Kind != nil {
						kind = string(*certificate.Kind)
					}
					version := "v1"
					if group == gatewayGroup {
						version = "v1"
					}
					entry.TLS.CertificateRefs = append(entry.TLS.CertificateRefs, identity(
						group, version, kind, namespace, string(certificate.Name),
					))
				}
			}
			entry.RouteConstraints = allowedRouteConstraints(listener.AllowedRoutes)
			analysis.EntryPoints = append(analysis.EntryPoints, entry)
			listeners = append(listeners, matchingListener{gateway: gateway, listener: listener, index: index})
		}
	}

	for _, route := range resources.httpRoutes {
		routeHosts := make([]string, len(route.Spec.Hostnames))
		for i, routeHostname := range route.Spec.Hostnames {
			routeHosts[i] = string(routeHostname)
		}
		if len(routeHosts) > 0 && !anyHostnameMatches(hostname, routeHosts) {
			continue
		}
		parents := make([]model.ResourceIdentity, 0)
		for _, listener := range listeners {
			if !routeAttaches(route, listener.gateway, listener.listener) {
				continue
			}
			if !allowedRouteNamespace(listener.listener.AllowedRoutes, route.Namespace, listener.gateway.Namespace) {
				continue
			}
			if !allowedRouteKind(listener.listener.AllowedRoutes) {
				continue
			}
			if !routeHostnameIntersects(routeHosts, listener.listener.Hostname) {
				continue
			}
			parent := identity(gatewayGroup, "v1", "Gateway", listener.gateway.Namespace, listener.gateway.Name)
			parents = appendIdentityOnce(parents, parent)
		}
		if len(parents) == 0 {
			continue
		}
		routeModel := model.Route{
			Origin:     model.OriginGatewayAPI,
			Resource:   identity(gatewayGroup, "v1", "HTTPRoute", route.Namespace, route.Name),
			Hostnames:  routeHosts,
			ParentRefs: parents,
			Conditions: routeConditions(route.Status.Parents, route.Spec.ParentRefs, route.Namespace),
		}
		for ruleIndex, ruleConfig := range route.Spec.Rules {
			rule := model.Rule{
				Index:   ruleIndex + 1,
				Filters: gatewayFilters(ruleConfig.Filters),
			}
			if ruleConfig.Timeouts != nil {
				rule.Filters = append(rule.Filters, model.Filter{
					Type: "Timeout",
					Details: map[string]string{
						"request":        durationValue(ruleConfig.Timeouts.Request),
						"backendRequest": durationValue(ruleConfig.Timeouts.BackendRequest),
					},
				})
			}
			for _, match := range ruleConfig.Matches {
				rule.Matches = append(rule.Matches, gatewayMatch(match))
			}
			if len(rule.Matches) == 0 {
				rule.Matches = []model.Match{{PathType: "PathPrefix", Path: "/"}}
			}
			for _, backend := range ruleConfig.BackendRefs {
				destination, err := gatewayDestination(ctx, route, backend, resources.referenceGrants, endpoints)
				if err != nil {
					return err
				}
				rule.Destinations = append(rule.Destinations, destination)
			}
			routeModel.Rules = append(routeModel.Rules, rule)
		}
		analysis.Routes = append(analysis.Routes, routeModel)
	}
	return nil
}

func gatewayDestination(
	ctx context.Context,
	route *gatewayv1.HTTPRoute,
	backend gatewayv1.HTTPBackendRef,
	grants []*gatewayv1beta1.ReferenceGrant,
	endpoints *endpointResolver,
) (model.Destination, error) {
	ref := backend.BackendObjectReference
	namespace := route.Namespace
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}
	group := ""
	if ref.Group != nil {
		group = string(*ref.Group)
	}
	kind := "Service"
	if ref.Kind != nil {
		kind = string(*ref.Kind)
	}
	name := string(ref.Name)
	result := model.Destination{
		Kind:         model.DestinationKindService,
		Namespace:    namespace,
		Name:         name,
		BackendGroup: group,
		BackendKind:  kind,
	}
	if ref.Port != nil {
		port := int32(*ref.Port)
		result.Port = &port
	}
	weight := int32(1)
	if backend.Weight != nil {
		weight = *backend.Weight
	}
	result.Weight = &weight
	if namespace != route.Namespace {
		grant, allowed := findReferenceGrant(grants, route.Namespace, route.Name, group, kind, namespace, name)
		result.ReferenceGrant = &model.GrantResult{Allowed: allowed}
		if grant != nil {
			grantIdentity := identity(gatewayGroup, "v1beta1", "ReferenceGrant", grant.Namespace, grant.Name)
			result.ReferenceGrant.GrantRef = &grantIdentity
		} else {
			result.ReferenceGrant.Reason = fmt.Sprintf("no ReferenceGrant in namespace %s allows this backend", namespace)
		}
	}
	if group != "" || kind != "Service" {
		return result, nil
	}
	service, err := endpoints.resolve(ctx, namespace, name)
	if err != nil {
		return model.Destination{}, err
	}
	result.Service = service
	return result, nil
}

func findReferenceGrant(
	grants []*gatewayv1beta1.ReferenceGrant,
	fromNamespace, routeName, backendGroup, backendKind, toNamespace, backendName string,
) (*gatewayv1beta1.ReferenceGrant, bool) {
	for _, grant := range grants {
		if grant.Namespace != toNamespace {
			continue
		}
		fromAllowed := false
		for _, from := range grant.Spec.From {
			group := string(from.Group)
			kind := string(from.Kind)
			if from.Namespace == gatewayv1.Namespace(fromNamespace) &&
				group == gatewayGroup && kind == "HTTPRoute" {
				fromAllowed = true
				break
			}
		}
		if !fromAllowed {
			continue
		}
		for _, to := range grant.Spec.To {
			if string(to.Group) != backendGroup || string(to.Kind) != backendKind {
				continue
			}
			if to.Name == nil || string(*to.Name) == backendName {
				return grant, true
			}
		}
	}
	return nil, false
}

func routeAttaches(route *gatewayv1.HTTPRoute, gateway *gatewayv1.Gateway, listener gatewayv1.Listener) bool {
	for _, parent := range route.Spec.ParentRefs {
		if string(parent.Name) != gateway.Name {
			continue
		}
		parentNamespace := route.Namespace
		if parent.Namespace != nil {
			parentNamespace = string(*parent.Namespace)
		}
		if parentNamespace != gateway.Namespace {
			continue
		}
		if parent.Group != nil && string(*parent.Group) != gatewayGroup {
			continue
		}
		if parent.Kind != nil && string(*parent.Kind) != "Gateway" {
			continue
		}
		if parent.SectionName != nil && *parent.SectionName != listener.Name {
			continue
		}
		return true
	}
	return false
}

func routeHostnameIntersects(routeHosts []string, listenerHostname *gatewayv1.Hostname) bool {
	if listenerHostname == nil || len(routeHosts) == 0 {
		return true
	}
	for _, routeHostname := range routeHosts {
		if HostnamesIntersect(routeHostname, string(*listenerHostname)) {
			return true
		}
	}
	return false
}

func allowedRouteNamespace(routes *gatewayv1.AllowedRoutes, routeNamespace, gatewayNamespace string) bool {
	if routes == nil || routes.Namespaces == nil || routes.Namespaces.From == nil {
		return routeNamespace == gatewayNamespace
	}
	switch *routes.Namespaces.From {
	case gatewayv1.NamespacesFromAll:
		return true
	case gatewayv1.NamespacesFromSame:
		return routeNamespace == gatewayNamespace
	default:
		return false
	}
}

func allowedRouteKind(routes *gatewayv1.AllowedRoutes) bool {
	if routes == nil || len(routes.Kinds) == 0 {
		return true
	}
	for _, kind := range routes.Kinds {
		if string(kind.Kind) == "HTTPRoute" && (kind.Group == nil || string(*kind.Group) == gatewayGroup) {
			return true
		}
	}
	return false
}

func allowedRouteConstraints(routes *gatewayv1.AllowedRoutes) []string {
	if routes == nil {
		return []string{"namespaces:Same", "kinds:protocol-default"}
	}
	constraints := make([]string, 0, 2)
	if routes.Namespaces == nil || routes.Namespaces.From == nil {
		constraints = append(constraints, "namespaces:Same")
	} else {
		constraints = append(constraints, "namespaces:"+string(*routes.Namespaces.From))
	}
	for _, kind := range routes.Kinds {
		constraints = append(constraints, "kind:"+string(kind.Kind))
	}
	return constraints
}

func gatewayMatch(match gatewayv1.HTTPRouteMatch) model.Match {
	result := model.Match{
		Headers:     make(map[string]string),
		QueryParams: make(map[string]string),
	}
	if match.Path != nil {
		if match.Path.Type != nil {
			result.PathType = string(*match.Path.Type)
		}
		if match.Path.Value != nil {
			result.Path = *match.Path.Value
		}
	}
	if match.Method != nil {
		result.Method = string(*match.Method)
	}
	for _, header := range match.Headers {
		result.Headers[string(header.Name)] = string(header.Value)
	}
	for _, query := range match.QueryParams {
		result.QueryParams[string(query.Name)] = string(query.Value)
	}
	if len(result.Headers) == 0 {
		result.Headers = nil
	}
	if len(result.QueryParams) == 0 {
		result.QueryParams = nil
	}
	return result
}

func gatewayFilters(filters []gatewayv1.HTTPRouteFilter) []model.Filter {
	result := make([]model.Filter, 0, len(filters))
	for _, filter := range filters {
		details := make(map[string]string)
		switch filter.Type {
		case gatewayv1.HTTPRouteFilterRequestHeaderModifier:
			if filter.RequestHeaderModifier != nil {
				for _, header := range filter.RequestHeaderModifier.Set {
					details["set:"+string(header.Name)] = header.Value
				}
				for _, header := range filter.RequestHeaderModifier.Add {
					details["add:"+string(header.Name)] = header.Value
				}
				if len(filter.RequestHeaderModifier.Remove) > 0 {
					details["remove"] = strings.Join(filter.RequestHeaderModifier.Remove, ",")
				}
			}
		case gatewayv1.HTTPRouteFilterRequestRedirect:
			if filter.RequestRedirect != nil {
				redirect := filter.RequestRedirect
				if redirect.Scheme != nil {
					details["scheme"] = *redirect.Scheme
				}
				if redirect.Hostname != nil {
					details["hostname"] = string(*redirect.Hostname)
				}
				if redirect.Port != nil {
					details["port"] = strconv.FormatInt(int64(*redirect.Port), 10)
				}
				if redirect.StatusCode != nil {
					details["statusCode"] = strconv.FormatInt(int64(*redirect.StatusCode), 10)
				}
				if redirect.Path != nil {
					details["path"] = fmt.Sprint(redirect.Path)
				}
			}
		case gatewayv1.HTTPRouteFilterURLRewrite:
			if filter.URLRewrite != nil {
				if filter.URLRewrite.Hostname != nil {
					details["hostname"] = string(*filter.URLRewrite.Hostname)
				}
				if filter.URLRewrite.Path != nil {
					details["path"] = fmt.Sprint(filter.URLRewrite.Path)
				}
			}
		case gatewayv1.HTTPRouteFilterRequestMirror:
			if filter.RequestMirror != nil {
				details["backend"] = string(filter.RequestMirror.BackendRef.Name)
				if filter.RequestMirror.BackendRef.Namespace != nil {
					details["namespace"] = string(*filter.RequestMirror.BackendRef.Namespace)
				}
			}
		}
		result = append(result, model.Filter{Type: string(filter.Type), Details: details})
	}
	return result
}

func gatewayConditions(conditions []metav1.Condition, conditionType string) []model.Condition {
	result := make([]model.Condition, 0, 1)
	for _, condition := range conditions {
		if condition.Type == conditionType {
			result = append(result, model.Condition{
				Type: condition.Type, Status: conditionStatus(condition.Status),
				Reason: condition.Reason, Message: condition.Message,
				ObservedGeneration: int64Pointer(condition.ObservedGeneration),
			})
		}
	}
	return result
}

func routeConditions(statuses []gatewayv1.RouteParentStatus, parents []gatewayv1.ParentReference, routeNamespace string) []model.Condition {
	result := make([]model.Condition, 0, len(parents)*2)
	for _, parent := range parents {
		for _, status := range statuses {
			if !parentReferenceEqual(parent, status.ParentRef, routeNamespace) {
				continue
			}
			result = append(result, gatewayConditions(status.Conditions, "Accepted")...)
			result = append(result, gatewayConditions(status.Conditions, "ResolvedRefs")...)
		}
	}
	return result
}

func parentReferenceEqual(first, second gatewayv1.ParentReference, routeNamespace string) bool {
	return first.Name == second.Name &&
		parentNamespace(first, routeNamespace) == parentNamespace(second, routeNamespace) &&
		parentGroup(first) == parentGroup(second) &&
		parentKind(first) == parentKind(second) &&
		parentSection(first) == parentSection(second)
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

func conditionStatus(status metav1.ConditionStatus) model.ConditionStatus {
	switch status {
	case metav1.ConditionTrue:
		return model.ConditionStatusTrue
	case metav1.ConditionFalse:
		return model.ConditionStatusFalse
	default:
		return model.ConditionStatusUnknown
	}
}

func durationValue(value *gatewayv1.Duration) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func int64Pointer(value int64) *int64 {
	return &value
}
