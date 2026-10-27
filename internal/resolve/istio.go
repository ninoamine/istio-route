package resolve

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ninoamine/istio-route/internal/model"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	istioclientv1 "istio.io/client-go/pkg/apis/networking/v1"
)

func resolveIstio(
	ctx context.Context,
	hostname string,
	resources catalog,
	endpoints *endpointResolver,
	analysis *model.Analysis,
) error {
	serviceEntries := make([]*istioclientv1.ServiceEntry, 0)
	for _, entry := range resources.serviceEntries {
		for _, host := range entry.Spec.Hosts {
			if MatchesHostname(hostname, host) {
				serviceEntries = append(serviceEntries, entry)
				break
			}
		}
	}

	type gatewayEntry struct {
		gateway *istioclientv1.Gateway
		server  *istiov1alpha3.Server
		index   int
	}
	matchingGateways := make([]gatewayEntry, 0)
	for _, gateway := range resources.istioGateways {
		for index, server := range gateway.Spec.Servers {
			if server == nil || server.Port == nil || !anyIstioHostMatches(hostname, gateway.Namespace, server.Hosts) {
				continue
			}
			entry := model.EntryPoint{
				Origin:   model.OriginIstio,
				Resource: identity("networking.istio.io", "v1", "Gateway", gateway.Namespace, gateway.Name),
				Name:     server.Name,
				Port:     int32(server.Port.Number),
				Protocol: server.Port.Protocol,
			}
			if entry.Name == "" {
				entry.Name = fmt.Sprintf("server-%d", index+1)
			}
			entry.RouteConstraints = append(entry.RouteConstraints, server.Hosts...)
			if server.Tls != nil {
				entry.TLS = &model.TLSConfig{Mode: server.Tls.Mode.String()}
				names := append([]string(nil), server.Tls.CredentialNames...)
				if server.Tls.CredentialName != "" {
					names = append(names, server.Tls.CredentialName)
				}
				for _, secret := range names {
					namespace, name := gateway.Namespace, secret
					if strings.Contains(secret, "/") {
						parts := strings.SplitN(secret, "/", 2)
						namespace, name = parts[0], parts[1]
					}
					entry.TLS.CertificateRefs = append(entry.TLS.CertificateRefs, identity("", "v1", "Secret", namespace, name))
				}
			}
			analysis.EntryPoints = append(analysis.EntryPoints, entry)
			matchingGateways = append(matchingGateways, gatewayEntry{gateway: gateway, server: server, index: index})
		}
	}

	for _, virtualService := range resources.virtualServices {
		if !anyIstioHostMatches(hostname, virtualService.Namespace, virtualService.Spec.Hosts) {
			continue
		}
		parents := istioGatewayParents(virtualService)
		attachedParents := make([]model.ResourceIdentity, 0)
		for _, gateway := range matchingGateways {
			if !referencesGateway(parents, virtualService.Namespace, gateway.gateway.Namespace, gateway.gateway.Name) {
				continue
			}
			if !anyIstioHostsIntersect(virtualService.Namespace, virtualService.Spec.Hosts, gateway.gateway.Namespace, gateway.server.Hosts) {
				continue
			}
			attachedParents = appendIdentityOnce(attachedParents, identity(
				"networking.istio.io", "v1", "Gateway", gateway.gateway.Namespace, gateway.gateway.Name,
			))
		}
		if len(attachedParents) == 0 {
			continue
		}
		route := model.Route{
			Origin:     model.OriginIstio,
			Resource:   identity("networking.istio.io", "v1", "VirtualService", virtualService.Namespace, virtualService.Name),
			Hostnames:  matchingIstioHosts(hostname, virtualService.Namespace, virtualService.Spec.Hosts),
			ParentRefs: attachedParents,
		}
		for index, httpRoute := range virtualService.Spec.Http {
			if httpRoute == nil {
				continue
			}
			rule := model.Rule{
				Index:   index + 1,
				Filters: istioFilters(httpRoute),
			}
			for _, match := range httpRoute.Match {
				if match != nil {
					rule.Matches = append(rule.Matches, istioMatch(match))
				}
			}
			if len(rule.Matches) == 0 {
				rule.Matches = []model.Match{{PathType: "default"}}
			}
			for _, destination := range httpRoute.Route {
				if destination == nil || destination.Destination == nil {
					continue
				}
				resolved, err := resolveIstioDestination(ctx, virtualService.Namespace, destination, resources.destinationRules, serviceEntries, endpoints)
				if err != nil {
					return err
				}
				rule.Destinations = append(rule.Destinations, resolved)
			}
			route.Rules = append(route.Rules, rule)
		}
		analysis.Routes = append(analysis.Routes, route)
	}

	for _, entry := range serviceEntries {
		identityValue := identity("networking.istio.io", "v1", "ServiceEntry", entry.Namespace, entry.Name)
		route := model.Route{
			Origin:    model.OriginIstio,
			Resource:  identityValue,
			Hostnames: matchingServiceEntryHosts(hostname, entry.Spec.Hosts),
			Rules: []model.Rule{{
				Index:   1,
				Matches: []model.Match{{PathType: "default"}},
				Destinations: []model.Destination{{
					Kind:     model.DestinationKindExternal,
					Name:     firstHost(entry.Spec.Hosts),
					Weight:   weightPointer(100),
					External: serviceEntryDestination(entry),
				}},
			}},
		}
		analysis.Routes = append(analysis.Routes, route)
	}
	return nil
}

