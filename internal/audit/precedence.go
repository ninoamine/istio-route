package audit

import (
	"cmp"
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
	}
	// Each slot holds the first decisive comparison for that rule, so findings can name the competitor and criterion.
	overlapped := make([]*precedenceDecision, len(refs))
	covered := make([]*precedenceDecision, len(refs))
	beats := make([]*precedenceDecision, len(refs))
	undecided := make([]string, len(refs))
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
			outcome, criterion := compareRulePrecedence(left, right)
			winnerIndex, loserIndex := i, j
			switch outcome {
			case outcomeLeftWins:
			case outcomeRightWins:
				winnerIndex, loserIndex = j, i
			default:
				// No winner is invented: neither rule gets a shadowed or partial-overlap state from this pair.
				if undecided[i] == "" {
					undecided[i] = ruleLabel(right.route.Identity, right.rule.Index)
				}
				if undecided[j] == "" {
					undecided[j] = ruleLabel(left.route.Identity, left.rule.Index)
				}
				continue
			}
			winner, loser := refs[winnerIndex], refs[loserIndex]
			if beats[winnerIndex] == nil {
				beats[winnerIndex] = newPrecedenceDecision(loser, criterion)
			}
			if overlapped[loserIndex] == nil {
				overlapped[loserIndex] = newPrecedenceDecision(winner, criterion)
			}
			if covered[loserIndex] == nil && rulesCover(winner, loser) {
				covered[loserIndex] = newPrecedenceDecision(winner, criterion)
			}
		}
	}
	for i, ref := range refs {
		switch {
		case covered[i] != nil:
			ref.rule.Precedence = PrecedenceShadowed
			ref.rule.decision = *covered[i]
		case overlapped[i] != nil:
			ref.rule.Precedence = PrecedencePartialOverlap
			ref.rule.decision = *overlapped[i]
		default:
			ref.rule.Precedence = PrecedenceWinner
			if beats[i] != nil {
				ref.rule.decision = *beats[i]
			}
		}
		ref.rule.decision.undecidedWith = undecided[i]
	}
}

// precedenceDecision records the competing rule and the criterion that decided the outcome.
type precedenceDecision struct {
	competitor string
	criterion  string
	// undecidedWith names a rule that overlaps this one with equal precedence on every criterion.
	undecidedWith string
}

func newPrecedenceDecision(other ruleRef, criterion string) *precedenceDecision {
	return &precedenceDecision{
		competitor: ruleLabel(other.route.Identity, other.rule.Index),
		criterion:  criterion,
	}
}

func ruleLabel(identity model.ResourceIdentity, index int) string {
	return fmt.Sprintf("%s %s/%s rule %d", identity.Kind, identity.Namespace, identity.Name, index)
}

func precedenceFindings(result Result) []model.Finding {
	var findings []model.Finding
	for _, route := range result.Routes {
		for _, rule := range route.Rules {
			self := ruleLabel(route.Identity, rule.Index)
			if rule.decision.undecidedWith != "" {
				findings = append(findings, model.Finding{
					Code:         "undecided-route-rule-overlap",
					Severity:     model.FindingSeverityInfo,
					Message:      fmt.Sprintf("%s overlaps %s with equal precedence on every criterion, so no winner can be determined", self, rule.decision.undecidedWith),
					Resources:    []model.ResourceIdentity{route.Identity},
					Category:     string(CategoryPrecedence),
					SuggestedFix: "Differentiate the overlapping rules (path, method, headers or query params) so one deterministically takes precedence.",
				})
			}
			switch rule.Precedence {
			case PrecedenceShadowed:
				findings = append(findings, model.Finding{
					Code:         "shadowed-route-rule",
					Severity:     model.FindingSeverityWarning,
					Message:      fmt.Sprintf("%s is shadowed by %s (decided by %s)", self, rule.decision.competitor, rule.decision.criterion),
					Resources:    []model.ResourceIdentity{route.Identity},
					Category:     string(CategoryPrecedence),
					SuggestedFix: "Reorder or narrow the higher-precedence route so the lower-priority rule still receives applicable traffic.",
				})
			case PrecedencePartialOverlap:
				findings = append(findings, model.Finding{
					Code:         "partial-overlap-route-rule",
					Severity:     model.FindingSeverityInfo,
					Message:      fmt.Sprintf("%s partially overlaps %s (decided by %s) and remains eligible for unmatched traffic", self, rule.decision.competitor, rule.decision.criterion),
					Resources:    []model.ResourceIdentity{route.Identity},
					Category:     string(CategoryPrecedence),
					SuggestedFix: "Keep the lower-priority rule only if it intentionally handles traffic outside the overlapping match set.",
				})
			case PrecedenceWinner:
				if rule.decision.competitor == "" {
					continue
				}
				findings = append(findings, model.Finding{
					Code:         "winning-route-rule",
					Severity:     model.FindingSeverityInfo,
					Message:      fmt.Sprintf("%s wins over overlapping lower-precedence %s (decided by %s)", self, rule.decision.competitor, rule.decision.criterion),
					Resources:    []model.ResourceIdentity{route.Identity},
					Category:     string(CategoryPrecedence),
					SuggestedFix: "Review overlapping rules to confirm the winning precedence is intentional.",
				})
			}
		}
	}
	return findings
}

