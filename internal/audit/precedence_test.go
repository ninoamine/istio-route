package audit

import (
	"strings"
	"testing"
	"time"

	"github.com/ninoamine/istio-route/internal/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestApplyPrecedenceUsesSpecificity(t *testing.T) {
	oldRoute := Route{
		Identity:  testIdentity("HTTPRoute", "shop", "general"),
		Hostnames: []string{"shop.example.com"},
		Rules: []Rule{{
			Index:         1,
			ListenerNames: []string{"https"},
			Matches:       []Match{{Path: &PathMatch{Type: "PathPrefix", Value: "/"}}},
			Precedence:    PrecedenceWinner,
		}},
	}
	specificRoute := Route{
		Identity:  testIdentity("HTTPRoute", "shop", "specific"),
		Hostnames: []string{"shop.example.com"},
		Rules: []Rule{{
			Index:         1,
			ListenerNames: []string{"https"},
			Matches:       []Match{{Path: &PathMatch{Type: "Exact", Value: "/orders"}}},
			Precedence:    PrecedenceWinner,
		}},
	}
	result := Result{Routes: []Route{oldRoute, specificRoute}}
	applyPrecedence(&result)
	if got := result.Routes[0].Rules[0].Precedence; got != PrecedencePartialOverlap {
		t.Fatalf("general route precedence = %q, want %q", got, PrecedencePartialOverlap)
	}
	if got := result.Routes[1].Rules[0].Precedence; got != PrecedenceWinner {
		t.Fatalf("specific route precedence = %q, want %q", got, PrecedenceWinner)
	}
}

func TestApplyPrecedenceUsesCreationTimeAndResourceTieBreakers(t *testing.T) {
	older := &Rule{Index: 1, Matches: []Match{{Path: &PathMatch{Type: "Exact", Value: "/"}}}, CreatedAt: &metav1.Time{Time: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)}}
	newer := &Rule{Index: 1, Matches: []Match{{Path: &PathMatch{Type: "Exact", Value: "/"}}}, CreatedAt: &metav1.Time{Time: time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)}}
	result := Result{Routes: []Route{{Identity: testIdentity("HTTPRoute", "shop", "older"), Hostnames: []string{"shop.example.com"}, Rules: []Rule{*older}}, {Identity: testIdentity("HTTPRoute", "shop", "newer"), Hostnames: []string{"shop.example.com"}, Rules: []Rule{*newer}}}}
	applyPrecedence(&result)
	if got := result.Routes[0].Rules[0].Precedence; got != PrecedenceWinner {
		t.Fatalf("older route precedence = %q, want %q", got, PrecedenceWinner)
	}
	if got := result.Routes[1].Rules[0].Precedence; got != PrecedenceShadowed {
		t.Fatalf("newer route precedence = %q, want %q", got, PrecedenceShadowed)
	}
}

func TestPrecedenceFindingsUseStableCategories(t *testing.T) {
	result := Result{Routes: []Route{{Identity: testIdentity("HTTPRoute", "shop", "general"), Hostnames: []string{"shop.example.com"}, Rules: []Rule{{Index: 1, ListenerNames: []string{"https"}, Matches: []Match{{Path: &PathMatch{Type: "PathPrefix", Value: "/"}}}, Precedence: PrecedenceShadowed}}}, {Identity: testIdentity("HTTPRoute", "shop", "specific"), Hostnames: []string{"shop.example.com"}, Rules: []Rule{{Index: 1, ListenerNames: []string{"https"}, Matches: []Match{{Path: &PathMatch{Type: "Exact", Value: "/"}}}, Precedence: PrecedenceWinner}}}}}
	findings := precedenceFindings(result)
	if len(findings) == 0 {
		t.Fatal("precedenceFindings() returned no findings")
	}
	if findings[0].Category != string(CategoryPrecedence) {
		t.Fatalf("first precedence finding category = %q, want %q", findings[0].Category, CategoryPrecedence)
	}
	if findings[0].Severity != model.FindingSeverityWarning {
		t.Fatalf("first precedence finding severity = %q, want %q", findings[0].Severity, model.FindingSeverityWarning)
	}
}

