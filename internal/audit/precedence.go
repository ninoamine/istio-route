package audit

import (
	"fmt"
	"strings"
	"time"

	"github.com/ninoamine/istio-route/internal/model"
	"github.com/ninoamine/istio-route/internal/resolve"
)

type ruleRef struct {
	routeIndex int
	ruleIndex  int
	route      Route
	rule       *Rule
}

func applyPrecedence(result *Result) {
	refs := make([]ruleRef, 0)
	for routeIndex, route := range result.Routes {
		for ruleIndex := range route.Rules {
			refs = append(refs, ruleRef{
				routeIndex: routeIndex,
				ruleIndex:  ruleIndex,
			})
		}
	}
	for i := range refs {
		ref := &refs[i]
		ref.route = result.Routes[ref.routeIndex]
		ref.rule = &result.Routes[ref.routeIndex].Rules[ref.ruleIndex]
		ref.rule.Precedence = PrecedenceWinner
	}
	for i := 0; i < len(refs); i++ {
		for j := i + 1; j < len(refs); j++ {
			left, right := refs[i], refs[j]
			if !rulesShareTraffic(left, right) {
				continue
			}
			if !rulesOverlap(left, right) {
				continue
			}
			if rulesEquivalent(left, right) {
				continue
			}
			preference := compareRulePrecedence(left, right)
			if preference == 0 {
				continue
			}
			if preference > 0 {
				if rulesCover(left, right) {
					right.rule.Precedence = PrecedenceShadowed
				} else {
					right.rule.Precedence = PrecedencePartialOverlap
				}
				continue
			}
			if rulesCover(right, left) {
				left.rule.Precedence = PrecedenceShadowed
			} else {
				left.rule.Precedence = PrecedencePartialOverlap
			}
		}
	}
}

func precedenceFindings(result Result) []model.Finding {
	var findings []model.Finding
	for _, route := range result.Routes {
		for _, rule := range route.Rules {
			switch rule.Precedence {
			case PrecedenceShadowed:
				findings = append(findings, model.Finding{
					Code:         "shadowed-route-rule",
					Severity:     model.FindingSeverityWarning,
					Message:      fmt.Sprintf("%s %s/%s rule %d is shadowed by a higher-precedence rule", route.Identity.Kind, route.Identity.Namespace, route.Identity.Name, rule.Index),
					Resources:    []model.ResourceIdentity{route.Identity},
					Category:     string(CategoryPrecedence),
					SuggestedFix: "Reorder or narrow the higher-precedence route so the lower-priority rule still receives applicable traffic.",
				})
			case PrecedencePartialOverlap:
				findings = append(findings, model.Finding{
					Code:         "partial-overlap-route-rule",
					Severity:     model.FindingSeverityInfo,
					Message:      fmt.Sprintf("%s %s/%s rule %d partially overlaps a higher-precedence rule and remains eligible for unmatched traffic", route.Identity.Kind, route.Identity.Namespace, route.Identity.Name, rule.Index),
					Resources:    []model.ResourceIdentity{route.Identity},
					Category:     string(CategoryPrecedence),
					SuggestedFix: "Keep the lower-priority rule only if it intentionally handles traffic outside the overlapping match set.",
				})
			case PrecedenceWinner:
				if hasOverlappingRule(result, route, rule) {
					findings = append(findings, model.Finding{
						Code:         "winning-route-rule",
						Severity:     model.FindingSeverityInfo,
						Message:      fmt.Sprintf("%s %s/%s rule %d wins over overlapping lower-precedence rules", route.Identity.Kind, route.Identity.Namespace, route.Identity.Name, rule.Index),
						Resources:    []model.ResourceIdentity{route.Identity},
						Category:     string(CategoryPrecedence),
						SuggestedFix: "Review overlapping rules to confirm the winning precedence is intentional.",
					})
				}
			}
		}
	}
	return findings
}

func hasOverlappingRule(result Result, route Route, rule Rule) bool {
	for _, otherRoute := range result.Routes {
		if otherRoute.Identity == route.Identity {
			continue
		}
		for _, otherRule := range otherRoute.Rules {
			left := ruleRef{route: route, rule: &rule}
			right := ruleRef{route: otherRoute, rule: &otherRule}
			if !rulesShareTraffic(left, right) || !rulesOverlap(left, right) {
				continue
			}
			if rulesEquivalent(left, right) {
				continue
			}
			if compareRulePrecedence(left, right) < 0 {
				return true
			}
		}
	}
	return false
}

