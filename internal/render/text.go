package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ninoamine/istio-route/internal/model"
)

type treeNode struct {
	label    string
	children []treeNode
}

func Text(analysis model.Analysis) string {
	return TextWithOptions(analysis, Options{})
}

func TextWithOptions(analysis model.Analysis, options Options) string {
	styles := newTextStyles(options.Color)
	root := treeNode{label: analysis.Hostname}
	attached := make(map[int][]model.Route)
	unattached := make([]model.Route, 0)
	for _, route := range analysis.Routes {
		parentIndex := -1
		for i, entry := range analysis.EntryPoints {
			for _, parent := range route.ParentRefs {
				if sameIdentity(parent, entry.Resource) {
					parentIndex = i
					break
				}
			}
			if parentIndex >= 0 {
				break
			}
		}
		if parentIndex < 0 {
			unattached = append(unattached, route)
		} else {
			attached[parentIndex] = append(attached[parentIndex], route)
		}
	}

	for i, entry := range analysis.EntryPoints {
		node := treeNode{label: entryLabel(entry, styles)}
		routes := attached[i]
		for _, route := range routes {
			node.children = append(node.children, routeNode(route, styles))
		}
		root.children = append(root.children, node)
	}
	for _, route := range unattached {
		root.children = append(root.children, routeNode(route, styles))
	}

	var builder strings.Builder
	builder.WriteString(styles.root(root.label))
	builder.WriteByte('\n')
	for i, child := range root.children {
		writeNode(&builder, child, "", i == len(root.children)-1, styles)
	}
	for _, finding := range analysis.Findings {
		if finding.Severity != model.FindingSeverityWarning {
			continue
		}
		builder.WriteString(styles.warning("⚠ " + finding.Message))
		builder.WriteByte('\n')
	}
	return builder.String()
}

func entryLabel(entry model.EntryPoint, styles textStyles) string {
	label := fmt.Sprintf("[Istio] Gateway %s", identityName(entry.Resource))
	if entry.Origin == model.OriginGatewayAPI {
		label = fmt.Sprintf("[Gateway API] Gateway %s", identityName(entry.Resource))
		if entry.GatewayClass != nil {
			label += fmt.Sprintf(" (class: %s, controller: %s)", entry.GatewayClass.Identity.Name, entry.GatewayClass.Controller)
		}
		label += fmt.Sprintf(" listener %s", entry.Name)
	} else {
		label += fmt.Sprintf(" server %s", entry.Name)
	}
	if entry.Port > 0 {
		label += fmt.Sprintf(" :%d", entry.Port)
	}
	if entry.Protocol != "" {
		label += " " + entry.Protocol
	}
	if entry.TLS != nil {
		label += " (TLS " + entry.TLS.Mode
		if len(entry.TLS.CertificateRefs) > 0 {
			secrets := make([]string, 0, len(entry.TLS.CertificateRefs))
			for _, ref := range entry.TLS.CertificateRefs {
				secrets = append(secrets, ref.Name)
			}
			label += ", secret " + strings.Join(secrets, ", ")
		}
		label += ")"
	}
	conditionCount := 0
	for _, condition := range entry.Conditions {
		if condition.Type == "Programmed" {
			if conditionCount == 0 {
				label += "  "
			} else {
				label += " "
			}
			label += styles.condition(condition)
			conditionCount++
		}
	}
	return label
}

func routeNode(route model.Route, styles textStyles) treeNode {
	label := fmt.Sprintf("[%s] %s %s", route.Origin, route.Resource.Kind, identityName(route.Resource))
	conditionCount := 0
	for _, condition := range route.Conditions {
		if condition.Type == "Accepted" || condition.Type == "ResolvedRefs" {
			if conditionCount == 0 {
				label += "  "
			} else {
				label += " "
			}
			label += styles.condition(condition)
			conditionCount++
		}
	}
	node := treeNode{label: label}
	for _, rule := range route.Rules {
		node.children = append(node.children, ruleNode(route.Origin, rule))
	}
	return node
}

func ruleNode(origin model.Origin, rule model.Rule) treeNode {
	label := fmt.Sprintf("[%s] %d. %s", origin, rule.Index, matchSummary(rule.Matches))
	node := treeNode{label: label}
	for _, filter := range rule.Filters {
		node.children = append(node.children, treeNode{
			label: fmt.Sprintf("[%s] filter: %s%s", origin, filter.Type, detailsSummary(filter.Details)),
		})
	}
	for _, destination := range rule.Destinations {
		node.children = append(node.children, destinationNode(origin, destination, rule.Destinations))
	}
	return node
}

