package render

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ninoamine/istio-route/internal/simulate"
	"sigs.k8s.io/yaml"
)

// RenderSimulation dispatches a simulation result to the renderer for format.
// Per-format renderers are stubs until their tasks land.
func RenderSimulation(result simulate.Result, format string, options Options) (string, error) {
	switch format {
	case FormatText:
		return simulateText(result, options)
	case FormatJSON:
		return simulateJSON(result)
	case FormatYAML:
		return simulateYAML(result)
	case FormatMermaid:
		return simulateMermaid(result)
	default:
		return "", fmt.Errorf("unsupported output format %q", format)
	}
}

var impactGroups = []struct {
	outcome simulate.Classification
	heading string
}{
	{simulate.ClassificationUnrouted, "UNROUTED"},
	{simulate.ClassificationStillCovered, "STILL COVERED"},
	{simulate.ClassificationUndetermined, "UNDETERMINED"},
	{simulate.ClassificationNoChange, "NO CHANGE"},
}

func simulateText(result simulate.Result, options Options) (string, error) {
	styles := newTextStyles(options.Color)
	var b strings.Builder
	writeWaypoint(&b, styles, result.Waypoint)
	for _, warning := range result.Warnings {
		fmt.Fprintf(&b, "%s %s\n", styles.warning("warning:"), warning)
	}
	if result.Scope.ServiceParentedExcluded > 0 {
		fmt.Fprintf(&b, "scope: %d Service-parented route(s) excluded (HTTPRoute parentRefs to the waypoint only)\n", result.Scope.ServiceParentedExcluded)
	}
	if result.Kind == simulate.ResultListing {
		writeListing(&b, styles, result.Routes)
		return b.String(), nil
	}
	if result.SelectedRoute != nil {
		fmt.Fprintf(&b, "%s %s/%s\n", styles.root("Removing:"), result.SelectedRoute.Namespace, result.SelectedRoute.Name)
	}
	b.WriteString("\n")
	for _, group := range impactGroups {
		writeGroup(&b, styles, group.heading, group.outcome, result.Entries)
	}
	writeRuleSummaries(&b, styles, result.RuleSummaries)
	t := result.Totals
	fmt.Fprintf(&b, "\n%s unrouted %d, still-covered %d, no-change %d, undetermined %d\n",
		styles.root("Totals:"), t.Unrouted, t.StillCovered, t.NoChange, t.Undetermined)
	return b.String(), nil
}

func writeWaypoint(b *strings.Builder, styles textStyles, waypoint simulate.Waypoint) {
	fmt.Fprintf(b, "%s %s/%s (class %s, validation %s)\n",
		styles.root("Waypoint:"), waypoint.Namespace, waypoint.Name, waypoint.Class, waypoint.Validation)
	if waypoint.ValidationReason != "" {
		fmt.Fprintf(b, "  %s\n", waypoint.ValidationReason)
	}
}

func writeListing(b *strings.Builder, styles textStyles, routes []simulate.RouteListing) {
	fmt.Fprintf(b, "%s\n", styles.root("Attached HTTPRoutes:"))
	if len(routes) == 0 {
		b.WriteString("  no eligible HTTPRoutes\n")
		return
	}
	for _, route := range routes {
		fmt.Fprintf(b, "  %s/%s", route.Route.Namespace, route.Route.Name)
		if len(route.Hostnames) > 0 {
			fmt.Fprintf(b, " hostnames=%s", strings.Join(route.Hostnames, ","))
		}
		b.WriteString("\n")
		for _, listener := range route.Listeners {
			fmt.Fprintf(b, "    listener %s: %s", listener.Listener, listener.State)
			if listener.Reason != "" {
				fmt.Fprintf(b, " (%s)", listener.Reason)
			}
			b.WriteString("\n")
		}
	}
}