func compareRulePrecedence(left, right ruleRef) int {
	leftSpecificity := ruleSpecificity(left.rule)
	rightSpecificity := ruleSpecificity(right.rule)
	if leftSpecificity != rightSpecificity {
		if leftSpecificity > rightSpecificity {
			return 1
		}
		return -1
	}
	leftTime, rightTime := time.Time{}, time.Time{}
	if left.rule.CreatedAt != nil {
		leftTime = left.rule.CreatedAt.Time
	}
	if right.rule.CreatedAt != nil {
		rightTime = right.rule.CreatedAt.Time
	}
	if !leftTime.Equal(rightTime) {
		if leftTime.Before(rightTime) {
			return 1
		}
		return -1
	}
	leftKey := identityKey(left.route.Identity)
	rightKey := identityKey(right.route.Identity)
	if leftKey != rightKey {
		if leftKey < rightKey {
			return 1
		}
		return -1
	}
	if left.rule.Index != right.rule.Index {
		if left.rule.Index < right.rule.Index {
			return 1
		}
		return -1
	}
	return 0
}

func ruleSpecificity(rule *Rule) int {
	if rule == nil {
		return 0
	}
	score := 0
	for _, match := range rule.Matches {
		score += matchSpecificity(match)
	}
	if len(rule.Matches) == 0 {
		score = 1
	}
	return score
}

func matchSpecificity(match Match) int {
	score := 0
	if match.Path != nil {
		switch match.Path.Type {
		case "Exact":
			score += 60
		case "PathPrefix":
			score += 40
		case "RegularExpression":
			score += 20
		default:
			score += 10
		}
		if match.Path.Value != "" && match.Path.Value != "/" {
			score += 10
		}
	}
	if match.Method != "" {
		score += 20
	}
	score += len(match.Headers) * 12
	score += len(match.QueryParams) * 12
	if match.GRPC != nil {
		if match.GRPC.Service != "" {
			score += 18
		}
		if match.GRPC.Method != "" {
			score += 18
		}
	}
	return score
}

func rulesShareTraffic(left, right ruleRef) bool {
	if !listenerNamesOverlap(left.rule.ListenerNames, right.rule.ListenerNames) {
		return false
	}
	if !hostnamesIntersect(left.route.Hostnames, right.route.Hostnames) {
		return false
	}
	return true
}

func listenerNamesOverlap(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return true
	}
	seen := map[string]struct{}{}
	for _, value := range left {
		seen[value] = struct{}{}
	}
	for _, value := range right {
		if _, ok := seen[value]; ok {
			return true
		}
	}
	return false
}

func hostnamesIntersect(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return true
	}
	for _, leftHost := range left {
		for _, rightHost := range right {
			if resolve.HostnamesIntersect(leftHost, rightHost) {
				return true
			}
		}
	}
	return false
}

func rulesOverlap(left, right ruleRef) bool {
	if len(left.rule.Matches) == 0 || len(right.rule.Matches) == 0 {
		return true
	}
	for _, leftMatch := range left.rule.Matches {
		for _, rightMatch := range right.rule.Matches {
			if matchesOverlap(leftMatch, rightMatch) {
				return true
			}
		}
	}
	return false
}

func rulesEquivalent(left, right ruleRef) bool {
	if left.route.Identity == right.route.Identity && left.rule.Index == right.rule.Index {
		return true
	}
	if !sameListenerNames(left.rule.ListenerNames, right.rule.ListenerNames) {
		return false
	}
	if !sameHostnames(left.route.Hostnames, right.route.Hostnames) {
		return false
	}
	leftTime, rightTime := time.Time{}, time.Time{}
	if left.rule.CreatedAt != nil {
		leftTime = left.rule.CreatedAt.Time
	}
	if right.rule.CreatedAt != nil {
		rightTime = right.rule.CreatedAt.Time
	}
	if !leftTime.Equal(rightTime) {
		return false
	}
	return strings.Join(canonicalMatches(left.rule.Matches), "||") == strings.Join(canonicalMatches(right.rule.Matches), "||")
}

func sameListenerNames(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return true
	}
	leftSet := make(map[string]struct{}, len(left))
	for _, name := range left {
		leftSet[name] = struct{}{}
	}
	for _, name := range right {
		if _, ok := leftSet[name]; ok {
			return true
		}
	}
	return false
}

