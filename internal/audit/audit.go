package audit

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ninoamine/istio-route/internal/model"
	"github.com/ninoamine/istio-route/internal/resolve"
	"github.com/ninoamine/istio-route/internal/source"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

const gatewayAPIGroup = "gateway.networking.k8s.io"

func Analyze(ctx context.Context, request Request, snapshot source.Snapshot) (Result, Outcome, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, "", fmt.Errorf("analyze Gateway: %w", err)
	}
	request.Source = snapshot.Metadata
	if err := request.Validate(); err != nil {
		return Result{}, "", fmt.Errorf("validate Gateway audit request: %w", err)
	}
	if err := snapshot.Validate(); err != nil {
		return Result{}, "", fmt.Errorf("validate Gateway source snapshot: %w", err)
	}
	gatewayResource, err := selectGateway(snapshot, request.GatewayName, request.Namespace)
	if err != nil {
		return Result{}, "", err
	}
	if gatewayResource == nil {
		result := Result{
			Source: snapshot.Metadata,
			WaypointSelections: missingWaypointSelections(
				snapshot, request.Namespace, request.GatewayName,
			),
			EndpointCheck: EndpointCheck{State: EndpointCheckChecked},
		}
		if snapshot.Metadata.Mode == model.SourceModeFiles && !hasEndpointSlices(snapshot) {
			result.EndpointCheck = EndpointCheck{
				State:  EndpointCheckSkipped,
				Reason: "no EndpointSlices were supplied in the offline input",
			}
		} else if !hasAvailableFamily(snapshot.Metadata, "endpointslices") {
			result.EndpointCheck = EndpointCheck{
				State:  EndpointCheckUnavailable,
				Reason: "EndpointSlice API is unavailable",
			}
		}
		result.Findings = orphanedFindings(result)
		return result, OutcomeTargetNotFound, nil
	}
	gateway, ok := gatewayResource.Object.(*gatewayv1.Gateway)
	if !ok {
		return Result{}, "", fmt.Errorf("decode selected Gateway %s/%s: unexpected object type %T", gatewayResource.Identity.Namespace, gatewayResource.Identity.Name, gatewayResource.Object)
	}
	classResource := findResource(snapshot, gatewayAPIGroup, "GatewayClass", "", string(gateway.Spec.GatewayClassName))
	if classResource == nil {
		return Result{}, "", fmt.Errorf("GatewayClass %q referenced by %s/%s is missing", gateway.Spec.GatewayClassName, gateway.Namespace, gateway.Name)
	}
	class, ok := classResource.Object.(*gatewayv1.GatewayClass)
	if !ok {
		return Result{}, "", fmt.Errorf("decode GatewayClass %q: unexpected object type %T", classResource.Identity.Name, classResource.Object)
	}
	result := Result{
		Source: snapshot.Metadata,
		Gateway: Gateway{
			Identity: gatewayResource.Identity,
			Class: model.GatewayClassRef{
				Identity:   classResource.Identity,
				Controller: string(class.Spec.ControllerName),
			},
			Programmed: conditionByType(gateway.Status.Conditions, "Programmed"),
		},
		EndpointCheck: EndpointCheck{State: EndpointCheckChecked},
	}
	result.Listeners = auditListeners(gateway)
	routeResources := linkedRoutes(snapshot, gatewayResource.Identity)
	for _, resource := range routeResources {
		route, err := auditRoute(resource, gatewayResource.Identity, result.Listeners, snapshot)
		if err != nil {
			return Result{}, "", err
		}
		result.Routes = append(result.Routes, route)
	}
	if isWaypoint(gateway) {
		result.WaypointSelections = waypointSelections(snapshot, gatewayResource.Identity, result.Gateway.Programmed)
	}
	if snapshot.Metadata.Mode == model.SourceModeFiles && !hasEndpointSlices(snapshot) {
		result.EndpointCheck = EndpointCheck{
			State:  EndpointCheckSkipped,
			Reason: "no EndpointSlices were supplied in the offline input",
		}
	} else if !hasAvailableFamily(snapshot.Metadata, "endpointslices") {
		result.EndpointCheck = EndpointCheck{
			State:  EndpointCheckUnavailable,
			Reason: "EndpointSlice API is unavailable",
		}
	}
	applyPrecedence(&result)
	result.Findings = append(result.Findings, orphanedFindings(result)...)
	result.Findings = append(result.Findings, duplicateFindings(result)...)
	result.Findings = append(result.Findings, precedenceFindings(result)...)
	result.Findings = append(result.Findings, deficientFindings(result)...)
	sortFindings(result.Findings)
	if err := result.Validate(); err != nil {
		return Result{}, "", fmt.Errorf("validate Gateway audit result: %w", err)
	}
	return result, OutcomeTargetFound, nil
}