func TestApplyPrecedenceExactPathBeatsPrefixWithMethodAndHeader(t *testing.T) {
	prefixRoute := Route{
		Identity:  testIdentity("HTTPRoute", "shop", "prefix"),
		Hostnames: []string{"shop.example.com"},
		Rules: []Rule{{
			Index:         1,
			ListenerNames: []string{"https"},
			Matches: []Match{{
				Path:    &PathMatch{Type: "PathPrefix", Value: "/"},
				Method:  "GET",
				Headers: []StringMatch{{Name: "x-env", Type: "Exact", Value: "prod"}},
			}},
			Precedence: PrecedenceWinner,
		}},
	}
	exactRoute := Route{
		Identity:  testIdentity("HTTPRoute", "shop", "exact"),
		Hostnames: []string{"shop.example.com"},
		Rules: []Rule{{
			Index:         1,
			ListenerNames: []string{"https"},
			Matches:       []Match{{Path: &PathMatch{Type: "Exact", Value: "/a"}}},
			Precedence:    PrecedenceWinner,
		}},
	}
	result := Result{Routes: []Route{prefixRoute, exactRoute}}
	applyPrecedence(&result)
	if got := result.Routes[1].Rules[0].Precedence; got != PrecedenceWinner {
		t.Fatalf("exact /a rule precedence = %q, want %q", got, PrecedenceWinner)
	}
	if got := result.Routes[0].Rules[0].Precedence; got != PrecedencePartialOverlap {
		t.Fatalf("prefix / rule with method and header precedence = %q, want %q", got, PrecedencePartialOverlap)
	}
}

func TestApplyPrecedenceLongerPrefixBeatsShorterRegardlessOfCreation(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	older := metav1.NewTime(now)
	newer := metav1.NewTime(now.Add(time.Hour))
	shortPrefixRoute := Route{
		Identity:  testIdentity("HTTPRoute", "shop", "short"),
		Hostnames: []string{"shop.example.com"},
		Rules: []Rule{{
			Index:         1,
			ListenerNames: []string{"https"},
			Matches:       []Match{{Path: &PathMatch{Type: "PathPrefix", Value: "/api"}}},
			CreatedAt:     &older,
			Precedence:    PrecedenceWinner,
		}},
	}
	longPrefixRoute := Route{
		Identity:  testIdentity("HTTPRoute", "shop", "long"),
		Hostnames: []string{"shop.example.com"},
		Rules: []Rule{{
			Index:         1,
			ListenerNames: []string{"https"},
			Matches:       []Match{{Path: &PathMatch{Type: "PathPrefix", Value: "/api/v2"}}},
			CreatedAt:     &newer,
			Precedence:    PrecedenceWinner,
		}},
	}
	result := Result{Routes: []Route{shortPrefixRoute, longPrefixRoute}}
	applyPrecedence(&result)
	if got := result.Routes[1].Rules[0].Precedence; got != PrecedenceWinner {
		t.Fatalf("newer /api/v2 rule precedence = %q, want %q", got, PrecedenceWinner)
	}
	if got := result.Routes[0].Rules[0].Precedence; got != PrecedencePartialOverlap {
		t.Fatalf("older /api rule precedence = %q, want %q", got, PrecedencePartialOverlap)
	}
}

func TestApplyPrecedenceSingleExactMatchBeatsTwoPrefixMatches(t *testing.T) {
	exactRoute := Route{
		Identity:  testIdentity("HTTPRoute", "shop", "exact"),
		Hostnames: []string{"shop.example.com"},
		Rules: []Rule{{
			Index:         1,
			ListenerNames: []string{"https"},
			Matches:       []Match{{Path: &PathMatch{Type: "Exact", Value: "/a"}}},
			Precedence:    PrecedenceWinner,
		}},
	}
	prefixRoute := Route{
		Identity:  testIdentity("HTTPRoute", "shop", "prefixes"),
		Hostnames: []string{"shop.example.com"},
		Rules: []Rule{{
			Index:         1,
			ListenerNames: []string{"https"},
			Matches: []Match{
				{Path: &PathMatch{Type: "PathPrefix", Value: "/a"}},
				{Path: &PathMatch{Type: "PathPrefix", Value: "/b"}},
			},
			Precedence: PrecedenceWinner,
		}},
	}
	result := Result{Routes: []Route{prefixRoute, exactRoute}}
	applyPrecedence(&result)
	if got := result.Routes[1].Rules[0].Precedence; got != PrecedenceWinner {
		t.Fatalf("single exact /a rule precedence = %q, want %q", got, PrecedenceWinner)
	}
	if got := result.Routes[0].Rules[0].Precedence; got != PrecedencePartialOverlap {
		t.Fatalf("two-prefix rule precedence = %q, want %q", got, PrecedencePartialOverlap)
	}
}

