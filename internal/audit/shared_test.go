package audit

import (
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/ninoamine/istio-route/internal/model"
)

func TestSharedIsWaypointGatewayMatchesIsWaypoint(t *testing.T) {
	tests := []struct {
		name  string
		class string
		want  bool
	}{
		{name: "waypoint class", class: "istio-waypoint", want: true},
		{name: "ingress class", class: "istio", want: false},
		{name: "empty class", class: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gateway := &gatewayv1.Gateway{Spec: gatewayv1.GatewaySpec{GatewayClassName: gatewayv1.ObjectName(tt.class)}}
			if got := IsWaypointGateway(gateway); got != isWaypoint(gateway) || got != tt.want {
				t.Fatalf("IsWaypointGateway() = %t, isWaypoint() = %t, want %t", got, isWaypoint(gateway), tt.want)
			}
		})
	}
}

func TestSharedParentTargetsMatchesParentTargets(t *testing.T) {
	waypoint := model.ResourceIdentity{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway", Namespace: "app", Name: "waypoint"}
	ns := func(v string) *gatewayv1.Namespace { n := gatewayv1.Namespace(v); return &n }
	group := func(v string) *gatewayv1.Group { g := gatewayv1.Group(v); return &g }
	kind := func(v string) *gatewayv1.Kind { k := gatewayv1.Kind(v); return &k }
	tests := []struct {
		name           string
		ref            gatewayv1.ParentReference
		routeNamespace string
		want           bool
	}{
		{name: "defaults resolve to same-namespace gateway", ref: gatewayv1.ParentReference{Name: "waypoint"}, routeNamespace: "app", want: true},
		{name: "explicit namespace", ref: gatewayv1.ParentReference{Name: "waypoint", Namespace: ns("app")}, routeNamespace: "shop", want: true},
		{name: "other namespace", ref: gatewayv1.ParentReference{Name: "waypoint", Namespace: ns("shop")}, routeNamespace: "app", want: false},
		{name: "other name", ref: gatewayv1.ParentReference{Name: "other"}, routeNamespace: "app", want: false},
		{name: "service parent", ref: gatewayv1.ParentReference{Name: "waypoint", Group: group(""), Kind: kind("Service")}, routeNamespace: "app", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParentTargets(tt.ref, tt.routeNamespace, waypoint)
			if got != parentTargets(tt.ref, tt.routeNamespace, waypoint) || got != tt.want {
				t.Fatalf("ParentTargets() = %t, parentTargets() = %t, want %t", got, parentTargets(tt.ref, tt.routeNamespace, waypoint), tt.want)
			}
		})
	}
}

func TestSharedHTTPMatchFromMatchesHTTPMatch(t *testing.T) {
	prefix := gatewayv1.PathMatchPathPrefix
	exact := gatewayv1.PathMatchExact
	method := gatewayv1.HTTPMethodPost
	tests := []struct {
		name  string
		match gatewayv1.HTTPRouteMatch
	}{
		{name: "empty match defaults to root prefix", match: gatewayv1.HTTPRouteMatch{}},
		{name: "exact path", match: gatewayv1.HTTPRouteMatch{Path: &gatewayv1.HTTPPathMatch{Type: &exact, Value: ptrString("/api")}}},
		{name: "prefix path with method", match: gatewayv1.HTTPRouteMatch{
			Path:   &gatewayv1.HTTPPathMatch{Type: &prefix, Value: ptrString("/v2")},
			Method: &method,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HTTPMatchFrom(tt.match)
			if want := httpMatch(tt.match); !reflect.DeepEqual(got, want) {
				t.Fatalf("HTTPMatchFrom() = %#v, httpMatch() = %#v", got, want)
			}
		})
	}
}

func ptrString(v string) *string { return &v }

