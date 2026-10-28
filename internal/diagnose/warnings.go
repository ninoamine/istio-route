package diagnose

import (
	"fmt"
	"strings"

	"github.com/ninoamine/istio-route/internal/model"
	"github.com/ninoamine/istio-route/internal/resolve"
)

func diagnoseWarnings(analysis *model.Analysis, add addFindingFunc) {
	if len(analysis.Routes) == 0 {
		addWarning(add, "no-matching-route", fmt.Sprintf("no supported route matches hostname %s", analysis.Hostname))
	}

	hasIstio, hasGatewayAPI := false, false
	for _, route := range analysis.Routes {
		switch route.Origin {
		case model.OriginIstio:
			hasIstio = true
		case model.OriginGatewayAPI:
			hasGatewayAPI = true
		}
		for _, rule := range route.Rules {
			diagnoseRuleWeights(route, rule, add)
			for _, destination := range rule.Destinations {
				if destination.ReferenceGrant != nil && !destination.ReferenceGrant.Allowed {
					addWarning(
						add, "missing-reference-grant",
						fmt.Sprintf(
							"cross-namespace backend %s/%s is not authorized by a ReferenceGrant: %s",
							destination.Namespace, destination.Name, destination.ReferenceGrant.Reason,
						),
						route.Resource,
					)
				}
				if route.Origin == model.OriginIstio &&
					destination.Subset != "" && destination.DestinationRule == nil {
					addWarning(
						add, "missing-destination-rule-subset",
						fmt.Sprintf(
							"destination %s/%s selects subset %s but no matching DestinationRule defines it",
							destination.Namespace, destination.Name, destination.Subset,
						),
						route.Resource,
					)
				}
			}
		}
	}
	if hasIstio && hasGatewayAPI {
		resources := make([]model.ResourceIdentity, 0, len(analysis.Routes))
		for _, route := range analysis.Routes {
			resources = append(resources, route.Resource)
		}
		addWarning(
			add, "mixed-api-hostname",
			fmt.Sprintf("%s is served by both Istio and Gateway API routes", analysis.Hostname),
			resources...,
		)
	}
	diagnoseWildcardOverlaps(analysis, add)
}

func diagnoseRuleWeights(route model.Route, rule model.Rule, add addFindingFunc) {
	if len(rule.Destinations) == 0 {
		return
	}
	var total int64
	allGatewayWeightsZero := true
	for _, destination := range rule.Destinations {
		if destination.Weight == nil {
			allGatewayWeightsZero = false
			continue
		}
		total += int64(*destination.Weight)
		if *destination.Weight != 0 {
			allGatewayWeightsZero = false
		}
	}
	if route.Origin == model.OriginIstio && total != 100 && (len(rule.Destinations) > 1 || total != 0) {
		addWarning(
			add, "invalid-istio-weights",
			fmt.Sprintf("Istio destination weights for rule %d on %s total %d; expected 100", rule.Index, route.Resource.Name, total),
			route.Resource,
		)
	}
	if route.Origin == model.OriginGatewayAPI && allGatewayWeightsZero {
		addWarning(
			add, "zero-gateway-backend-weights",
			fmt.Sprintf("all Gateway API backend weights for rule %d on %s are zero", rule.Index, route.Resource.Name),
			route.Resource,
		)
	}
}

type hostnamePattern struct {
	value    string
	resource model.ResourceIdentity
}

func diagnoseWildcardOverlaps(analysis *model.Analysis, add addFindingFunc) {
	patterns := make([]hostnamePattern, 0)
	for _, entry := range analysis.EntryPoints {
		if entry.Hostname != "" {
			patterns = append(patterns, hostnamePattern{
				value: normalizeIstioPattern(entry.Hostname), resource: entry.Resource,
			})
		}
	}
	for _, route := range analysis.Routes {
		for _, hostname := range route.Hostnames {
			patterns = append(patterns, hostnamePattern{
				value: normalizeIstioPattern(hostname), resource: route.Resource,
			})
		}
	}
	for i := 0; i < len(patterns); i++ {
		for j := i + 1; j < len(patterns); j++ {
			first, second := patterns[i], patterns[j]
			if sameResource(first.resource, second.resource) || !resolve.HostnamesIntersect(first.value, second.value) {
				continue
			}
			if isWildcard(first.value) == isWildcard(second.value) || first.value == second.value {
				continue
			}
			addWarning(
				add, "wildcard-hostname-overlap",
				fmt.Sprintf(
					"hostname patterns %s and %s overlap; the wildcard may shadow the more specific route",
					first.value, second.value,
				),
				first.resource, second.resource,
			)
		}
	}
}

func normalizeIstioPattern(hostname string) string {
	if _, pattern, found := strings.Cut(hostname, "/"); found {
		return pattern
	}
	return hostname
}

func isWildcard(hostname string) bool {
	return hostname == "*" || strings.HasPrefix(hostname, "*.")
}