func auditListeners(gateway *gatewayv1.Gateway) []Listener {
	listeners := make([]Listener, 0, len(gateway.Spec.Listeners))
	statusByName := make(map[string][]metav1.Condition, len(gateway.Status.Listeners))
	for _, status := range gateway.Status.Listeners {
		statusByName[string(status.Name)] = status.Conditions
	}
	for _, item := range gateway.Spec.Listeners {
		listener := Listener{
			Name: string(item.Name), Port: int32(item.Port), Protocol: string(item.Protocol),
		}
		if item.Hostname != nil {
			listener.Hostnames = []string{string(*item.Hostname)}
		}
		if item.AllowedRoutes != nil {
			for _, kind := range item.AllowedRoutes.Kinds {
				listener.RouteConstraints = append(listener.RouteConstraints, string(kind.Kind))
			}
		}
		listener.Conditions = convertConditions(statusByName[string(item.Name)])
		listeners = append(listeners, listener)
	}
	return listeners
}

func auditRoute(
	resource source.Resource,
	gateway model.ResourceIdentity,
	listeners []Listener,
	snapshot source.Snapshot,
) (Route, error) {
	route := Route{Identity: resource.Identity}
	switch object := resource.Object.(type) {
	case *gatewayv1.HTTPRoute:
		route.Hostnames = stringSlice(object.Spec.Hostnames)
		route.ParentRefs = matchingParentRefs(object.Spec.ParentRefs, object.Namespace, gateway)
		route.Acceptance = routeAcceptances(object.Spec.ParentRefs, object.Status.Parents, object.Namespace, gateway)
		for i, rule := range object.Spec.Rules {
			auditRule := Rule{Index: i + 1, Precedence: PrecedenceWinner}
			if !object.CreationTimestamp.IsZero() {
				auditRule.CreatedAt = &object.CreationTimestamp
			}
			for _, match := range rule.Matches {
				auditRule.Matches = append(auditRule.Matches, httpMatch(match))
			}
			if len(rule.Matches) == 0 {
				auditRule.Matches = []Match{{}}
			}
			auditRule.ListenerNames = listenerNames(route.ParentRefs, listeners, route.Identity.Kind, route.Hostnames)
			for _, backend := range rule.BackendRefs {
				auditRule.Backends = append(auditRule.Backends, makeBackend(
					resource.Identity, backend.BackendObjectReference, snapshot,
				))
			}
			route.Rules = append(route.Rules, auditRule)
		}
	case *gatewayv1.GRPCRoute:
		route.Hostnames = stringSlice(object.Spec.Hostnames)
		route.ParentRefs = matchingParentRefs(object.Spec.ParentRefs, object.Namespace, gateway)
		route.Acceptance = routeAcceptances(object.Spec.ParentRefs, object.Status.Parents, object.Namespace, gateway)
		for i, rule := range object.Spec.Rules {
			auditRule := Rule{Index: i + 1, Precedence: PrecedenceWinner}
			if !object.CreationTimestamp.IsZero() {
				auditRule.CreatedAt = &object.CreationTimestamp
			}
			for _, match := range rule.Matches {
				auditRule.Matches = append(auditRule.Matches, grpcMatch(match))
			}
			if len(rule.Matches) == 0 {
				auditRule.Matches = []Match{{}}
			}
			auditRule.ListenerNames = listenerNames(route.ParentRefs, listeners, route.Identity.Kind, route.Hostnames)
			for _, backend := range rule.BackendRefs {
				auditRule.Backends = append(auditRule.Backends, makeBackend(
					resource.Identity, backend.BackendObjectReference, snapshot,
				))
			}
			route.Rules = append(route.Rules, auditRule)
		}
	default:
		return Route{}, fmt.Errorf("decode route %s %s/%s: unexpected object type %T",
			resource.Identity.Kind, resource.Identity.Namespace, resource.Identity.Name, resource.Object)
	}
	return route, nil
}

