package simulate

import (
	"fmt"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/ninoamine/istio-route/internal/audit"
	"github.com/ninoamine/istio-route/internal/source"
)

// Certainty states whether a comparison is settled by the snapshot or depends on unconfirmed evidence.
type Certainty string

const (
	CertaintyDefinite     Certainty = "definite"
	CertaintyUndetermined Certainty = "undetermined"
)

// Candidate is one rule of an attached HTTPRoute competing for traffic on one listener.
type Candidate struct {
	Route     RouteRef
	RuleIndex int
	Backends  []BackendRef
	State     AttachmentState
	// Matches holds one region per rule match, with route hostnames applied.
	Matches []Predicate
	// Region is the union of Matches.
	Region Predicate

	route audit.Route
	rule  audit.Rule
}

// View maps each applicable listener name to its candidates, highest precedence first.
type View map[string][]Candidate

// BuildView builds the candidate rules of every route whose attachment is accepted or unconfirmed.
// Rejected attachments never compete.
func BuildView(snapshot source.Snapshot, waypoint Waypoint, attachments []RouteAttachments) View {
	routes := make(map[RouteRef]source.Resource)
	for _, res := range snapshot.Resources {
		if _, ok := res.Object.(*gatewayv1.HTTPRoute); ok {
			routes[routeRef(res)] = res
		}
	}
	listenerHostnames := make(map[string]string, len(waypoint.Listeners))
	for _, listener := range waypoint.Listeners {
		listenerHostnames[listener.Name] = listener.Hostname
	}

	view := make(View)
	for _, attachment := range attachments {
		res, ok := routes[attachment.Route]
		if !ok {
			continue
		}
		route, ok := res.Object.(*gatewayv1.HTTPRoute)
		if !ok {
			continue
		}
		for _, listener := range attachment.Listeners {
			if listener.State != AttachmentAccepted && listener.State != AttachmentUnconfirmed {
				continue
			}
			view[listener.Listener] = append(view[listener.Listener],
				routeCandidates(res, route, listener, listenerHostnames[listener.Listener])...)
		}
	}
	for name := range view {
		sortCandidates(view[name])
	}
	return view
}

// routeCandidates returns one candidate per rule of route on a single listener.
func routeCandidates(res source.Resource, route *gatewayv1.HTTPRoute, listener ListenerAttachment, listenerHostname string) []Candidate {
	hostnames := routeHostnames(route)
	if len(hostnames) == 0 && listenerHostname != "" {
		hostnames = []string{listenerHostname}
	}
	if len(hostnames) == 0 {
		hostnames = []string{""}
	}
	created := route.CreationTimestamp
	auditRoute := audit.Route{Identity: res.Identity}
	auditRules := make([]audit.Rule, 0, len(route.Spec.Rules))
	for i, rule := range route.Spec.Rules {
		auditRules = append(auditRules, audit.Rule{Index: i + 1, Matches: ruleMatches(rule), CreatedAt: timePtr(created)})
	}
	auditRoute.Rules = auditRules

	out := make([]Candidate, 0, len(route.Spec.Rules))
	for i, rule := range route.Spec.Rules {
		auditRule := auditRules[i]
		regions := make([]Predicate, 0, len(auditRule.Matches))
		for _, match := range auditRule.Matches {
			hostRegions := make([]Predicate, 0, len(hostnames))
			for _, host := range hostnames {
				hostRegions = append(hostRegions, FromMatch(host, match))
			}
			regions = append(regions, Or(hostRegions...))
		}
		out = append(out, Candidate{
			Route:     routeRef(res),
			RuleIndex: auditRule.Index,
			Backends:  ruleBackends(res.Identity.Namespace, rule),
			State:     listener.State,
			Matches:   regions,
			Region:    Or(regions...),
			route:     auditRoute,
			rule:      auditRule,
		})
	}
	return out
}

// ruleMatches returns the rule's matches; a rule without matches matches all traffic.
func ruleMatches(rule gatewayv1.HTTPRouteRule) []audit.Match {
	if len(rule.Matches) == 0 {
		return []audit.Match{{}}
	}
	out := make([]audit.Match, 0, len(rule.Matches))
	for _, match := range rule.Matches {
		out = append(out, audit.HTTPMatchFrom(match))
	}
	return out
}

func ruleBackends(routeNamespace string, rule gatewayv1.HTTPRouteRule) []BackendRef {
	out := make([]BackendRef, 0, len(rule.BackendRefs))
	for _, ref := range rule.BackendRefs {
		backend := BackendRef{Namespace: routeNamespace, Name: string(ref.Name), Weight: ref.Weight}
		if ref.Namespace != nil {
			backend.Namespace = string(*ref.Namespace)
		}
		if ref.Port != nil {
			port := int32(*ref.Port)
			backend.Port = &port
		}
		out = append(out, backend)
	}
	return out
}

func timePtr(t metav1.Time) *metav1.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// sortCandidates orders candidates by audit precedence. Ties that precedence leaves undecided
// fall back to route and rule identity so the order never depends on input order.
func sortCandidates(candidates []Candidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if c, _ := audit.CompareRules(a.route, a.rule, b.route, b.rule); c != 0 {
			return c > 0
		}
		if c := compareRouteRefs(a.Route, b.Route); c != 0 {
			return c < 0
		}
		return a.RuleIndex < b.RuleIndex
	})
}

// Winner returns the highest-precedence candidate on listener that serves region.
// It reports ProviderNone when no candidate can serve any part of region. Any relation it cannot
// decide, a partial cover by the first overlapping candidate, or an unconfirmed candidate yields
// an undetermined result.
func Winner(view View, listener string, region Predicate) (ProviderView, Certainty) {
	for _, candidate := range view[listener] {
		switch Relate(region, candidate.Region) {
		case RelationDisjoint:
			continue
		case RelationSubset, RelationEqual:
			return knownProvider(candidate), candidateCertainty(candidate)
		default:
			return ProviderView{State: ProviderUnknown}, CertaintyUndetermined
		}
	}
	return ProviderView{State: ProviderNone}, CertaintyDefinite
}

func knownProvider(c Candidate) ProviderView {
	return ProviderView{
		State:    ProviderKnown,
		Provider: &Provider{Route: c.Route, RuleIndex: c.RuleIndex, Backends: c.Backends},
	}
}

func candidateCertainty(c Candidate) Certainty {
	if c.State == AttachmentUnconfirmed {
		return CertaintyUndetermined
	}
	return CertaintyDefinite
}

func backendKey(b BackendRef) string {
	port := "*"
	if b.Port != nil {
		port = fmt.Sprint(*b.Port)
	}
	return fmt.Sprintf("%s/%s:%s", b.Namespace, b.Name, port)
}

func sameBackends(a, b []BackendRef) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if backendKey(a[i]) != backendKey(b[i]) {
			return false
		}
	}
	return true
}

func sortedListeners(view View) []string {
	names := make([]string, 0, len(view))
	for name := range view {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