type precedenceOutcome int

const (
	outcomeUndecided precedenceOutcome = iota
	outcomeLeftWins
	outcomeRightWins
)

const (
	criterionPathType     = "path type"
	criterionPrefixLength = "prefix length"
	criterionMethod       = "method"
	criterionHeaders      = "headers"
	criterionQueryParams  = "query params"
	criterionCreatedAt    = "creation timestamp"
	criterionNamespace    = "namespace/name"
	criterionRuleIndex    = "rule index"
	criterionUndecided    = "undecided"
)

const (
	pathRankNone = iota
	pathRankRegularExpression
	pathRankPathPrefix
	pathRankExact
)

// matchKey is compared lexicographically; larger wins. prefixSegments is only
// set for PathPrefix matches, so it is significant only when both sides are
// PathPrefix (equal path ranks imply equal path types).
type matchKey struct {
	pathRank       int
	prefixSegments int
	hasMethod      bool
	headerCount    int
	queryCount     int
}

// newMatchKey ranks a match. For HTTP matches (httpPath), a missing path is
// the Gateway API default PathPrefix "/"; gRPC matches have no path.
func newMatchKey(match Match, httpPath bool) matchKey {
	key := matchKey{
		hasMethod:   match.Method != "",
		headerCount: len(match.Headers),
		queryCount:  len(match.QueryParams),
	}
	if match.Path == nil {
		if httpPath && match.GRPC == nil {
			key.pathRank = pathRankPathPrefix
		}
		return key
	}
	switch match.Path.Type {
	case "Exact":
		key.pathRank = pathRankExact
	case "PathPrefix":
		key.pathRank = pathRankPathPrefix
		key.prefixSegments = pathSegments(match.Path.Value)
	case "RegularExpression":
		key.pathRank = pathRankRegularExpression
	}
	return key
}

func pathSegments(value string) int {
	trimmed := strings.Trim(value, "/")
	if trimmed == "" {
		return 0
	}
	return strings.Count(trimmed, "/") + 1
}

// compare returns the sign of key versus other and the deciding criterion.
func (key matchKey) compare(other matchKey) (int, string) {
	if c := cmp.Compare(key.pathRank, other.pathRank); c != 0 {
		return c, criterionPathType
	}
	if c := cmp.Compare(key.prefixSegments, other.prefixSegments); c != 0 {
		return c, criterionPrefixLength
	}
	if key.hasMethod != other.hasMethod {
		if key.hasMethod {
			return 1, criterionMethod
		}
		return -1, criterionMethod
	}
	if c := cmp.Compare(key.headerCount, other.headerCount); c != 0 {
		return c, criterionHeaders
	}
	if c := cmp.Compare(key.queryCount, other.queryCount); c != 0 {
		return c, criterionQueryParams
	}
	return 0, ""
}

// ruleKey is the best match key of the rule. A rule with no matches is ranked as
// a single empty match, so on HTTP routes it equals PathPrefix "/".
func ruleKey(ref ruleRef) matchKey {
	var best matchKey
	if ref.rule == nil {
		return best
	}
	httpPath := ref.route.Identity.Kind != "GRPCRoute"
	if len(ref.rule.Matches) == 0 {
		return newMatchKey(Match{}, httpPath)
	}
	for _, match := range ref.rule.Matches {
		key := newMatchKey(match, httpPath)
		if c, _ := key.compare(best); c > 0 {
			best = key
		}
	}
	return best
}

func compareRulePrecedence(left, right ruleRef) (precedenceOutcome, string) {
	if c, criterion := ruleKey(left).compare(ruleKey(right)); c != 0 {
		return outcomeFor(c), criterion
	}
	// A missing timestamp is unknown, not oldest: skip the step unless both are known.
	if leftTime, ok := creationTime(left.rule); ok {
		if rightTime, ok := creationTime(right.rule); ok {
			if c := leftTime.Compare(rightTime); c != 0 {
				return outcomeFor(-c), criterionCreatedAt
			}
		}
	}
	if c := strings.Compare(identityKey(left.route.Identity), identityKey(right.route.Identity)); c != 0 {
		return outcomeFor(-c), criterionNamespace
	}
	if c := cmp.Compare(left.rule.Index, right.rule.Index); c != 0 {
		return outcomeFor(-c), criterionRuleIndex
	}
	return outcomeUndecided, criterionUndecided
}

func creationTime(rule *Rule) (time.Time, bool) {
	if rule.CreatedAt == nil || rule.CreatedAt.IsZero() {
		return time.Time{}, false
	}
	return rule.CreatedAt.Time, true
}

// outcomeFor maps a positive comparison to a left win and a negative one to a right win.
func outcomeFor(c int) precedenceOutcome {
	if c > 0 {
		return outcomeLeftWins
	}
	return outcomeRightWins
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