func matchingParentRefs(refs []gatewayv1.ParentReference, routeNamespace string, gateway model.ResourceIdentity) []ParentReference {
	var parents []ParentReference
	for _, ref := range refs {
		if !parentTargets(ref, routeNamespace, gateway) {
			continue
		}
		parents = append(parents, ParentReference{
			Identity:    gateway,
			SectionName: pointerString(ref.SectionName),
		})
	}
	return parents
}

func routeAcceptances(
	parentRefs []gatewayv1.ParentReference,
	parents []gatewayv1.RouteParentStatus,
	routeNamespace string,
	gateway model.ResourceIdentity,
) []RouteAcceptance {
	var acceptances []RouteAcceptance
	for _, status := range parents {
		if !parentTargets(status.ParentRef, routeNamespace, gateway) {
			continue
		}
		acceptance := RouteAcceptance{
			Parent: ParentReference{
				Identity: gateway, SectionName: pointerString(status.ParentRef.SectionName),
			},
			ListenerName: pointerString(status.ParentRef.SectionName), State: RouteStateUnknown,
		}
		for _, condition := range status.Conditions {
			if condition.Type != "Accepted" {
				continue
			}
			switch condition.Status {
			case "True":
				acceptance.State = RouteStateAccepted
			case "False":
				acceptance.State = RouteStateRejected
			default:
				acceptance.State = RouteStateUnknown
			}
			acceptance.Reason = condition.Reason
			acceptance.Message = condition.Message
			break
		}
		acceptances = append(acceptances, acceptance)
	}
	if len(acceptances) == 0 {
		for _, ref := range parentRefs {
			if parentTargets(ref, routeNamespace, gateway) {
				acceptances = append(acceptances, RouteAcceptance{
					Parent: ParentReference{Identity: gateway, SectionName: pointerString(ref.SectionName)},
					State:  RouteStateUnknown,
				})
			}
		}
	}
	return acceptances
}

func listenerNames(
	parents []ParentReference,
	listeners []Listener,
	routeKind string,
	hostnames []string,
) []string {
	names := make([]string, 0, len(parents))
	for _, parent := range parents {
		if parent.SectionName != "" {
			names = append(names, parent.SectionName)
			continue
		}
		for _, listener := range listeners {
			if !listenerAllowsKind(listener, routeKind) || !listenerIntersectsHostnames(listener, hostnames) {
				continue
			}
			names = append(names, listener.Name)
		}
	}
	sort.Strings(names)
	return uniqueStrings(names)
}

func listenerAllowsKind(listener Listener, routeKind string) bool {
	if listener.Protocol != "HTTP" && listener.Protocol != "HTTPS" {
		return false
	}
	if len(listener.RouteConstraints) == 0 {
		return true
	}
	for _, kind := range listener.RouteConstraints {
		if kind == routeKind {
			return true
		}
	}
	return false
}

func listenerIntersectsHostnames(listener Listener, routeHostnames []string) bool {
	if len(listener.Hostnames) == 0 || len(routeHostnames) == 0 {
		return true
	}
	for _, listenerHostname := range listener.Hostnames {
		for _, routeHostname := range routeHostnames {
			if resolve.HostnamesIntersect(listenerHostname, routeHostname) {
				return true
			}
		}
	}
	return false
}

func httpMatch(match gatewayv1.HTTPRouteMatch) Match {
	result := Match{Path: &PathMatch{Type: "PathPrefix", Value: "/"}}
	if match.Path != nil {
		pathType := ""
		if match.Path.Type != nil {
			pathType = string(*match.Path.Type)
		}
		result.Path = &PathMatch{Type: pathType, Value: derefString(match.Path.Value)}
	}
	if match.Method != nil {
		result.Method = string(*match.Method)
	}
	for _, header := range match.Headers {
		matchType := ""
		if header.Type != nil {
			matchType = string(*header.Type)
		}
		result.Headers = append(result.Headers, StringMatch{
			Name: string(header.Name), Type: matchType, Value: header.Value,
		})
	}
	for _, query := range match.QueryParams {
		matchType := ""
		if query.Type != nil {
			matchType = string(*query.Type)
		}
		result.QueryParams = append(result.QueryParams, StringMatch{
			Name: string(query.Name), Type: matchType, Value: query.Value,
		})
	}
	return result
}