func resolveIstioDestination(
	ctx context.Context,
	namespace string,
	configured *istiov1alpha3.HTTPRouteDestination,
	destinationRules []*istioclientv1.DestinationRule,
	serviceEntries []*istioclientv1.ServiceEntry,
	endpoints *endpointResolver,
) (model.Destination, error) {
	destination := configured.Destination
	host := destination.Host
	for _, entry := range serviceEntries {
		if anyHostnameMatches(host, entry.Spec.Hosts) {
			return model.Destination{
				Kind:     model.DestinationKindExternal,
				Name:     host,
				Weight:   weightPointer(configured.Weight),
				External: serviceEntryDestination(entry),
			}, nil
		}
	}
	serviceNamespace, serviceName := serviceReference(namespace, host)
	result := model.Destination{
		Kind:      model.DestinationKindService,
		Namespace: serviceNamespace,
		Name:      serviceName,
		Subset:    destination.Subset,
		Weight:    weightPointer(configured.Weight),
	}
	if destination.Port != nil && destination.Port.Number != 0 {
		port := int32(destination.Port.Number)
		result.Port = &port
	}
	for _, rule := range destinationRules {
		if rule.Namespace != namespace && rule.Namespace != serviceNamespace {
			continue
		}
		if !istioRuleHostMatches(host, rule.Namespace, rule.Spec.Host) {
			continue
		}
		ruleIdentity := identity("networking.istio.io", "v1", "DestinationRule", rule.Namespace, rule.Name)
		for _, subset := range rule.Spec.Subsets {
			if subset != nil && subset.Name == destination.Subset {
				result.DestinationRule = &ruleIdentity
				result.SubsetLabels = subset.Labels
				break
			}
		}
		if result.DestinationRule != nil {
			break
		}
	}
	if destination.Subset == "" {
		result.Subset = ""
	}
	service, err := endpoints.resolve(ctx, serviceNamespace, serviceName)
	if err != nil {
		return model.Destination{}, err
	}
	result.Service = service
	return result, nil
}

func istioMatch(match *istiov1alpha3.HTTPMatchRequest) model.Match {
	result := model.Match{
		Headers:     make(map[string]string),
		QueryParams: make(map[string]string),
		Method:      stringMatchValue(match.Method),
	}
	if match.Uri != nil {
		result.PathType, result.Path = stringMatchTypeValue(match.Uri)
	}
	for name, value := range match.Headers {
		result.Headers[name] = stringMatchValue(value)
	}
	for name, value := range match.QueryParams {
		result.QueryParams[name] = stringMatchValue(value)
	}
	if len(result.Headers) == 0 {
		result.Headers = nil
	}
	if len(result.QueryParams) == 0 {
		result.QueryParams = nil
	}
	return result
}

func istioFilters(route *istiov1alpha3.HTTPRoute) []model.Filter {
	filters := make([]model.Filter, 0, 5)
	if route.Rewrite != nil {
		details := make(map[string]string)
		if route.Rewrite.Uri != "" {
			details["uri"] = route.Rewrite.Uri
		}
		if route.Rewrite.Authority != "" {
			details["authority"] = route.Rewrite.Authority
		}
		filters = append(filters, model.Filter{Type: "Rewrite", Details: details})
	}
	if route.Redirect != nil {
		details := make(map[string]string)
		if route.Redirect.Uri != "" {
			details["uri"] = route.Redirect.Uri
		}
		if route.Redirect.Authority != "" {
			details["authority"] = route.Redirect.Authority
		}
		if route.Redirect.RedirectCode != 0 {
			details["code"] = strconv.FormatUint(uint64(route.Redirect.RedirectCode), 10)
		}
		filters = append(filters, model.Filter{Type: "Redirect", Details: details})
	}
	if route.Timeout != nil {
		filters = append(filters, model.Filter{Type: "Timeout", Details: map[string]string{
			"value": time.Duration(route.Timeout.Seconds*int64(time.Second) + int64(route.Timeout.Nanos)).String(),
		}})
	}
	if route.Retries != nil {
		details := map[string]string{
			"attempts": strconv.FormatUint(uint64(route.Retries.Attempts), 10),
			"retryOn":  route.Retries.RetryOn,
		}
		if route.Retries.PerTryTimeout != nil {
			details["perTryTimeout"] = time.Duration(route.Retries.PerTryTimeout.Seconds*int64(time.Second) + int64(route.Retries.PerTryTimeout.Nanos)).String()
		}
		filters = append(filters, model.Filter{Type: "Retries", Details: details})
	}
	if route.Mirror != nil {
		filters = append(filters, model.Filter{Type: "Mirror", Details: map[string]string{
			"host": route.Mirror.Host, "subset": route.Mirror.Subset,
		}})
	}
	return filters
}

