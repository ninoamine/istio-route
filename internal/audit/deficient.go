package audit

import (
	"fmt"

	"github.com/ninoamine/istio-route/internal/model"
)

func deficientFindings(result Result) []model.Finding {
	var findings []model.Finding
	for _, route := range result.Routes {
		for _, rule := range route.Rules {
			for _, backend := range rule.Backends {
				service := backend.Service
				if service == nil || !service.EndpointSlicesSupplied || service.ReadyAddressCount > 0 {
					continue
				}
				severity := model.FindingSeverityWarning
				if rule.Precedence == PrecedenceWinner {
					severity = model.FindingSeverityError
				}
				findings = append(findings, model.Finding{
					Code:         "deficient-destination",
					Severity:     severity,
					Message:      fmt.Sprintf("%s %s/%s rule %d backend %s/%s has no ready addresses", route.Identity.Kind, route.Identity.Namespace, route.Identity.Name, rule.Index, backend.Reference.Namespace, backend.Reference.Name),
					Resources:    []model.ResourceIdentity{route.Identity, service.Identity},
					Category:     string(CategoryDeficientDestination),
					SuggestedFix: "Ensure the backend Service has ready EndpointSlice addresses or change the route to a healthy destination.",
				})
			}
		}
	}
	return findings
}
