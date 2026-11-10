package simulate

import (
	"fmt"
	"sort"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/ninoamine/istio-route/internal/audit"
	"github.com/ninoamine/istio-route/internal/model"
	"github.com/ninoamine/istio-route/internal/source"
)

const gatewayClassUnavailableReason = "GatewayClass istio-waypoint definition unavailable"

// BuildImpact classifies the traffic that the target route serves before removal on each listener.
// before is the view with the target attached; after is the view with the target removed.
func BuildImpact(before, after View, target RouteRef, waypoint Waypoint, scope Scope) Result {
	result := Result{Kind: ResultImpact, Waypoint: waypoint, Scope: scope}
	waypointUnconfirmed := waypoint.Validation == ValidationUnconfirmed

	remainingAny := false
	for _, listener := range sortedListeners(after) {
		if len(after[listener]) > 0 {
			remainingAny = true
			break
		}
	}
	if !remainingAny {
		result.Warnings = append(result.Warnings,
			"no remaining in-scope HTTPRoutes attach to this waypoint; removal would leave all traffic unrouted")
	}

	for _, listener := range sortedListeners(before) {
		remaining := after[listener]
		var higher []Candidate
		for _, cand := range before[listener] {
			if cand.Route == target {
				for _, match := range cand.Matches {
					result.Entries = append(result.Entries,
						classifyMatch(listener, cand, match, higher, remaining, waypointUnconfirmed)...)
				}
				continue
			}
			higher = append(higher, cand)
		}
	}

	sortEntries(result.Entries)
	result.RuleSummaries = summarizeRules(result.Entries, before)
	result.Totals = totalEntries(result.Entries)
	return result
}

// classifyMatch splits one match region of the target into the part shadowed by higher-precedence
// rules (no change) and the effective part, then classifies the effective part against the
// remaining rules.
func classifyMatch(listener string, target Candidate, match Predicate, higher, remaining []Candidate, waypointUnconfirmed bool) []Entry {
	var entries []Entry
	targetUnconfirmed := target.State == AttachmentUnconfirmed
	higherRegions := make([]Predicate, 0, len(higher))
	for _, c := range higher {
		higherRegions = append(higherRegions, c.Region)
	}

	effective := match
	if len(higher) > 0 {
		shadowed := And(match, Or(higherRegions...))
		if !provablyEmpty(shadowed) {
			provider := firstOverlap(shadowed, higher)
			certainty := CertaintyDefinite
			if targetUnconfirmed || provider == nil || provider.State == AttachmentUnconfirmed {
				certainty = CertaintyUndetermined
			}
			view := ProviderView{State: ProviderUnknown}
			if provider != nil {
				view = knownProvider(*provider)
			}
			entries = append(entries, Entry{
				Listener: listener,
				Rule:     RuleRef{Route: target.Route, Index: target.RuleIndex},
				Region:   shadowed,
				Outcome:  ClassificationNoChange,
				Before:   view,
				After:    view,
				Reasons:  []string{"traffic is served by a higher-precedence rule and is unaffected"},
			})
			if certainty == CertaintyUndetermined {
				entries[len(entries)-1] = undetermined(entries[len(entries)-1], ClassificationNoChange, "shadowing evidence is unconfirmed")
			}
		}
		effective = And(match, Not(Or(higherRegions...)))
	}
	if provablyEmpty(effective) {
		return entries
	}

	entry := Entry{
		Listener: listener,
		Rule:     RuleRef{Route: target.Route, Index: target.RuleIndex},
		Region:   effective,
		Before:   knownProvider(target),
	}
	overlapping := overlapsOf(effective, remaining)
	remainingRegions := make([]Predicate, 0, len(remaining))
	for _, c := range remaining {
		remainingRegions = append(remainingRegions, c.Region)
	}
	otherUnconfirmed := targetUnconfirmed || anyUnconfirmed(higher, match)

	switch {
	case len(overlapping) == 0:
		entry.Outcome = ClassificationUnrouted
		entry.After = ProviderView{State: ProviderNone}
		entry.Reasons = []string{"no remaining rule serves this traffic"}
		if reason := unconfirmedReason(waypointUnconfirmed, otherUnconfirmed); reason != "" {
			entry = undetermined(entry, ClassificationUnrouted, reason)
		}
	case Relate(effective, Or(remainingRegions...)) == RelationSubset || Relate(effective, Or(remainingRegions...)) == RelationEqual:
		provider := overlapping[0]
		entry.Outcome = ClassificationStillCovered
		entry.After = knownProvider(provider)
		entry.BackendChanged = !sameBackends(target.Backends, provider.Backends)
		entry.Reasons = []string{"traffic is served by a remaining rule"}
		if reason := unconfirmedReason(waypointUnconfirmed, otherUnconfirmed || anyUnconfirmedCandidate(overlapping)); reason != "" {
			entry = undetermined(entry, ClassificationStillCovered, reason)
		}
	default:
		entry.Outcome = ClassificationUndetermined
		entry.After = ProviderView{State: ProviderUnknown}
		entry.Reasons = []string{"remaining rules only partially cover this traffic"}
		provisional := ClassificationUnrouted
		if len(overlapping) > 0 {
			provisional = ClassificationStillCovered
		}
		entry.Provisional = &Provisional{Outcome: provisional, Reasons: []string{"provisional: partial coverage"}}
	}
	return append(entries, entry)
}

// unconfirmedReason explains why an unrouted or still-covered entry is undetermined.
// An unconfirmed waypoint takes precedence because its GatewayClass definition is missing.
func unconfirmedReason(waypointUnconfirmed, otherUnconfirmed bool) string {
	switch {
	case waypointUnconfirmed:
		return gatewayClassUnavailableReason
	case otherUnconfirmed:
		return "removal depends on unconfirmed evidence"
	default:
		return ""
	}
}