func TestApplyPrecedenceMethodBeatsTwoHeaders(t *testing.T) {
	headersRoute := Route{
		Identity:  testIdentity("HTTPRoute", "shop", "headers"),
		Hostnames: []string{"shop.example.com"},
		Rules: []Rule{{
			Index:         1,
			ListenerNames: []string{"https"},
			Matches: []Match{{
				Path: &PathMatch{Type: "PathPrefix", Value: "/"},
				Headers: []StringMatch{
					{Name: "x-env", Type: "Exact", Value: "prod"},
					{Name: "x-tier", Type: "Exact", Value: "gold"},
				},
			}},
			Precedence: PrecedenceWinner,
		}},
	}
	methodRoute := Route{
		Identity:  testIdentity("HTTPRoute", "shop", "method"),
		Hostnames: []string{"shop.example.com"},
		Rules: []Rule{{
			Index:         1,
			ListenerNames: []string{"https"},
			Matches: []Match{{
				Path:   &PathMatch{Type: "PathPrefix", Value: "/"},
				Method: "GET",
			}},
			Precedence: PrecedenceWinner,
		}},
	}
	result := Result{Routes: []Route{headersRoute, methodRoute}}
	applyPrecedence(&result)
	if got := result.Routes[1].Rules[0].Precedence; got != PrecedenceWinner {
		t.Fatalf("method rule precedence = %q, want %q", got, PrecedenceWinner)
	}
	if got := result.Routes[0].Rules[0].Precedence; got != PrecedencePartialOverlap {
		t.Fatalf("two-header rule precedence = %q, want %q", got, PrecedencePartialOverlap)
	}
}

type precedenceSide struct {
	namespace string
	name      string
	index     int
	version   string
	createdAt *time.Time
	matches   []Match
}

func (s precedenceSide) route() Route {
	namespace, name := s.namespace, s.name
	if namespace == "" {
		namespace = "shop"
	}
	if name == "" {
		name = "route"
	}
	index := s.index
	if index == 0 {
		index = 1
	}
	rule := Rule{
		Index:         index,
		ListenerNames: []string{"https"},
		Matches:       s.matches,
		Precedence:    PrecedenceWinner,
	}
	if s.createdAt != nil {
		created := metav1.NewTime(*s.createdAt)
		rule.CreatedAt = &created
	}
	identity := testIdentity("HTTPRoute", namespace, name)
	if s.version != "" {
		identity.Version = s.version
	}
	return Route{
		Identity:  identity,
		Hostnames: []string{"shop.example.com"},
		Rules:     []Rule{rule},
	}
}