func grpcMatch(match gatewayv1.GRPCRouteMatch) Match {
	result := Match{GRPC: &GRPCMatch{}}
	if match.Method != nil {
		result.GRPC.Service = derefString(match.Method.Service)
		result.GRPC.Method = derefString(match.Method.Method)
	}
	for _, header := range match.Headers {
		matchType := ""
		if header.Type != nil {
			matchType = string(*header.Type)
		}
		result.Headers = append(result.Headers, StringMatch{
			Name: string(header.Name), Type: matchType, Value: header.Value,
		})
	}
	return result
}

func makeBackend(route model.ResourceIdentity, ref gatewayv1.BackendObjectReference, snapshot source.Snapshot) Backend {
	namespace := route.Namespace
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}
	group, kind := "", "Service"
	if ref.Group != nil {
		group = string(*ref.Group)
	}
	if ref.Kind != nil {
		kind = string(*ref.Kind)
	}
	backend := Backend{
		Reference: BackendReference{
			Group: group, Kind: kind, Namespace: namespace, Name: string(ref.Name), Port: ref.Port,
		},
	}
	if group != "" || kind != "Service" {
		return backend
	}
	serviceResource := findResource(snapshot, "", "Service", namespace, backend.Reference.Name)
	if serviceResource == nil {
		backend.Service = &BackendService{
			Identity:     model.ResourceIdentity{Version: "v1", Kind: "Service", Namespace: namespace, Name: backend.Reference.Name},
			Availability: model.AvailabilityNotFound,
		}
	} else if service, ok := serviceResource.Object.(*corev1.Service); ok {
		resolved := &BackendService{
			Identity: serviceResource.Identity, Availability: model.AvailabilityPresent,
			EndpointSlicesSupplied: false,
		}
		for _, port := range service.Spec.Ports {
			resolved.Ports = append(resolved.Ports, port.Port)
			if ref.Port != nil && port.Port == int32(*ref.Port) {
				present := true
				resolved.PortPresent = &present
			}
		}
		if ref.Port != nil && resolved.PortPresent == nil {
			present := false
			resolved.PortPresent = &present
		}
		for _, resource := range snapshot.Resources {
			slice, ok := resource.Object.(*discoveryv1.EndpointSlice)
			if !ok || slice.Namespace != namespace || slice.Labels["kubernetes.io/service-name"] != backend.Reference.Name {
				continue
			}
			resolved.EndpointSlicesSupplied = true
			for _, endpoint := range slice.Endpoints {
				if endpoint.Conditions.Ready != nil && *endpoint.Conditions.Ready {
					resolved.ReadyAddressCount += len(endpoint.Addresses)
				}
			}
		}
		backend.Service = resolved
	}
	if namespace != route.Namespace && hasAvailableFamily(snapshot.Metadata, "referencegrant") {
		backend.Grant = referenceGrantUse(route, backend.Reference, snapshot)
	}
	return backend
}

func referenceGrantUse(route model.ResourceIdentity, backend BackendReference, snapshot source.Snapshot) *ReferenceGrantUse {
	use := &ReferenceGrantUse{}
	for _, resource := range snapshot.Resources {
		if resource.Identity.Group != gatewayAPIGroup || resource.Identity.Kind != "ReferenceGrant" ||
			resource.Identity.Namespace != backend.Namespace {
			continue
		}
		grant, ok := resource.Object.(*gatewayv1beta1.ReferenceGrant)
		if !ok {
			continue
		}
		converted := ReferenceGrant{Identity: resource.Identity}
		allows := false
		for _, from := range grant.Spec.From {
			constraint := GrantConstraint{
				Group: string(from.Group), Kind: string(from.Kind), Namespace: string(from.Namespace),
			}
			converted.From = append(converted.From, constraint)
			if constraint.Group == route.Group && constraint.Kind == route.Kind && constraint.Namespace == route.Namespace {
				for _, to := range grant.Spec.To {
					toConstraint := GrantConstraint{
						Group: string(to.Group), Kind: string(to.Kind), Name: objectName(to.Name),
					}
					converted.To = append(converted.To, toConstraint)
					if toConstraint.Group == "" && toConstraint.Kind == "Service" &&
						(toConstraint.Name == "" || toConstraint.Name == backend.Name) {
						allows = true
					}
				}
			}
		}
		use.Grants = append(use.Grants, converted)
		use.Authorized = use.Authorized || allows
	}
	return use
}