func destinationNode(origin model.Origin, destination model.Destination, destinations []model.Destination) treeNode {
	label := fmt.Sprintf("[%s] → %s", origin, destinationName(destination))
	if destination.Subset != "" {
		label += " subset " + destination.Subset
	}
	if destination.Weight != nil {
		label += fmt.Sprintf(" (%s%%)", effectiveWeight(origin, *destination.Weight, destinations))
	}
	node := treeNode{label: label}
	if destination.ReferenceGrant != nil {
		state := "not allowed"
		if destination.ReferenceGrant.Allowed {
			state = "allowed"
		}
		node.children = append(node.children, treeNode{label: fmt.Sprintf("[%s] ReferenceGrant: %s", origin, state)})
	}
	if destination.External != nil {
		details := []string{
			"hosts=" + strings.Join(destination.External.Hosts, ","),
			"location=" + destination.External.Location,
			"resolution=" + destination.External.Resolution,
		}
		node.children = append(node.children, treeNode{
			label: fmt.Sprintf("[%s] external destination: %s", origin, strings.Join(details, " ")),
		})
	}
	if destination.Service != nil {
		service := destination.Service
		status := string(service.Availability)
		node.children = append(node.children, treeNode{
			label: fmt.Sprintf("[%s] Service %s (%s)", origin, identityName(service.Identity), status),
		})
		for _, endpoint := range service.Endpoints {
			endpointStatus := "status unavailable"
			if endpoint.Ready != nil {
				if *endpoint.Ready {
					endpointStatus = "ready"
				} else {
					endpointStatus = "not ready"
				}
			}
			endpointLabel := fmt.Sprintf("[%s] Endpoint %s (%s)", origin, strings.Join(endpoint.Addresses, ","), endpointStatus)
			node.children = append(node.children, treeNode{label: endpointLabel})
			if endpoint.Pod != nil {
				podStatus := string(endpoint.Pod.Availability)
				if endpoint.Pod.Ready != nil {
					podStatus = "not ready"
					if *endpoint.Pod.Ready {
						podStatus = "ready"
					}
				}
				node.children = append(node.children, treeNode{
					label: fmt.Sprintf("[%s] Pod %s (%s, phase %s)", origin, identityName(endpoint.Pod.Identity), podStatus, endpoint.Pod.Phase),
				})
			}
		}
	}
	return node
}

func matchSummary(matches []model.Match) string {
	if len(matches) == 0 {
		return "(default)"
	}
	summaries := make([]string, 0, len(matches))
	for _, match := range matches {
		parts := make([]string, 0, 4)
		if match.PathType == "default" || (match.Path == "" && len(match.Headers) == 0 && len(match.QueryParams) == 0 && match.Method == "") {
			parts = append(parts, "(default)")
		} else if match.Path != "" {
			pathType := match.PathType
			prefix := ""
			if strings.HasPrefix(pathType, "Path") {
				pathType = strings.TrimPrefix(pathType, "Path")
				prefix = "path "
			}
			pathType = strings.ToLower(pathType)
			parts = append(parts, prefix+pathType+" "+match.Path)
		}
		appendMapMatches(&parts, "header", match.Headers)
		appendMapMatches(&parts, "query", match.QueryParams)
		if match.Method != "" {
			parts = append(parts, "method "+match.Method)
		}
		summaries = append(summaries, strings.Join(parts, " "))
	}
	return strings.Join(summaries, " or ")
}

func appendMapMatches(parts *[]string, label string, values map[string]string) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		*parts = append(*parts, label+" "+key+"="+values[key])
	}
}

func detailsSummary(details map[string]string) string {
	if len(details) == 0 {
		return ""
	}
	keys := make([]string, 0, len(details))
	for key := range details {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values := make([]string, 0, len(keys))
	for _, key := range keys {
		values = append(values, key+"="+details[key])
	}
	return " " + strings.Join(values, " ")
}

func destinationName(destination model.Destination) string {
	if destination.Kind == model.DestinationKindExternal {
		return destination.Name
	}
	name := destination.Name
	if destination.Namespace != "" {
		name += "." + destination.Namespace
	}
	if destination.Port != nil {
		name += ":" + fmt.Sprint(*destination.Port)
	}
	return name
}

func effectiveWeight(origin model.Origin, weight int32, destinations []model.Destination) string {
	if origin == model.OriginIstio {
		return fmt.Sprint(weight)
	}
	total := int32(0)
	for _, destination := range destinations {
		if destination.Weight != nil {
			total += *destination.Weight
		}
	}
	if total <= 0 {
		return "0"
	}
	return fmt.Sprint(weight * 100 / total)
}

func plainConditionLabel(condition model.Condition) string {
	switch condition.Status {
	case model.ConditionStatusTrue:
		return "✔ " + condition.Type
	case model.ConditionStatusFalse:
		return "✘ " + condition.Type
	default:
		return "? " + condition.Type
	}
}

func identityName(identity model.ResourceIdentity) string {
	if identity.Namespace == "" {
		return identity.Name
	}
	return identity.Namespace + "/" + identity.Name
}

func sameIdentity(first, second model.ResourceIdentity) bool {
	return first.Group == second.Group &&
		first.Kind == second.Kind &&
		first.Namespace == second.Namespace &&
		first.Name == second.Name
}

func writeNode(builder *strings.Builder, node treeNode, prefix string, last bool, styles textStyles) {
	builder.WriteString(prefix)
	if last {
		builder.WriteString("└─ ")
	} else {
		builder.WriteString("├─ ")
	}
	builder.WriteString(styles.node(node.label))
	builder.WriteByte('\n')
	childPrefix := prefix + "│  "
	if last {
		childPrefix = prefix + "   "
	}
	for i, child := range node.children {
		writeNode(builder, child, childPrefix, i == len(node.children)-1, styles)
	}
}