func TestApplyPrecedenceComparatorCriteria(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	older := base
	newer := base.Add(time.Hour)
	prefix := func(value string) *PathMatch { return &PathMatch{Type: "PathPrefix", Value: value} }
	regex := func(value string) *PathMatch { return &PathMatch{Type: "RegularExpression", Value: value} }
	exact := func(value string) *PathMatch { return &PathMatch{Type: "Exact", Value: value} }
	header := func(name string) StringMatch { return StringMatch{Name: name, Type: "Exact", Value: "v"} }

	tests := []struct {
		name      string
		left      precedenceSide
		right     precedenceSide
		wantLeft  PrecedenceState
		wantRight PrecedenceState
	}{
		{
			name: "query param count is the last normative criterion",
			left: precedenceSide{name: "left", matches: []Match{{
				Path: prefix("/"), Method: "GET", Headers: []StringMatch{header("x-h")},
				QueryParams: []StringMatch{header("p1"), header("p2")},
			}}},
			right: precedenceSide{name: "right", matches: []Match{{
				Path: prefix("/"), Method: "GET", Headers: []StringMatch{header("x-h")},
				QueryParams: []StringMatch{header("p3")},
			}}},
			wantLeft:  PrecedenceWinner,
			wantRight: PrecedencePartialOverlap,
		},
		{
			name:      "regular expression ranks below prefix",
			left:      precedenceSide{name: "left", matches: []Match{{Path: regex("/a.*")}}},
			right:     precedenceSide{name: "right", matches: []Match{{Path: prefix("/a")}}},
			wantLeft:  PrecedenceShadowed,
			wantRight: PrecedenceWinner,
		},
		{
			name:      "regular expression ranks below exact",
			left:      precedenceSide{name: "left", matches: []Match{{Path: regex("/a.*")}}},
			right:     precedenceSide{name: "right", matches: []Match{{Path: exact("/a")}}},
			wantLeft:  PrecedencePartialOverlap,
			wantRight: PrecedenceWinner,
		},
		{
			name: "regular expression with method and headers still ranks below prefix",
			left: precedenceSide{name: "left", matches: []Match{{
				Path: regex("/a.*"), Method: "GET", Headers: []StringMatch{header("x-a"), header("x-b")},
			}}},
			right:     precedenceSide{name: "right", matches: []Match{{Path: prefix("/a")}}},
			wantLeft:  PrecedencePartialOverlap,
			wantRight: PrecedenceWinner,
		},
		{
			name: "regular expression ranks above a gRPC match, which has no path",
			left: precedenceSide{name: "left", matches: []Match{{Path: regex("/a.*")}}},
			right: precedenceSide{name: "right", matches: []Match{{
				GRPC: &GRPCMatch{}, Headers: []StringMatch{header("x-a")},
			}}},
			wantLeft:  PrecedenceWinner,
			wantRight: PrecedencePartialOverlap,
		},
		{
			name: "regular expression ranks below an HTTP match without path (default PathPrefix /)",
			left: precedenceSide{name: "left", matches: []Match{{Path: regex("/a.*")}}},
			right: precedenceSide{name: "right", matches: []Match{{
				Headers: []StringMatch{header("x-a")},
			}}},
			wantLeft:  PrecedencePartialOverlap,
			wantRight: PrecedenceWinner,
		},
		{
			name:      "catch-all equals PathPrefix / on the path criterion and falls to the timestamp",
			left:      precedenceSide{name: "left", createdAt: &older},
			right:     precedenceSide{name: "right", createdAt: &newer, matches: []Match{{Path: prefix("/")}}},
			wantLeft:  PrecedenceWinner,
			wantRight: PrecedencePartialOverlap,
		},
		{
			name:      "catch-all ranks below a method-only match",
			left:      precedenceSide{name: "left", createdAt: &older},
			right:     precedenceSide{name: "right", createdAt: &newer, matches: []Match{{Method: "GET"}}},
			wantLeft:  PrecedenceShadowed,
			wantRight: PrecedenceWinner,
		},
		{
			name:      "two catch-alls fall through to creation timestamp",
			left:      precedenceSide{name: "left", createdAt: &newer},
			right:     precedenceSide{name: "right", createdAt: &older},
			wantLeft:  PrecedenceShadowed,
			wantRight: PrecedenceWinner,
		},
		{
			name:      "oldest creation timestamp wins first tie-breaker",
			left:      precedenceSide{name: "a", createdAt: &newer, matches: []Match{{Path: prefix("/"), Headers: []StringMatch{header("x-a")}}}},
			right:     precedenceSide{name: "b", createdAt: &older, matches: []Match{{Path: prefix("/"), Headers: []StringMatch{header("x-b")}}}},
			wantLeft:  PrecedencePartialOverlap,
			wantRight: PrecedenceWinner,
		},
		{
			name:      "a missing creation timestamp skips the timestamp step",
			left:      precedenceSide{name: "b", matches: []Match{{Path: prefix("/"), Headers: []StringMatch{header("x-a")}}}},
			right:     precedenceSide{name: "a", createdAt: &older, matches: []Match{{Path: prefix("/"), Headers: []StringMatch{header("x-b")}}}},
			wantLeft:  PrecedencePartialOverlap,
			wantRight: PrecedenceWinner,
		},
		{
			name:      "namespace and name alphabetical is the second tie-breaker",
			left:      precedenceSide{name: "beta", matches: []Match{{Path: prefix("/"), Headers: []StringMatch{header("x-a")}}}},
			right:     precedenceSide{name: "alpha", matches: []Match{{Path: prefix("/"), Headers: []StringMatch{header("x-b")}}}},
			wantLeft:  PrecedencePartialOverlap,
			wantRight: PrecedenceWinner,
		},
		{
			name:      "namespace is compared before name",
			left:      precedenceSide{namespace: "b", name: "a", matches: []Match{{Path: prefix("/"), Headers: []StringMatch{header("x-a")}}}},
			right:     precedenceSide{namespace: "a", name: "z", matches: []Match{{Path: prefix("/"), Headers: []StringMatch{header("x-b")}}}},
			wantLeft:  PrecedencePartialOverlap,
			wantRight: PrecedenceWinner,
		},
		{
			name:      "rule index is the last tie-breaker",
			left:      precedenceSide{name: "same", index: 2, matches: []Match{{Path: prefix("/"), Headers: []StringMatch{header("x-a")}}}},
			right:     precedenceSide{name: "same", index: 1, matches: []Match{{Path: prefix("/"), Headers: []StringMatch{header("x-b")}}}},
			wantLeft:  PrecedencePartialOverlap,
			wantRight: PrecedenceWinner,
		},
		{
			name:      "undecided when criteria and all tie-breakers are equal",
			left:      precedenceSide{name: "same", matches: []Match{{Path: prefix("/"), Headers: []StringMatch{header("x-a")}}}},
			right:     precedenceSide{name: "same", matches: []Match{{Path: prefix("/"), Headers: []StringMatch{header("x-b")}}}},
			wantLeft:  PrecedenceWinner,
			wantRight: PrecedenceWinner,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, swapped := range []bool{false, true} {
				routes := []Route{tt.left.route(), tt.right.route()}
				if swapped {
					routes[0], routes[1] = routes[1], routes[0]
				}
				result := Result{Routes: routes}
				applyPrecedence(&result)
				gotLeft, gotRight := result.Routes[0].Rules[0].Precedence, result.Routes[1].Rules[0].Precedence
				if swapped {
					gotLeft, gotRight = gotRight, gotLeft
				}
				if gotLeft != tt.wantLeft || gotRight != tt.wantRight {
					t.Fatalf("swapped=%t: precedence = (left %q, right %q), want (left %q, right %q)",
						swapped, gotLeft, gotRight, tt.wantLeft, tt.wantRight)
				}
			}
		})
	}
}