func waypointSelections(snapshot source.Snapshot, gateway model.ResourceIdentity, programmed *model.Condition) []WaypointSelection {
	return waypointSelectionsForTarget(
		snapshot, gateway.Namespace, gateway.Name, true, programmed,
	)
}

func missingWaypointSelections(snapshot source.Snapshot, namespace, name string) []WaypointSelection {
	return waypointSelectionsForTarget(snapshot, namespace, name, false, nil)
}

func waypointSelectionsForTarget(
	snapshot source.Snapshot,
	requestedNamespace string,
	requestedName string,
	targetFound bool,
	programmed *model.Condition,
) []WaypointSelection {
	var selections []WaypointSelection
	for _, resource := range snapshot.Resources {
		var labels map[string]string
		switch object := resource.Object.(type) {
		case *corev1.Namespace:
			labels = object.Labels
		case *corev1.Service:
			labels = object.Labels
		default:
			continue
		}
		value := labels["istio.io/use-waypoint"]
		if value == "" {
			continue
		}
		waypointNamespace, waypointName := resource.Identity.Namespace, value
		if resource.Identity.Kind == "Namespace" {
			waypointNamespace = resource.Identity.Name
		}
		qualified := strings.Contains(value, "/")
		if qualified {
			parts := strings.SplitN(value, "/", 2)
			waypointNamespace, waypointName = parts[0], parts[1]
		}
		if waypointName != requestedName ||
			(requestedNamespace != "" && waypointNamespace != requestedNamespace) {
			continue
		}
		selections = append(selections, WaypointSelection{
			Resource: resource.Identity, LabelValue: value,
			WaypointNamespace: waypointNamespace, WaypointName: waypointName,
			NamespaceQualified: qualified, TargetFound: targetFound, Programmed: programmed,
		})
	}
	sort.Slice(selections, func(i, j int) bool {
		return identityKey(selections[i].Resource) < identityKey(selections[j].Resource)
	})
	return selections
}

func isWaypoint(gateway *gatewayv1.Gateway) bool {
	return string(gateway.Spec.GatewayClassName) == "istio-waypoint"
}

func conditionByType(conditions []metav1.Condition, kind string) *model.Condition {
	for _, condition := range convertConditions(conditions) {
		if condition.Type == kind {
			return &condition
		}
	}
	return nil
}

func convertConditions(conditions []metav1.Condition) []model.Condition {
	result := make([]model.Condition, 0, len(conditions))
	for _, condition := range conditions {
		result = append(result, model.Condition{
			Type: condition.Type, Status: model.ConditionStatus(condition.Status),
			Reason: condition.Reason, Message: condition.Message,
		})
	}
	return result
}

func findResource(snapshot source.Snapshot, group, kind, namespace, name string) *source.Resource {
	for i := range snapshot.Resources {
		resource := &snapshot.Resources[i]
		if resource.Identity.Group == group && resource.Identity.Kind == kind &&
			resource.Identity.Namespace == namespace && resource.Identity.Name == name {
			return resource
		}
	}
	return nil
}

func hasEndpointSlices(snapshot source.Snapshot) bool {
	for _, resource := range snapshot.Resources {
		if resource.Identity.Kind == "EndpointSlice" {
			return true
		}
	}
	return false
}

func hasAvailableFamily(metadata model.SourceSnapshot, family string) bool {
	for _, status := range metadata.Families {
		if status.Family == family {
			return status.Available
		}
	}
	return metadata.Mode == model.SourceModeFiles
}

func parentTargets(ref gatewayv1.ParentReference, routeNamespace string, gateway model.ResourceIdentity) bool {
	return parentReferenceTargets(ref, routeNamespace, gateway)
}

func stringSlice(values []gatewayv1.Hostname) []string {
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = string(value)
	}
	return result
}

func pointerString(value *gatewayv1.SectionName) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func objectName(value *gatewayv1.ObjectName) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func uniqueStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}

func identityKey(identity model.ResourceIdentity) string {
	return strings.Join([]string{identity.Group, identity.Kind, identity.Namespace, identity.Name}, "/")
}