func stringMatchValue(match *istiov1alpha3.StringMatch) string {
	_, value := stringMatchTypeValue(match)
	return value
}

func stringMatchTypeValue(match *istiov1alpha3.StringMatch) (string, string) {
	if match == nil {
		return "", ""
	}
	switch value := match.MatchType.(type) {
	case *istiov1alpha3.StringMatch_Exact:
		return "exact", value.Exact
	case *istiov1alpha3.StringMatch_Prefix:
		return "prefix", value.Prefix
	case *istiov1alpha3.StringMatch_Regex:
		return "regex", value.Regex
	default:
		return "", ""
	}
}

func serviceEntryDestination(entry *istioclientv1.ServiceEntry) *model.ExternalDestination {
	result := &model.ExternalDestination{
		Hosts:      append([]string(nil), entry.Spec.Hosts...),
		Location:   entry.Spec.Location.String(),
		Resolution: entry.Spec.Resolution.String(),
	}
	for _, port := range entry.Spec.Ports {
		if port == nil {
			continue
		}
		result.Ports = append(result.Ports, model.ExternalPort{
			Name: port.Name, Number: port.Number, Protocol: port.Protocol,
		})
	}
	return result
}

func matchingIstioHosts(hostname, namespace string, hosts []string) []string {
	result := make([]string, 0)
	for _, host := range hosts {
		if MatchesIstioHost(hostname, namespace, host) {
			result = append(result, host)
		}
	}
	return result
}

func matchingServiceEntryHosts(hostname string, hosts []string) []string {
	result := make([]string, 0)
	for _, host := range hosts {
		if MatchesHostname(hostname, host) {
			result = append(result, host)
		}
	}
	return result
}

func anyIstioHostMatches(hostname, namespace string, hosts []string) bool {
	for _, host := range hosts {
		if MatchesIstioHost(hostname, namespace, host) {
			return true
		}
	}
	return false
}

func anyHostnameMatches(hostname string, hosts []string) bool {
	for _, host := range hosts {
		if MatchesHostname(hostname, host) {
			return true
		}
	}
	return false
}

func anyIstioHostsIntersect(firstNamespace string, first []string, secondNamespace string, second []string) bool {
	for _, firstHost := range first {
		firstName := stripIstioNamespace(firstHost, firstNamespace)
		for _, secondHost := range second {
			secondName := stripIstioNamespace(secondHost, secondNamespace)
			if HostnamesIntersect(firstName, secondName) {
				return true
			}
		}
	}
	return false
}

func stripIstioNamespace(host, namespace string) string {
	parts := strings.SplitN(host, "/", 2)
	if len(parts) == 1 {
		return host
	}
	if parts[0] != "*" && parts[0] != "." && parts[0] != namespace {
		return ""
	}
	return parts[1]
}

func istioGatewayParents(virtualService *istioclientv1.VirtualService) []string {
	return append([]string(nil), virtualService.Spec.Gateways...)
}

func referencesGateway(parents []string, routeNamespace, gatewayNamespace, gatewayName string) bool {
	for _, parent := range parents {
		if parent == "mesh" {
			continue
		}
		namespace, name := routeNamespace, parent
		if strings.Contains(parent, "/") {
			parts := strings.SplitN(parent, "/", 2)
			namespace, name = parts[0], parts[1]
		}
		if namespace == gatewayNamespace && name == gatewayName {
			return true
		}
	}
	return false
}

func istioRuleHostMatches(destinationHost, ruleNamespace, ruleHost string) bool {
	if !strings.Contains(ruleHost, ".") && !strings.Contains(destinationHost, ".") {
		return ruleHost == destinationHost
	}
	if !strings.Contains(ruleHost, ".") {
		ruleHost += "." + ruleNamespace + ".svc.cluster.local"
	}
	if !strings.Contains(destinationHost, ".") {
		destinationHost += "." + ruleNamespace + ".svc.cluster.local"
	}
	return MatchesHostname(destinationHost, ruleHost)
}

func serviceReference(defaultNamespace, host string) (string, string) {
	host = strings.TrimSuffix(host, ".")
	if strings.HasSuffix(host, ".svc.cluster.local") {
		service := strings.TrimSuffix(host, ".svc.cluster.local")
		parts := strings.Split(service, ".")
		if len(parts) >= 2 {
			return parts[len(parts)-1], parts[0]
		}
	}
	if strings.HasSuffix(host, ".svc") {
		service := strings.TrimSuffix(host, ".svc")
		parts := strings.Split(service, ".")
		if len(parts) >= 2 {
			return parts[len(parts)-1], parts[0]
		}
	}
	return defaultNamespace, host
}

func firstHost(hosts []string) string {
	if len(hosts) == 0 {
		return ""
	}
	return hosts[0]
}

func appendIdentityOnce(identities []model.ResourceIdentity, identityValue model.ResourceIdentity) []model.ResourceIdentity {
	for _, existing := range identities {
		if existing == identityValue {
			return identities
		}
	}
	return append(identities, identityValue)
}

func weightPointer(value int32) *int32 {
	return &value
}