func TestCompareRulePrecedenceCreationTimestamp(t *testing.T) {
	older := metav1.NewTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	newer := metav1.NewTime(older.Add(time.Hour))
	zero := metav1.Time{}
	prefix := []Match{{Path: &PathMatch{Type: "PathPrefix", Value: "/"}}}
	ref := func(name string, createdAt *metav1.Time) ruleRef {
		route := Route{Identity: testIdentity("HTTPRoute", "shop", name)}
		route.Rules = []Rule{{Index: 1, Matches: prefix, CreatedAt: createdAt}}
		return ruleRef{route: route, rule: &route.Rules[0]}
	}

	tests := []struct {
		name          string
		left          ruleRef
		right         ruleRef
		wantOutcome   precedenceOutcome
		wantCriterion string
	}{
		{
			name:          "one missing timestamp is decided by namespace/name, timestamped rule first",
			left:          ref("a", &newer),
			right:         ref("b", nil),
			wantOutcome:   outcomeLeftWins,
			wantCriterion: criterionNamespace,
		},
		{
			name:          "one missing timestamp is decided by namespace/name, timestamped rule second",
			left:          ref("a", nil),
			right:         ref("b", &older),
			wantOutcome:   outcomeLeftWins,
			wantCriterion: criterionNamespace,
		},
		{
			name:          "one missing timestamp does not let the timestamped rule lose to zero time",
			left:          ref("b", nil),
			right:         ref("a", &older),
			wantOutcome:   outcomeRightWins,
			wantCriterion: criterionNamespace,
		},
		{
			name:          "one zero timestamp is treated as missing",
			left:          ref("b", &zero),
			right:         ref("a", &older),
			wantOutcome:   outcomeRightWins,
			wantCriterion: criterionNamespace,
		},
		{
			name:          "both missing is decided by namespace/name",
			left:          ref("b", nil),
			right:         ref("a", nil),
			wantOutcome:   outcomeRightWins,
			wantCriterion: criterionNamespace,
		},
		{
			name:          "both zero is decided by namespace/name",
			left:          ref("b", &zero),
			right:         ref("a", &zero),
			wantOutcome:   outcomeRightWins,
			wantCriterion: criterionNamespace,
		},
		{
			name:          "both present, older wins on the left",
			left:          ref("z", &older),
			right:         ref("a", &newer),
			wantOutcome:   outcomeLeftWins,
			wantCriterion: criterionCreatedAt,
		},
		{
			name:          "both present, older wins on the right",
			left:          ref("a", &newer),
			right:         ref("z", &older),
			wantOutcome:   outcomeRightWins,
			wantCriterion: criterionCreatedAt,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOutcome, gotCriterion := compareRulePrecedence(tt.left, tt.right)
			if gotOutcome != tt.wantOutcome || gotCriterion != tt.wantCriterion {
				t.Fatalf("compareRulePrecedence() = (%v, %q), want (%v, %q)", gotOutcome, gotCriterion, tt.wantOutcome, tt.wantCriterion)
			}
		})
	}
}

func findFinding(t *testing.T, findings []model.Finding, code, route string) model.Finding {
	t.Helper()
	for _, finding := range findings {
		if finding.Code == code && len(finding.Resources) == 1 && finding.Resources[0].Name == route {
			return finding
		}
	}
	t.Fatalf("no %s finding for route %q", code, route)
	return model.Finding{}
}