func writeGroup(b *strings.Builder, styles textStyles, heading string, outcome simulate.Classification, entries []simulate.Entry) {
	var matched []simulate.Entry
	for _, entry := range entries {
		if entry.Outcome == outcome {
			matched = append(matched, entry)
		}
	}
	label := fmt.Sprintf("%s (%d)", heading, len(matched))
	if outcome == simulate.ClassificationUndetermined {
		fmt.Fprintf(b, "%s\n", styles.warning(label))
	} else {
		fmt.Fprintf(b, "%s\n", styles.root(label))
	}
	if len(matched) == 0 {
		b.WriteString("  none\n\n")
		return
	}
	for _, entry := range matched {
		fmt.Fprintf(b, "  - listener %s, rule %s/%s #%d: %s\n",
			entry.Listener, entry.Rule.Route.Namespace, entry.Rule.Route.Name, entry.Rule.Index, entry.Region.String())
		fmt.Fprintf(b, "      before: %s\n", providerText(entry.Before))
		if outcome == simulate.ClassificationStillCovered {
			fmt.Fprintf(b, "      covered by: %s\n", providerText(entry.After))
		}
		if entry.BackendChanged {
			fmt.Fprintf(b, "      backend changed: %s → %s\n", backendsText(entry.Before), backendsText(entry.After))
		}
		if entry.Provisional != nil {
			fmt.Fprintf(b, "      provisional: %s (%s)\n", entry.Provisional.Outcome, strings.Join(entry.Provisional.Reasons, "; "))
		}
		if len(entry.Reasons) > 0 {
			fmt.Fprintf(b, "      reason: %s\n", strings.Join(entry.Reasons, "; "))
		}
	}
	b.WriteString("\n")
}

func writeRuleSummaries(b *strings.Builder, styles textStyles, summaries []simulate.RuleSummary) {
	if len(summaries) == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s\n", styles.root("Rules:"))
	for _, summary := range summaries {
		fmt.Fprintf(b, "  %s/%s #%d:", summary.Rule.Route.Namespace, summary.Rule.Route.Name, summary.Rule.Index)
		for _, group := range impactGroups {
			if count := summary.Counts[group.outcome]; count > 0 {
				fmt.Fprintf(b, " %s=%d", group.outcome, count)
			}
		}
		if summary.Partial {
			b.WriteString(" (partial)")
		}
		b.WriteString("\n")
	}
}

func providerText(view simulate.ProviderView) string {
	switch view.State {
	case simulate.ProviderKnown:
		if view.Provider == nil {
			return "unknown"
		}
		p := view.Provider
		return fmt.Sprintf("%s/%s rule #%d → %s", p.Route.Namespace, p.Route.Name, p.RuleIndex, backendList(p.Backends))
	case simulate.ProviderNone:
		return "no route"
	default:
		return "unknown"
	}
}

func backendsText(view simulate.ProviderView) string {
	if view.Provider == nil {
		return "none"
	}
	return backendList(view.Provider.Backends)
}

func backendList(backends []simulate.BackendRef) string {
	if len(backends) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(backends))
	for _, backend := range backends {
		ref := backend.Namespace + "/" + backend.Name
		if backend.Port != nil {
			ref = fmt.Sprintf("%s:%d", ref, *backend.Port)
		}
		parts = append(parts, ref)
	}
	return strings.Join(parts, ", ")
}

func simulateJSON(result simulate.Result) (string, error) {
	contents, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode simulation JSON report: %w", err)
	}
	return string(contents) + "\n", nil
}

func simulateYAML(result simulate.Result) (string, error) {
	contents, err := yaml.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("encode simulation YAML report: %w", err)
	}
	return string(contents), nil
}

func simulateMermaid(result simulate.Result) (string, error) {
	var b strings.Builder
	b.WriteString("flowchart LR\n")
	waypoint := result.Waypoint
	waypointID := mermaidID("waypoint", waypoint.Namespace+"/"+waypoint.Name)
	writeMermaidNode(&b, waypointID, fmt.Sprintf("Waypoint %s/%s (validation %s)",
		waypoint.Namespace, waypoint.Name, waypoint.Validation))
	if result.Kind == simulate.ResultListing {
		writeMermaidListing(&b, waypointID, result.Routes)
		return b.String(), nil
	}
	removedID := waypointID
	if result.SelectedRoute != nil {
		route := *result.SelectedRoute
		removedID = mermaidID("route", route.Namespace+"/"+route.Name)
		writeMermaidNode(&b, removedID, fmt.Sprintf("Removing HTTPRoute %s/%s", route.Namespace, route.Name))
		fmt.Fprintf(&b, "  %s -.->|attached| %s\n", waypointID, removedID)
	}
	nodes := map[string]bool{}
	for i, entry := range result.Entries {
		writeMermaidEntry(&b, removedID, i, entry, nodes)
	}
	return b.String(), nil
}

