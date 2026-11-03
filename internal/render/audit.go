package render

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ninoamine/istio-route/internal/audit"
	"sigs.k8s.io/yaml"
)

func RenderAudit(result audit.Result, format string, options Options) (string, error) {
	switch format {
	case FormatText:
		return auditText(result, options), nil
	case FormatJSON:
		contents, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return "", fmt.Errorf("encode audit JSON report: %w", err)
		}
		return string(contents) + "\n", nil
	case FormatYAML:
		contents, err := yaml.Marshal(result)
		if err != nil {
			return "", fmt.Errorf("encode audit YAML report: %w", err)
		}
		return string(contents), nil
	case FormatMermaid:
		return auditMermaid(result), nil
	default:
		return "", fmt.Errorf("unsupported output format %q", format)
	}
}

func auditText(result audit.Result, options Options) string {
	styles := newTextStyles(options.Color)
	var output strings.Builder
	fmt.Fprintf(&output, "%s %s/%s (class: %s, controller: %s)\n",
		styles.root("Gateway"), result.Gateway.Identity.Namespace, result.Gateway.Identity.Name,
		result.Gateway.Class.Identity.Name, result.Gateway.Class.Controller)
	for _, listener := range result.Listeners {
		fmt.Fprintf(&output, "  listener %s :%d %s", listener.Name, listener.Port, listener.Protocol)
		if len(listener.Hostnames) > 0 {
			fmt.Fprintf(&output, " hostnames=%s", strings.Join(listener.Hostnames, ","))
		}
		output.WriteByte('\n')
	}
	for _, route := range result.Routes {
		fmt.Fprintf(&output, "  %s %s/%s\n", route.Identity.Kind, route.Identity.Namespace, route.Identity.Name)
		for _, acceptance := range route.Acceptance {
			fmt.Fprintf(&output, "    acceptance: %s", acceptance.State)
			if acceptance.Reason != "" {
				fmt.Fprintf(&output, " (%s)", acceptance.Reason)
			}
			output.WriteByte('\n')
		}
		for _, rule := range route.Rules {
			fmt.Fprintf(&output, "    rule %d: %s\n", rule.Index, rule.Precedence)
			for _, backend := range rule.Backends {
				fmt.Fprintf(&output, "      backend %s/%s", backend.Reference.Namespace, backend.Reference.Name)
				if backend.Service != nil {
					fmt.Fprintf(&output, " (%s", backend.Service.Availability)
					if backend.Service.EndpointSlicesSupplied {
						fmt.Fprintf(&output, ", %d ready addresses", backend.Service.ReadyAddressCount)
						if backend.Service.ReadyAddressCount == 0 {
							output.WriteString(", deficient")
						}
					}
					output.WriteByte(')')
				}
				output.WriteByte('\n')
			}
		}
	}
	for _, selection := range result.WaypointSelections {
		selector := selection.Resource.Namespace + "/" + selection.Resource.Name
		if selection.Resource.Kind == "Namespace" {
			selector = selection.Resource.Name
		}
		fmt.Fprintf(&output, "  selection %s %s -> %s/%s (%s)\n",
			selection.Resource.Kind, selector,
			selection.WaypointNamespace, selection.WaypointName, selection.LabelValue)
	}
	fmt.Fprintf(&output, "endpoint check: %s", result.EndpointCheck.State)
	if result.EndpointCheck.Reason != "" {
		fmt.Fprintf(&output, " (%s)", result.EndpointCheck.Reason)
	}
	output.WriteByte('\n')
	for _, family := range result.Source.Families {
		if !family.Available {
			fmt.Fprintf(&output, "source family unavailable: %s (%s)\n", family.Family, family.Reason)
		}
	}
	for _, finding := range result.Findings {
		fmt.Fprintf(&output, "%s %s: %s\n", finding.Severity, finding.Code, finding.Message)
	}
	return output.String()
}

func auditMermaid(result audit.Result) string {
	var output strings.Builder
	output.WriteString("flowchart LR\n")
	gatewayID := mermaidID("gateway", result.Gateway.Identity.Namespace+"/"+result.Gateway.Identity.Name)
	fmt.Fprintf(&output, "  %s[\"Gateway %s/%s\"]\n", gatewayID,
		escapeMermaid(result.Gateway.Identity.Namespace), escapeMermaid(result.Gateway.Identity.Name))
	for _, route := range result.Routes {
		routeID := mermaidID(route.Identity.Kind, route.Identity.Namespace+"/"+route.Identity.Name)
		fmt.Fprintf(&output, "  %s[\"%s %s/%s\"]\n", routeID, route.Identity.Kind,
			escapeMermaid(route.Identity.Namespace), escapeMermaid(route.Identity.Name))
		fmt.Fprintf(&output, "  %s --> %s\n", gatewayID, routeID)
		for _, rule := range route.Rules {
			for _, backend := range rule.Backends {
				if backend.Reference.Kind != "" && backend.Reference.Kind != "Service" ||
					backend.Reference.Group != "" {
					continue
				}
				serviceID := mermaidID("service", backend.Reference.Namespace+"/"+backend.Reference.Name)
				label := fmt.Sprintf("Service %s/%s", escapeMermaid(backend.Reference.Namespace),
					escapeMermaid(backend.Reference.Name))
				if backend.Service != nil && backend.Service.Availability == "not-found" {
					label += " (missing)"
				}
				fmt.Fprintf(&output, "  %s[\"%s\"]\n", serviceID, escapeMermaid(label))
				fmt.Fprintf(&output, "  %s -->|%s| %s\n", routeID, rule.Precedence, serviceID)
			}
		}
	}
	for _, selection := range result.WaypointSelections {
		selector := selection.Resource.Namespace + "/" + selection.Resource.Name
		if selection.Resource.Kind == "Namespace" {
			selector = selection.Resource.Name
		}
		selectionID := mermaidID("selection", selection.Resource.Kind+"/"+selector)
		fmt.Fprintf(&output, "  %s[\"%s %s selects waypoint\"]\n", selectionID,
			selection.Resource.Kind, escapeMermaid(selector))
		fmt.Fprintf(&output, "  %s -.->|%s| %s\n", selectionID,
			escapeMermaid(selection.LabelValue), gatewayID)
	}
	endpointID := mermaidID("endpoint_check", string(result.EndpointCheck.State))
	endpointLabel := "Endpoint check: " + string(result.EndpointCheck.State)
	if result.EndpointCheck.Reason != "" {
		endpointLabel += " (" + result.EndpointCheck.Reason + ")"
	}
	fmt.Fprintf(&output, "  %s[\"%s\"]\n", endpointID, escapeMermaid(endpointLabel))
	fmt.Fprintf(&output, "  %s -.-> %s\n", gatewayID, endpointID)
	for i, finding := range result.Findings {
		findingID := mermaidID("finding", fmt.Sprintf("%d/%s/%s", i, finding.Code, finding.Message))
		fmt.Fprintf(&output, "  %s[\"%s %s: %s\"]\n", findingID,
			escapeMermaid(string(finding.Severity)), escapeMermaid(finding.Code), escapeMermaid(finding.Message))
		fmt.Fprintf(&output, "  %s -.-> %s\n", gatewayID, findingID)
	}
	return output.String()
}