func TestShadowedFindingNamesCoveringRuleAndCriterion(t *testing.T) {
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	exact := []Match{{Path: &PathMatch{Type: "Exact", Value: "/a"}}}
	covering := precedenceSide{name: "covering", createdAt: &older, matches: exact}
	covered := precedenceSide{name: "covered", createdAt: &newer, matches: exact}
	result := Result{Routes: []Route{covered.route(), covering.route()}}
	applyPrecedence(&result)
	findings := precedenceFindings(result)

	finding := findFinding(t, findings, "shadowed-route-rule", "covered")
	if finding.Severity != model.FindingSeverityWarning {
		t.Errorf("shadowed severity = %q, want %q", finding.Severity, model.FindingSeverityWarning)
	}
	if finding.Category != string(CategoryPrecedence) {
		t.Errorf("shadowed category = %q, want %q", finding.Category, CategoryPrecedence)
	}
	if want := "Reorder or narrow the higher-precedence route so the lower-priority rule still receives applicable traffic."; finding.SuggestedFix != want {
		t.Errorf("shadowed SuggestedFix = %q, want %q", finding.SuggestedFix, want)
	}
	for _, want := range []string{"HTTPRoute shop/covering rule 1", criterionCreatedAt} {
		if !strings.Contains(finding.Message, want) {
			t.Errorf("shadowed message = %q, want it to contain %q", finding.Message, want)
		}
	}
	for _, f := range findings {
		if f.Code == "partial-overlap-route-rule" {
			t.Errorf("fully covered rule must not be reported as partial-overlap: %+v", f)
		}
	}
}

func TestPartialOverlapFindingNamesCompetingRuleAndCriterion(t *testing.T) {
	exact := precedenceSide{name: "exact", matches: []Match{{Path: &PathMatch{Type: "Exact", Value: "/a"}}}}
	prefix := precedenceSide{name: "prefix", matches: []Match{{Path: &PathMatch{Type: "PathPrefix", Value: "/"}}}}
	result := Result{Routes: []Route{prefix.route(), exact.route()}}
	applyPrecedence(&result)
	findings := precedenceFindings(result)

	finding := findFinding(t, findings, "partial-overlap-route-rule", "prefix")
	if finding.Severity != model.FindingSeverityInfo {
		t.Errorf("partial-overlap severity = %q, want %q", finding.Severity, model.FindingSeverityInfo)
	}
	if finding.Category != string(CategoryPrecedence) {
		t.Errorf("partial-overlap category = %q, want %q", finding.Category, CategoryPrecedence)
	}
	if want := "Keep the lower-priority rule only if it intentionally handles traffic outside the overlapping match set."; finding.SuggestedFix != want {
		t.Errorf("partial-overlap SuggestedFix = %q, want %q", finding.SuggestedFix, want)
	}
	for _, want := range []string{"HTTPRoute shop/exact rule 1", criterionPathType} {
		if !strings.Contains(finding.Message, want) {
			t.Errorf("partial-overlap message = %q, want it to contain %q", finding.Message, want)
		}
	}
	for _, f := range findings {
		if f.Code == "shadowed-route-rule" {
			t.Errorf("partially overlapping rule must not be reported as shadowed: %+v", f)
		}
	}
}

func TestWinningFindingNamesDecidingCriterion(t *testing.T) {
	exact := precedenceSide{name: "exact", matches: []Match{{Path: &PathMatch{Type: "Exact", Value: "/a"}}}}
	prefix := precedenceSide{name: "prefix", matches: []Match{{Path: &PathMatch{Type: "PathPrefix", Value: "/"}}}}
	result := Result{Routes: []Route{exact.route(), prefix.route()}}
	applyPrecedence(&result)
	findings := precedenceFindings(result)

	finding := findFinding(t, findings, "winning-route-rule", "exact")
	if finding.Severity != model.FindingSeverityInfo {
		t.Errorf("winning severity = %q, want %q", finding.Severity, model.FindingSeverityInfo)
	}
	if finding.Category != string(CategoryPrecedence) {
		t.Errorf("winning category = %q, want %q", finding.Category, CategoryPrecedence)
	}
	if want := "Review overlapping rules to confirm the winning precedence is intentional."; finding.SuggestedFix != want {
		t.Errorf("winning SuggestedFix = %q, want %q", finding.SuggestedFix, want)
	}
	if !strings.Contains(finding.Message, criterionPathType) {
		t.Errorf("winning message = %q, want it to contain deciding criterion %q", finding.Message, criterionPathType)
	}
}

