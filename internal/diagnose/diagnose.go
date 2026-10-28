package diagnose

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ninoamine/istio-route/internal/model"
	"github.com/ninoamine/istio-route/internal/source"
)

const gatewayGroup = "gateway.networking.k8s.io"

func Diagnose(analysis *model.Analysis, snapshot source.Snapshot) error {
	if analysis == nil {
		return fmt.Errorf("diagnose analysis: analysis is required")
	}
	if analysis.Hostname == "" {
		return fmt.Errorf("diagnose analysis: hostname is required")
	}

	addFinding := findingAdder(analysis)
	diagnoseGatewayRouteReferences(analysis, snapshot, addFinding)
	diagnoseConditions(analysis, snapshot, addFinding)
	diagnoseIstioBindings(analysis, snapshot, addFinding)
	diagnoseWarnings(analysis, addFinding)
	sortFindings(analysis.Findings)
	return nil
}

type addFindingFunc func(model.Finding)

func findingAdder(analysis *model.Analysis) addFindingFunc {
	return func(finding model.Finding) {
		sort.Slice(finding.Resources, func(i, j int) bool {
			return identityKey(finding.Resources[i]) < identityKey(finding.Resources[j])
		})
		for _, existing := range analysis.Findings {
			if existing.Code == finding.Code &&
				findingResourcesKey(existing.Resources) == findingResourcesKey(finding.Resources) &&
				existing.Message == finding.Message {
				return
			}
		}
		analysis.Findings = append(analysis.Findings, finding)
	}
}

func sortFindings(findings []model.Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Code != findings[j].Code {
			return findings[i].Code < findings[j].Code
		}
		if resourcesKey := findingResourcesKey(findings[i].Resources); resourcesKey != findingResourcesKey(findings[j].Resources) {
			return resourcesKey < findingResourcesKey(findings[j].Resources)
		}
		return findings[i].Message < findings[j].Message
	})
}

func findingResourcesKey(resources []model.ResourceIdentity) string {
	keys := make([]string, 0, len(resources))
	for _, resource := range resources {
		keys = append(keys, identityKey(resource))
	}
	sort.Strings(keys)
	return strings.Join(keys, "|")
}

func identityKey(identity model.ResourceIdentity) string {
	return strings.Join([]string{
		identity.Group, identity.Version, identity.Kind, identity.Namespace, identity.Name,
	}, "/")
}

func addWarning(add addFindingFunc, code, message string, resources ...model.ResourceIdentity) {
	add(model.Finding{
		Code: code, Severity: model.FindingSeverityWarning,
		Message: message, Resources: resources,
	})
}

func sameResource(first, second model.ResourceIdentity) bool {
	return first.Group == second.Group &&
		first.Kind == second.Kind &&
		first.Namespace == second.Namespace &&
		first.Name == second.Name
}