// otherParentNotes names the Gateways other than the selected waypoint that the target route also
// attaches to directly. Their traffic is outside this simulation.
func otherParentNotes(snapshot source.Snapshot, target RouteRef, waypoint Waypoint) []string {
	waypointID := model.ResourceIdentity{Group: gatewayAPIGroup, Kind: gatewayKind, Namespace: waypoint.Namespace, Name: waypoint.Name}
	var notes []string
	for _, res := range snapshot.Resources {
		route, ok := res.Object.(*gatewayv1.HTTPRoute)
		if !ok || routeRef(res) != target {
			continue
		}
		for _, ref := range route.Spec.ParentRefs {
			if !isGatewayParentRef(ref) || audit.ParentTargets(ref, target.Namespace, waypointID) {
				continue
			}
			namespace := target.Namespace
			if ref.Namespace != nil && string(*ref.Namespace) != "" {
				namespace = string(*ref.Namespace)
			}
			notes = append(notes, fmt.Sprintf("also attached to %s/%s; impact not included", namespace, ref.Name))
		}
	}
	return notes
}

func isGatewayParentRef(ref gatewayv1.ParentReference) bool {
	return (ref.Group == nil || string(*ref.Group) == gatewayAPIGroup) &&
		(ref.Kind == nil || string(*ref.Kind) == gatewayKind)
}

// undetermined downgrades a classified entry to undetermined while keeping the provisional outcome.
func undetermined(e Entry, provisional Classification, reason string) Entry {
	e.Provisional = &Provisional{Outcome: provisional, Reasons: e.Reasons}
	e.Outcome = ClassificationUndetermined
	e.Reasons = append(e.Reasons, reason)
	return e
}

// overlapsOf returns the candidates that may serve some part of region, in precedence order.
func overlapsOf(region Predicate, candidates []Candidate) []Candidate {
	var out []Candidate
	for _, c := range candidates {
		if !provablyEmpty(And(region, c.Region)) {
			out = append(out, c)
		}
	}
	return out
}

func firstOverlap(region Predicate, candidates []Candidate) *Candidate {
	for i := range candidates {
		if !provablyEmpty(And(region, candidates[i].Region)) {
			return &candidates[i]
		}
	}
	return nil
}

func anyUnconfirmed(candidates []Candidate, region Predicate) bool {
	for _, c := range candidates {
		if c.State == AttachmentUnconfirmed && !provablyEmpty(And(region, c.Region)) {
			return true
		}
	}
	return false
}

func anyUnconfirmedCandidate(candidates []Candidate) bool {
	for _, c := range candidates {
		if c.State == AttachmentUnconfirmed {
			return true
		}
	}
	return false
}

// provablyEmpty reports whether p is proven to match no traffic. Expansions that exceed the term
// bound are never proven empty.
func provablyEmpty(p Predicate) bool {
	terms, ok := toTerms(p, false)
	if !ok {
		return false
	}
	for _, t := range terms {
		if !termEmpty(t) {
			return false
		}
	}
	return true
}

func termEmpty(t term) bool {
	for i, a := range t.pos {
		for _, b := range t.pos[i+1:] {
			if atomRelation(a, b) == RelationDisjoint {
				return true
			}
		}
		for _, n := range t.neg {
			if r := atomRelation(a, n); r == RelationEqual || r == RelationSubset {
				return true
			}
		}
	}
	return false
}

func sortEntries(entries []Entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.Listener != b.Listener {
			return a.Listener < b.Listener
		}
		if a.Rule.Index != b.Rule.Index {
			return a.Rule.Index < b.Rule.Index
		}
		return a.Region.String() < b.Region.String()
	})
}

func summarizeRules(entries []Entry, before View) []RuleSummary {
	backends := make(map[RuleRef][]BackendRef)
	for _, listener := range sortedListeners(before) {
		for _, c := range before[listener] {
			backends[RuleRef{Route: c.Route, Index: c.RuleIndex}] = c.Backends
		}
	}
	byRule := make(map[RuleRef]*RuleSummary)
	var order []RuleRef
	for _, e := range entries {
		s, ok := byRule[e.Rule]
		if !ok {
			s = &RuleSummary{Rule: e.Rule, Backends: backends[e.Rule], Counts: make(map[Classification]int)}
			byRule[e.Rule] = s
			order = append(order, e.Rule)
		}
		s.Counts[e.Outcome]++
	}
	out := make([]RuleSummary, 0, len(order))
	for _, ref := range order {
		s := byRule[ref]
		for _, class := range classificationOrder {
			if s.Counts[class] > 0 {
				s.Outcomes = append(s.Outcomes, class)
			}
		}
		s.Partial = s.Counts[ClassificationUnrouted] > 0 && s.Counts[ClassificationStillCovered] > 0
		out = append(out, *s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if c := compareRouteRefs(out[i].Rule.Route, out[j].Rule.Route); c != 0 {
			return c < 0
		}
		return out[i].Rule.Index < out[j].Rule.Index
	})
	return out
}

var classificationOrder = []Classification{
	ClassificationUnrouted,
	ClassificationStillCovered,
	ClassificationNoChange,
	ClassificationUndetermined,
}

func totalEntries(entries []Entry) Totals {
	var t Totals
	for _, e := range entries {
		switch e.Outcome {
		case ClassificationUnrouted:
			t.Unrouted++
		case ClassificationStillCovered:
			t.StillCovered++
		case ClassificationNoChange:
			t.NoChange++
		case ClassificationUndetermined:
			t.Undetermined++
		}
	}
	return t
}