func TestUndecidedOverlapEmitsFindingOnBothRules(t *testing.T) {
	same := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	header := func(name string) []Match {
		return []Match{{Path: &PathMatch{Type: "PathPrefix", Value: "/"}, Headers: []StringMatch{{Name: name, Type: "Exact", Value: "v"}}}}
	}
	// The sides share namespace/name and rule index (the identity tie-breakers); only the
	// API version differs so rulesEquivalent does not treat them as the same rule.
	tests := []struct {
		name  string
		left  precedenceSide
		right precedenceSide
	}{
		{
			name:  "missing creation timestamps",
			left:  precedenceSide{name: "same", matches: header("x-a")},
			right: precedenceSide{name: "same", version: "v1beta1", matches: header("x-b")},
		},
		{
			name:  "identical creation timestamps",
			left:  precedenceSide{name: "same", createdAt: &same, matches: header("x-a")},
			right: precedenceSide{name: "same", version: "v1beta1", createdAt: &same, matches: header("x-b")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, swapped := range []bool{false, true} {
				routes := []Route{tt.left.route(), tt.right.route()}
				if swapped {
					routes[0], routes[1] = routes[1], routes[0]
				}
				result := Result{Routes: routes}
				applyPrecedence(&result)
				for _, route := range result.Routes {
					for _, rule := range route.Rules {
						if rule.Precedence == PrecedenceShadowed || rule.Precedence == PrecedencePartialOverlap {
							t.Errorf("swapped=%t: undecided rule %d precedence = %q, want no shadowed or partial-overlap state", swapped, rule.Index, rule.Precedence)
						}
					}
				}
				findings := precedenceFindings(result)
				undecided := 0
				for _, f := range findings {
					switch f.Code {
					case "undecided-route-rule-overlap":
						undecided++
						if f.Severity != model.FindingSeverityInfo {
							t.Errorf("swapped=%t: undecided severity = %q, want %q", swapped, f.Severity, model.FindingSeverityInfo)
						}
						if f.Category != string(CategoryPrecedence) {
							t.Errorf("swapped=%t: undecided category = %q, want %q", swapped, f.Category, CategoryPrecedence)
						}
					case "winning-route-rule":
						t.Errorf("swapped=%t: undecided pair must not invent a winner: %+v", swapped, f)
					case "shadowed-route-rule", "partial-overlap-route-rule":
						t.Errorf("swapped=%t: undecided pair must not produce %s: %+v", swapped, f.Code, f)
					}
				}
				if undecided != 2 {
					t.Errorf("swapped=%t: got %d undecided-route-rule-overlap findings, want 2 (one per rule)", swapped, undecided)
				}
			}
		})
	}
}

func hasWinningFinding(findings []model.Finding, route string) bool {
	for _, finding := range findings {
		if finding.Code == "winning-route-rule" && len(finding.Resources) == 1 && finding.Resources[0].Name == route {
			return true
		}
	}
	return false
}

func TestSameRouteRulesAreTreatedConsistently(t *testing.T) {
	rule := func(index int, path *PathMatch) Rule {
		return Rule{Index: index, ListenerNames: []string{"https"}, Matches: []Match{{Path: path}}, Precedence: PrecedenceWinner}
	}
	exact := &PathMatch{Type: "Exact", Value: "/a"}
	prefix := &PathMatch{Type: "PathPrefix", Value: "/"}

	tests := []struct {
		name       string
		routes     []Route
		wantStates [][]PrecedenceState
		wantWinner string
	}{
		{
			name: "winner over a sibling in its own route gets a winning finding",
			routes: []Route{{
				Identity:  testIdentity("HTTPRoute", "shop", "one"),
				Hostnames: []string{"shop.example.com"},
				Rules:     []Rule{rule(1, exact), rule(2, prefix)},
			}},
			wantStates: [][]PrecedenceState{{PrecedenceWinner, PrecedencePartialOverlap}},
			wantWinner: "one",
		},
		{
			name: "winner over a rule of another route gets a winning finding",
			routes: []Route{
				{Identity: testIdentity("HTTPRoute", "shop", "one"), Hostnames: []string{"shop.example.com"}, Rules: []Rule{rule(1, exact)}},
				{Identity: testIdentity("HTTPRoute", "shop", "two"), Hostnames: []string{"shop.example.com"}, Rules: []Rule{rule(1, prefix)}},
			},
			wantStates: [][]PrecedenceState{{PrecedenceWinner}, {PrecedencePartialOverlap}},
			wantWinner: "one",
		},
		{
			name: "a tie inside one route goes to the first rule in list order",
			routes: []Route{{
				Identity:  testIdentity("HTTPRoute", "shop", "one"),
				Hostnames: []string{"shop.example.com"},
				Rules: []Rule{
					{Index: 1, ListenerNames: []string{"https"}, Matches: []Match{{Path: prefix, Headers: []StringMatch{{Name: "x-a", Type: "Exact", Value: "v"}}}}, Precedence: PrecedenceWinner},
					{Index: 2, ListenerNames: []string{"https"}, Matches: []Match{{Path: prefix, Headers: []StringMatch{{Name: "x-b", Type: "Exact", Value: "v"}}}}, Precedence: PrecedenceWinner},
				},
			}},
			wantStates: [][]PrecedenceState{{PrecedenceWinner, PrecedencePartialOverlap}},
			wantWinner: "one",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Result{Routes: tt.routes}
			applyPrecedence(&result)
			for i, route := range result.Routes {
				for j, rule := range route.Rules {
					if rule.Precedence != tt.wantStates[i][j] {
						t.Errorf("route %d rule %d precedence = %q, want %q", i, j, rule.Precedence, tt.wantStates[i][j])
					}
				}
			}
			if !hasWinningFinding(precedenceFindings(result), tt.wantWinner) {
				t.Errorf("no winning-route-rule finding for %q", tt.wantWinner)
			}
		})
	}
}

