package audit

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ninoamine/istio-route/internal/model"
)

type duplicateEntry struct {
	route model.ResourceIdentity
	rule  Rule
}

func duplicateFindings(result Result) []model.Finding {
	bySignature := make(map[string][]duplicateEntry)
	for _, route := range result.Routes {
		hosts := normalizedHostnames(route.Hostnames)
		for _, rule := range route.Rules {
			matches := canonicalMatches(rule.Matches)
			listeners := append([]string(nil), rule.ListenerNames...)
			if len(listeners) == 0 {
				listeners = []string{""}
			}
			sort.Strings(listeners)
			listeners = uniqueStrings(listeners)
			for _, host := range hosts {
				for _, listener := range listeners {
					key := strings.Join([]string{
						route.Identity.Kind, listener, host, strings.Join(matches, "||"),
					}, "\x00")
					bySignature[key] = append(bySignature[key], duplicateEntry{
						route: route.Identity, rule: rule,
					})
				}
			}
		}
	}

	var findings []model.Finding
	seen := make(map[string]struct{})
	for _, entries := range bySignature {
		if len(entries) < 2 {
			continue
		}
		sort.Slice(entries, func(i, j int) bool {
			left := identityKey(entries[i].route) + fmt.Sprintf("/%08d", entries[i].rule.Index)
			right := identityKey(entries[j].route) + fmt.Sprintf("/%08d", entries[j].rule.Index)
			return left < right
		})
		for i := 0; i < len(entries); i++ {
			for j := i + 1; j < len(entries); j++ {
				if entries[i].route == entries[j].route && entries[i].rule.Index == entries[j].rule.Index {
					continue
				}
				pair := identityKey(entries[i].route) + fmt.Sprintf("/%d|", entries[i].rule.Index) +
					identityKey(entries[j].route) + fmt.Sprintf("/%d", entries[j].rule.Index)
				if _, exists := seen[pair]; exists {
					continue
				}
				seen[pair] = struct{}{}
				resources := []model.ResourceIdentity{entries[i].route, entries[j].route}
				resources = normalizeResourceList(resources)
				findings = append(findings, model.Finding{
					Code: "duplicate-route-rule", Severity: model.FindingSeverityWarning,
					Message: fmt.Sprintf("%s %s/%s rule %d duplicates %s %s/%s rule %d for the same listener, hostname, and matches",
						entries[i].route.Kind, entries[i].route.Namespace, entries[i].route.Name, entries[i].rule.Index,
						entries[j].route.Kind, entries[j].route.Namespace, entries[j].route.Name, entries[j].rule.Index),
					Resources: resources, Category: string(CategoryDuplicated),
					SuggestedFix: "Remove or consolidate one of the identical route rules.",
				})
			}
		}
	}
	return findings
}

func normalizedHostnames(hostnames []string) []string {
	if len(hostnames) == 0 {
		return []string{""}
	}
	result := make([]string, len(hostnames))
	for i, hostname := range hostnames {
		result[i] = strings.TrimSuffix(strings.ToLower(hostname), ".")
	}
	sort.Strings(result)
	return uniqueStrings(result)
}

func canonicalMatches(matches []Match) []string {
	if len(matches) == 0 {
		return []string{""}
	}
	result := make([]string, len(matches))
	for i, match := range matches {
		headers := canonicalStringMatches(match.Headers, true)
		queries := canonicalStringMatches(match.QueryParams, false)
		pathType, pathValue := "PathPrefix", "/"
		if match.Path != nil {
			pathType, pathValue = match.Path.Type, match.Path.Value
			if pathType == "" {
				pathType = "PathPrefix"
			}
			if pathValue == "" {
				pathValue = "/"
			}
		}
		grpcService, grpcMethod := "", ""
		if match.GRPC != nil {
			grpcService, grpcMethod = match.GRPC.Service, match.GRPC.Method
		}
		result[i] = strings.Join([]string{
			pathType, pathValue, strings.Join(headers, "&"), strings.ToUpper(match.Method),
			strings.Join(queries, "&"), grpcService, grpcMethod,
		}, "\x01")
	}
	sort.Strings(result)
	return result
}

func canonicalStringMatches(matches []StringMatch, caseInsensitiveName bool) []string {
	result := make([]string, len(matches))
	for i, match := range matches {
		matchType := match.Type
		if matchType == "" {
			matchType = "Exact"
		}
		name := match.Name
		if caseInsensitiveName {
			name = strings.ToLower(name)
		}
		result[i] = name + "\x02" + matchType + "\x02" + match.Value
	}
	sort.Strings(result)
	return result
}

func sortFindings(findings []model.Finding) {
	for i := range findings {
		findings[i].Resources = normalizeResourceList(findings[i].Resources)
	}
	severityOrder := map[model.FindingSeverity]int{
		model.FindingSeverityError:   0,
		model.FindingSeverityWarning: 1,
		model.FindingSeverityInfo:    2,
	}
	categoryOrder := map[string]int{
		string(CategoryOrphaned):             0,
		string(CategoryDuplicated):           1,
		string(CategoryPrecedence):           2,
		string(CategoryDeficientDestination): 3,
	}
	sort.SliceStable(findings, func(i, j int) bool {
		left, right := findings[i], findings[j]
		if severityOrder[left.Severity] != severityOrder[right.Severity] {
			return severityOrder[left.Severity] < severityOrder[right.Severity]
		}
		if categoryOrder[left.Category] != categoryOrder[right.Category] {
			return categoryOrder[left.Category] < categoryOrder[right.Category]
		}
		leftKey, rightKey := findingKey(left), findingKey(right)
		return leftKey < rightKey
	})
}

func findingKey(finding model.Finding) string {
	resources := make([]string, len(finding.Resources))
	for i, resource := range finding.Resources {
		resources[i] = identityKey(resource)
	}
	sort.Strings(resources)
	return strings.Join(resources, "|") + "|" + finding.Code + "|" + finding.Message
}