func sameHostnames(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return true
	}
	leftSet := make(map[string]struct{}, len(left))
	for _, host := range normalizedHostnames(left) {
		leftSet[host] = struct{}{}
	}
	for _, host := range normalizedHostnames(right) {
		if _, ok := leftSet[host]; ok {
			return true
		}
	}
	return false
}

func matchesOverlap(left, right Match) bool {
	if !pathValuesOverlap(left.Path, right.Path) {
		return false
	}
	if left.Method != "" && right.Method != "" && left.Method != right.Method {
		return false
	}
	for _, leftHeader := range left.Headers {
		for _, rightHeader := range right.Headers {
			if leftHeader.Name == rightHeader.Name && leftHeader.Value != rightHeader.Value && leftHeader.Type == rightHeader.Type {
				return false
			}
		}
	}
	for _, leftParam := range left.QueryParams {
		for _, rightParam := range right.QueryParams {
			if leftParam.Name == rightParam.Name && leftParam.Value != rightParam.Value && leftParam.Type == rightParam.Type {
				return false
			}
		}
	}
	if left.GRPC != nil && right.GRPC != nil {
		if left.GRPC.Service != "" && right.GRPC.Service != "" && left.GRPC.Service != right.GRPC.Service {
			return false
		}
		if left.GRPC.Method != "" && right.GRPC.Method != "" && left.GRPC.Method != right.GRPC.Method {
			return false
		}
	}
	return true
}

func pathValuesOverlap(left, right *PathMatch) bool {
	if left == nil || right == nil {
		return true
	}
	if left.Value == right.Value {
		return true
	}
	if left.Type == "PathPrefix" && (right.Type == "PathPrefix" || right.Type == "Exact") {
		return strings.HasPrefix(right.Value, left.Value) || left.Value == "/" || right.Value == "/"
	}
	if right.Type == "PathPrefix" && (left.Type == "PathPrefix" || left.Type == "Exact") {
		return strings.HasPrefix(left.Value, right.Value) || left.Value == "/" || right.Value == "/"
	}
	if left.Type == "Exact" && right.Type == "Exact" {
		return false
	}
	return true
}

func rulesCover(higher, lower ruleRef) bool {
	if len(lower.rule.Matches) == 0 {
		return true
	}
	if len(higher.rule.Matches) == 0 {
		return false
	}
	for _, lowerMatch := range lower.rule.Matches {
		covered := false
		for _, higherMatch := range higher.rule.Matches {
			if matchCovers(higherMatch, lowerMatch) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

func matchCovers(higher, lower Match) bool {
	if !pathValuesCovered(higher.Path, lower.Path) {
		return false
	}
	if higher.Method != "" && lower.Method != "" && higher.Method != lower.Method {
		return false
	}
	for _, lowerHeader := range lower.Headers {
		covered := false
		for _, higherHeader := range higher.Headers {
			if higherHeader.Name == lowerHeader.Name && (higherHeader.Value == lowerHeader.Value || higherHeader.Value == "" || higherHeader.Value == "*") {
				covered = true
				break
			}
		}
		if !covered && lowerHeader.Name != "" {
			return false
		}
	}
	for _, lowerParam := range lower.QueryParams {
		covered := false
		for _, higherParam := range higher.QueryParams {
			if higherParam.Name == lowerParam.Name && (higherParam.Value == lowerParam.Value || higherParam.Value == "" || higherParam.Value == "*") {
				covered = true
				break
			}
		}
		if !covered && lowerParam.Name != "" {
			return false
		}
	}
	if lower.GRPC != nil && higher.GRPC != nil {
		if lower.GRPC.Service != "" && higher.GRPC.Service != "" && lower.GRPC.Service != higher.GRPC.Service {
			return false
		}
		if lower.GRPC.Method != "" && higher.GRPC.Method != "" && lower.GRPC.Method != higher.GRPC.Method {
			return false
		}
	}
	return true
}

func pathValuesCovered(higher, lower *PathMatch) bool {
	if lower == nil {
		return true
	}
	if higher == nil {
		return false
	}
	if higher.Value == lower.Value {
		return true
	}
	if higher.Type == "PathPrefix" {
		return strings.HasPrefix(lower.Value, higher.Value) || higher.Value == "/" || lower.Value == "/"
	}
	if higher.Type == "Exact" && lower.Type == "PathPrefix" {
		return higher.Value == lower.Value
	}
	return false
}