func TestCompareRulePrecedenceCatchAllEqualsRootPrefix(t *testing.T) {
	prefix := func(value string) []Match { return []Match{{Path: &PathMatch{Type: "PathPrefix", Value: value}}} }
	ref := func(kind, name string, matches []Match) ruleRef {
		route := Route{Identity: testIdentity(kind, "shop", name)}
		route.Rules = []Rule{{Index: 1, Matches: matches}}
		return ruleRef{route: route, rule: &route.Rules[0]}
	}
	header := StringMatch{Name: "x-a", Type: "Exact", Value: "v"}

	tests := []struct {
		name          string
		left          ruleRef
		right         ruleRef
		wantOutcome   precedenceOutcome
		wantCriterion string
	}{
		{
			name:          "no matches vs PathPrefix / falls to tie-breakers",
			left:          ref("HTTPRoute", "b", nil),
			right:         ref("HTTPRoute", "a", prefix("/")),
			wantOutcome:   outcomeRightWins,
			wantCriterion: criterionNamespace,
		},
		{
			name:          "empty match (pipeline form of no matches) vs PathPrefix / falls to tie-breakers",
			left:          ref("HTTPRoute", "b", []Match{{}}),
			right:         ref("HTTPRoute", "a", prefix("/")),
			wantOutcome:   outcomeRightWins,
			wantCriterion: criterionNamespace,
		},
		{
			name:          "match without path vs PathPrefix / falls to tie-breakers",
			left:          ref("HTTPRoute", "a", []Match{{}}),
			right:         ref("HTTPRoute", "b", prefix("/")),
			wantOutcome:   outcomeLeftWins,
			wantCriterion: criterionNamespace,
		},
		{
			name:          "no matches vs PathPrefix / plus header: header rule wins",
			left:          ref("HTTPRoute", "a", nil),
			right:         ref("HTTPRoute", "b", []Match{{Path: &PathMatch{Type: "PathPrefix", Value: "/"}, Headers: []StringMatch{header}}}),
			wantOutcome:   outcomeRightWins,
			wantCriterion: criterionHeaders,
		},
		{
			name:          "no matches vs match with only a method: method rule wins",
			left:          ref("HTTPRoute", "a", nil),
			right:         ref("HTTPRoute", "b", []Match{{Method: "GET"}}),
			wantOutcome:   outcomeRightWins,
			wantCriterion: criterionMethod,
		},
		{
			name:          "no matches vs PathPrefix /api: longer prefix wins",
			left:          ref("HTTPRoute", "a", nil),
			right:         ref("HTTPRoute", "b", prefix("/api")),
			wantOutcome:   outcomeRightWins,
			wantCriterion: criterionPrefixLength,
		},
		{
			name:          "gRPC empty match still ranks below an HTTP PathPrefix /",
			left:          ref("GRPCRoute", "a", []Match{{}}),
			right:         ref("HTTPRoute", "b", prefix("/")),
			wantOutcome:   outcomeRightWins,
			wantCriterion: criterionPathType,
		},
		{
			name:          "gRPC no matches still ranks below an HTTP PathPrefix /",
			left:          ref("GRPCRoute", "a", nil),
			right:         ref("HTTPRoute", "b", prefix("/")),
			wantOutcome:   outcomeRightWins,
			wantCriterion: criterionPathType,
		},
		{
			name:          "gRPC header match beats a gRPC empty match",
			left:          ref("GRPCRoute", "b", []Match{{GRPC: &GRPCMatch{}, Headers: []StringMatch{header}}}),
			right:         ref("GRPCRoute", "a", []Match{{}}),
			wantOutcome:   outcomeLeftWins,
			wantCriterion: criterionHeaders,
		},
		{
			name:          "two gRPC empty matches fall to tie-breakers",
			left:          ref("GRPCRoute", "b", []Match{{}}),
			right:         ref("GRPCRoute", "a", nil),
			wantOutcome:   outcomeRightWins,
			wantCriterion: criterionNamespace,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOutcome, gotCriterion := compareRulePrecedence(tt.left, tt.right)
			if gotOutcome != tt.wantOutcome || gotCriterion != tt.wantCriterion {
				t.Fatalf("compareRulePrecedence() = (%v, %q), want (%v, %q)", gotOutcome, gotCriterion, tt.wantOutcome, tt.wantCriterion)
			}
		})
	}
}
