package render

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ninoamine/istio-route/internal/model"
)

func Mermaid(analysis model.Analysis) string {
	var builder strings.Builder
	builder.WriteString("flowchart TD\n")
	hostID := mermaidID("host", analysis.Hostname)
	writeMermaidNode(&builder, hostID, analysis.Hostname)

	entryIDs := make(map[string][]string)
	declaredParents := make(map[string]bool)
	for _, entry := range analysis.EntryPoints {
		id := entryPointID(entry)
		label := fmt.Sprintf("%s %s %s listener=%s", entry.Origin, entry.Resource.Kind, identityName(entry.Resource), entry.Name)
		if entry.Hostname != "" {
			label += " host=" + entry.Hostname
		}
		if entry.Port > 0 {
			label += fmt.Sprintf(" port=%d", entry.Port)
		}
		if entry.Protocol != "" {
			label += " protocol=" + entry.Protocol
		}
		if entry.GatewayClass != nil {
			label += " class=" + entry.GatewayClass.Identity.Name + " controller=" + entry.GatewayClass.Controller
		}
		if entry.TLS != nil {
			label += " TLS=" + entry.TLS.Mode
			for _, ref := range entry.TLS.CertificateRefs {
				label += " certificate=" + ref.Name
			}
		}
		label += conditionSummary(entry.Conditions)
		writeMermaidNode(&builder, id, label)
		entryIDs[resourceIdentityKey(entry.Resource)] = append(entryIDs[resourceIdentityKey(entry.Resource)], id)
		declaredParents[resourceIdentityKey(entry.Resource)] = true
	}

	routeIDs := make(map[string]string, len(analysis.Routes))
	for _, route := range analysis.Routes {
		id := mermaidID("route", resourceIdentityKey(route.Resource))
		routeIDs[resourceIdentityKey(route.Resource)] = id
		label := fmt.Sprintf("%s %s %s", route.Origin, route.Resource.Kind, identityName(route.Resource))
		if len(route.Hostnames) > 0 {
			label += " host=" + strings.Join(route.Hostnames, ",")
		}
		label += conditionSummary(route.Conditions)
		writeMermaidNode(&builder, id, label)
	}

	type mermaidEdge struct {
		from  string
		to    string
		label string
	}
	var edges []mermaidEdge
	for _, entry := range analysis.EntryPoints {
		edges = append(edges, mermaidEdge{from: hostID, to: entryPointID(entry), label: "matches"})
	}
	for _, route := range analysis.Routes {
		routeID := routeIDs[resourceIdentityKey(route.Resource)]
		if len(route.ParentRefs) == 0 {
			edges = append(edges, mermaidEdge{from: hostID, to: routeID, label: "matches"})
		}
		for _, parent := range route.ParentRefs {
			for _, entryID := range entryIDs[resourceIdentityKey(parent)] {
				edges = append(edges, mermaidEdge{from: entryID, to: routeID, label: "attaches"})
			}
			parentKey := resourceIdentityKey(parent)
			if len(entryIDs[parentKey]) == 0 {
				parentID := mermaidID("parent", parentKey)
				if !declaredParents[parentKey] {
					writeMermaidNode(&builder, parentID, fmt.Sprintf("parent reference %s %s", parent.Kind, identityName(parent)))
					declaredParents[parentKey] = true
				}
				edges = append(edges, mermaidEdge{from: parentID, to: routeID, label: "references"})
			}
		}
		for _, rule := range route.Rules {
			ruleID := fmt.Sprintf("%s_rule_%d", routeID, rule.Index)
			ruleLabel := fmt.Sprintf("%s rule %d %s", route.Origin, rule.Index, matchSummary(rule.Matches))
			if len(rule.Filters) > 0 {
				filterTypes := make([]string, 0, len(rule.Filters))
				for _, filter := range rule.Filters {
					filterTypes = append(filterTypes, filter.Type+detailsSummary(filter.Details))
				}
				ruleLabel += " filters=" + strings.Join(filterTypes, ",")
			}
			if len(rule.Actions) > 0 {
				ruleLabel += " actions=" + strings.TrimSpace(detailsSummary(rule.Actions))
			}
			writeMermaidNode(&builder, ruleID, ruleLabel)
			edges = append(edges, mermaidEdge{from: routeID, to: ruleID, label: "contains"})
			for index, destination := range rule.Destinations {
				destinationID := fmt.Sprintf("%s_backend_%d", ruleID, index+1)
				destinationLabel := destinationName(destination)
				if destination.Subset != "" {
					destinationLabel += " subset=" + destination.Subset
				}
				if destination.Weight != nil {
					destinationLabel += fmt.Sprintf(" weight=%d", *destination.Weight)
				}
				if destination.BackendGroup != "" {
					destinationLabel += " group=" + destination.BackendGroup
				}
				if destination.BackendKind != "" {
					destinationLabel += " kind=" + destination.BackendKind
				}
				if destination.External != nil {
					destinationLabel += " hosts=" + strings.Join(destination.External.Hosts, ",")
					destinationLabel += " location=" + destination.External.Location
					destinationLabel += " resolution=" + destination.External.Resolution
				}
				if destination.ReferenceGrant != nil {
					destinationLabel += fmt.Sprintf(" referenceGrantAllowed=%t", destination.ReferenceGrant.Allowed)
				}
				if destination.Service != nil {
					destinationLabel += " availability=" + string(destination.Service.Availability)
					destinationLabel += fmt.Sprintf(" endpoints=%d", len(destination.Service.Endpoints))
					for _, endpoint := range destination.Service.Endpoints {
						if endpoint.Pod != nil {
							destinationLabel += " pod=" + identityName(endpoint.Pod.Identity)
						}
						if endpoint.Ready != nil {
							destinationLabel += fmt.Sprintf(" endpointReady=%t", *endpoint.Ready)
						}
					}
				}
				writeMermaidNode(&builder, destinationID, destinationLabel)
				edges = append(edges, mermaidEdge{from: ruleID, to: destinationID, label: "routes to"})
			}
		}
	}

	for index, finding := range analysis.Findings {
		findingID := fmt.Sprintf("finding_%d", index+1)
		writeMermaidNode(&builder, findingID, strings.ToUpper(string(finding.Severity))+": "+finding.Message)
		if len(finding.Resources) == 0 {
			edges = append(edges, mermaidEdge{from: hostID, to: findingID, label: "finding"})
			continue
		}
		for _, resource := range finding.Resources {
			resourceKey := resourceIdentityKey(resource)
			if routeID, ok := routeIDs[resourceKey]; ok {
				edges = append(edges, mermaidEdge{from: findingID, to: routeID, label: "about"})
				continue
			}
			relatedEntries := entryIDs[resourceKey]
			for _, entryID := range relatedEntries {
				edges = append(edges, mermaidEdge{from: findingID, to: entryID, label: "about"})
			}
			if len(relatedEntries) == 0 {
				resourceID := mermaidID("resource", resourceKey)
				writeMermaidNode(&builder, resourceID, fmt.Sprintf("%s %s", resource.Kind, identityName(resource)))
				edges = append(edges, mermaidEdge{from: findingID, to: resourceID, label: "about"})
			}
		}
	}

	for _, edge := range edges {
		builder.WriteString("  ")
		builder.WriteString(edge.from)
		builder.WriteString(" -->|")
		builder.WriteString(escapeMermaid(edge.label))
		builder.WriteString("| ")
		builder.WriteString(edge.to)
		builder.WriteByte('\n')
	}
	if len(analysis.Findings) > 0 {
		builder.WriteString("  classDef finding fill:#fff3cd,stroke:#b58900,color:#333\n")
		for index := range analysis.Findings {
			fmt.Fprintf(&builder, "  class finding_%d finding\n", index+1)
		}
	}
	return builder.String()
}