func TestSharedCompareRulesMatchesComparator(t *testing.T) {
	older := metav1.NewTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	newer := metav1.NewTime(older.Add(time.Hour))
	pathPrefix := func(v string) []Match { return []Match{{Path: &PathMatch{Type: "PathPrefix", Value: v}}} }
	pathExact := func(v string) []Match { return []Match{{Path: &PathMatch{Type: "Exact", Value: v}}} }
	route := func(name string) Route {
		return Route{Identity: testIdentity("HTTPRoute", "shop", name)}
	}
	rule := func(matches []Match, createdAt *metav1.Time) Rule {
		return Rule{Index: 1, Matches: matches, CreatedAt: createdAt}
	}

	tests := []struct {
		name          string
		left, right   Rule
		leftRoute     string
		rightRoute    string
		wantWinner    int
		wantCriterion string
	}{
		{
			name:      "exact path beats prefix",
			left:      rule(pathPrefix("/api"), nil),
			right:     rule(pathExact("/api"), nil),
			leftRoute: "a", rightRoute: "b",
			wantWinner:    -1,
			wantCriterion: criterionPathType,
		},
		{
			name:      "longer prefix beats shorter prefix",
			left:      rule(pathPrefix("/api"), nil),
			right:     rule(pathPrefix("/api/v2"), nil),
			leftRoute: "a", rightRoute: "b",
			wantWinner:    -1,
			wantCriterion: criterionPrefixLength,
		},
		{
			name:      "method beats no method",
			left:      rule([]Match{{Path: &PathMatch{Type: "PathPrefix", Value: "/"}, Method: "GET"}}, nil),
			right:     rule(pathPrefix("/"), nil),
			leftRoute: "a", rightRoute: "b",
			wantWinner:    1,
			wantCriterion: criterionMethod,
		},
		{
			name:      "older creation time wins when specificity ties",
			left:      rule(pathPrefix("/"), &newer),
			right:     rule(pathPrefix("/"), &older),
			leftRoute: "a", rightRoute: "b",
			wantWinner:    -1,
			wantCriterion: criterionCreatedAt,
		},
		{
			name:      "namespace/name breaks ties when timestamps are missing",
			left:      rule(pathPrefix("/"), nil),
			right:     rule(pathPrefix("/"), nil),
			leftRoute: "a", rightRoute: "b",
			wantWinner:    1,
			wantCriterion: criterionNamespace,
		},
		{
			name:      "rule index breaks ties within one route",
			left:      Rule{Index: 1, Matches: pathPrefix("/")},
			right:     Rule{Index: 2, Matches: pathPrefix("/")},
			leftRoute: "a", rightRoute: "a",
			wantWinner:    1,
			wantCriterion: criterionRuleIndex,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			leftRoute, rightRoute := route(tt.leftRoute), route(tt.rightRoute)
			gotWinner, gotCriterion := CompareRules(leftRoute, tt.left, rightRoute, tt.right)
			wantOutcome, wantCriterion := compareRulePrecedence(
				ruleRef{route: leftRoute, rule: &tt.left, ruleIndex: tt.left.Index},
				ruleRef{route: rightRoute, rule: &tt.right, ruleIndex: tt.right.Index},
			)
			if gotWinner != tt.wantWinner || gotCriterion != tt.wantCriterion {
				t.Fatalf("CompareRules() = (%d, %q), want (%d, %q)", gotWinner, gotCriterion, tt.wantWinner, tt.wantCriterion)
			}
			if gotCriterion != wantCriterion || (gotWinner == 1) != (wantOutcome == outcomeLeftWins) {
				t.Fatalf("CompareRules() diverges from compareRulePrecedence: (%d, %q) vs (%d, %q)",
					gotWinner, gotCriterion, wantOutcome, wantCriterion)
			}
		})
	}
}

func TestSharedCompareRulesUndecidedWhenEqual(t *testing.T) {
	r := Rule{Index: 1, Matches: []Match{{Path: &PathMatch{Type: "PathPrefix", Value: "/"}}}}
	route := Route{Identity: testIdentity("HTTPRoute", "shop", "a")}
	gotWinner, gotCriterion := CompareRules(route, r, route, r)
	if gotWinner != 0 || gotCriterion != criterionUndecided {
		t.Fatalf("CompareRules() = (%d, %q), want (0, %q)", gotWinner, gotCriterion, criterionUndecided)
	}
}