func writeMermaidEntry(b *strings.Builder, fromID string, index int, entry simulate.Entry, nodes map[string]bool) {
	rule := entry.Rule
	entryID := mermaidID("entry", fmt.Sprintf("%d/%s/%s/%s/%d",
		index, entry.Listener, rule.Route.Namespace, rule.Route.Name, rule.Index))
	label := fmt.Sprintf("%s: listener %s, rule %s/%s #%d, %s", entry.Outcome,
		entry.Listener, rule.Route.Namespace, rule.Route.Name, rule.Index, entry.Region.String())
	if entry.Provisional != nil {
		label += fmt.Sprintf(" (provisional: %s)", entry.Provisional.Outcome)
	}
	writeMermaidNode(b, entryID, label)

	arrow, edgeLabel := "-.->", string(entry.Outcome)
	if entry.Outcome == simulate.ClassificationUnrouted || entry.Outcome == simulate.ClassificationStillCovered {
		arrow = "-->"
	}
	if entry.Outcome == simulate.ClassificationStillCovered && entry.BackendChanged {
		edgeLabel = "backend changed"
	}
	fmt.Fprintf(b, "  %s %s|%s| %s\n", fromID, arrow, escapeMermaid(edgeLabel), entryID)

	if entry.Outcome != simulate.ClassificationStillCovered || entry.After.Provider == nil {
		return
	}
	provider := entry.After.Provider
	providerID := mermaidID("provider", fmt.Sprintf("%s/%s/%d",
		provider.Route.Namespace, provider.Route.Name, provider.RuleIndex))
	if !nodes[providerID] {
		nodes[providerID] = true
		writeMermaidNode(b, providerID, fmt.Sprintf("Covered by %s/%s rule #%d",
			provider.Route.Namespace, provider.Route.Name, provider.RuleIndex))
		for _, backend := range provider.Backends {
			serviceKey := backend.Namespace + "/" + backend.Name
			label := "Service " + serviceKey
			if backend.Port != nil {
				serviceKey += fmt.Sprintf(":%d", *backend.Port)
				label += fmt.Sprintf(":%d", *backend.Port)
			}
			serviceID := mermaidID("service", serviceKey)
			if !nodes[serviceID] {
				nodes[serviceID] = true
				writeMermaidNode(b, serviceID, label)
			}
			fmt.Fprintf(b, "  %s --> %s\n", providerID, serviceID)
		}
	}
	fmt.Fprintf(b, "  %s -->|covered by| %s\n", entryID, providerID)
}

func writeMermaidListing(b *strings.Builder, waypointID string, routes []simulate.RouteListing) {
	if len(routes) == 0 {
		noneID := mermaidID("listing", "none")
		writeMermaidNode(b, noneID, "No eligible HTTPRoutes")
		fmt.Fprintf(b, "  %s -.-> %s\n", waypointID, noneID)
		return
	}
	for _, route := range routes {
		routeID := mermaidID("route", route.Route.Namespace+"/"+route.Route.Name)
		label := fmt.Sprintf("HTTPRoute %s/%s", route.Route.Namespace, route.Route.Name)
		if len(route.Hostnames) > 0 {
			label += " hostnames=" + strings.Join(route.Hostnames, ",")
		}
		writeMermaidNode(b, routeID, label)
		states := make([]string, 0, len(route.Listeners))
		for _, listener := range route.Listeners {
			states = append(states, fmt.Sprintf("%s: %s", listener.Listener, listener.State))
		}
		fmt.Fprintf(b, "  %s -.->|%s| %s\n", waypointID, escapeMermaid(strings.Join(states, "; ")), routeID)
	}
}