func conditionSummary(conditions []model.Condition) string {
	var summary strings.Builder
	for _, condition := range conditions {
		fmt.Fprintf(&summary, " %s=%s", condition.Type, condition.Status)
		if condition.Reason != "" {
			summary.WriteString("(")
			summary.WriteString(condition.Reason)
			summary.WriteString(")")
		}
	}
	return summary.String()
}

func writeMermaidNode(builder *strings.Builder, id, label string) {
	builder.WriteString("  ")
	builder.WriteString(id)
	builder.WriteString("[\"")
	builder.WriteString(escapeMermaid(label))
	builder.WriteString("\"]\n")
}

func entryPointID(entry model.EntryPoint) string {
	return mermaidID("entry", resourceIdentityKey(entry.Resource)+"|"+entry.Name)
}

func mermaidID(prefix, value string) string {
	hash := sha256.Sum256([]byte(value))
	return prefix + "_" + hex.EncodeToString(hash[:6])
}

func resourceIdentityKey(identity model.ResourceIdentity) string {
	return strings.Join([]string{
		identity.Group, identity.Version, identity.Kind, identity.Namespace, identity.Name,
	}, "/")
}

func escapeMermaid(value string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"\"", "&quot;",
		"<", "&lt;",
		">", "&gt;",
		"\n", "&#10;",
	).Replace(value)
}
