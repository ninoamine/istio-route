package simulate

import (
	"testing"

	"github.com/ninoamine/istio-route/internal/audit"
)

func pathMatch(matchType, value string) audit.Match {
	return audit.Match{Path: &audit.PathMatch{Type: matchType, Value: value}}
}

func pathAtom(matchType, value string) Predicate {
	return atomPredicate(Atom{Dimension: dimPath, MatchType: matchType, Value: value})
}

func TestFromMatchDefaults(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		host  string
		match audit.Match
		want  string
	}{
		{name: "missing path defaults to prefix root", match: audit.Match{}, want: "path:PathPrefix=/"},
		{name: "wildcard host is omitted", host: "*", match: pathMatch("Exact", "/a"), want: "path:Exact=/a"},
		{name: "host and method", host: "shop.example.com", match: audit.Match{Method: "GET"}, want: "(host=shop.example.com && method=GET && path:PathPrefix=/)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := FromMatch(tt.host, tt.match).String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCanonicalIsOrderIndependent(t *testing.T) {
	t.Parallel()
	a := And(pathAtom("PathPrefix", "/api"), atomPredicate(Atom{Dimension: dimMethod, Value: "GET"}))
	b := And(atomPredicate(Atom{Dimension: dimMethod, Value: "GET"}), pathAtom("PathPrefix", "/api"))
	if a.String() != b.String() {
		t.Fatalf("canonical forms differ: %q vs %q", a.String(), b.String())
	}
	if got := And(a, a).String(); got != a.String() {
		t.Fatalf("duplicate children not removed: %q", got)
	}
}

func TestNotCollapsesDoubleNegation(t *testing.T) {
	t.Parallel()
	p := pathAtom("PathPrefix", "/api")
	if got := Not(Not(p)).String(); got != p.String() {
		t.Fatalf("Not(Not(p)) = %q, want %q", got, p.String())
	}
}

func TestRelate(t *testing.T) {
	t.Parallel()
	api := pathAtom("PathPrefix", "/api")
	apiV2 := pathAtom("PathPrefix", "/api/v2")
	apix := pathAtom("PathPrefix", "/apix")
	root := pathAtom("PathPrefix", "/")
	get := atomPredicate(Atom{Dimension: dimMethod, Value: "GET"})
	post := atomPredicate(Atom{Dimension: dimMethod, Value: "POST"})
	headerA := atomPredicate(Atom{Dimension: dimHeader, Name: "x-env", MatchType: matchExact, Value: "a"})
	headerB := atomPredicate(Atom{Dimension: dimHeader, Name: "x-env", MatchType: matchExact, Value: "b"})
	headerRegex := atomPredicate(Atom{Dimension: dimHeader, Name: "x-env", MatchType: "RegularExpression", Value: "a.*"})
	exactAPI := pathAtom("Exact", "/api")
	wild := atomPredicate(Atom{Dimension: dimHostname, Value: "*.example.com"})
	exactHost := atomPredicate(Atom{Dimension: dimHostname, Value: "shop.example.com"})

	tests := []struct {
		name string
		a, b Predicate
		want Relation
	}{
		{name: "identical", a: api, b: api, want: RelationEqual},
		{name: "prefix segment disjoint from longer sibling", a: api, b: apix, want: RelationDisjoint},
		{name: "nested prefix is subset", a: apiV2, b: api, want: RelationSubset},
		{name: "nested prefix is superset", a: api, b: apiV2, want: RelationSuperset},
		{name: "root covers all paths", a: api, b: root, want: RelationSubset},
		{name: "exact inside prefix", a: exactAPI, b: api, want: RelationSubset},
		{name: "prefix covers exact", a: api, b: exactAPI, want: RelationSuperset},
		{name: "different exact paths are disjoint", a: exactAPI, b: pathAtom("Exact", "/web"), want: RelationDisjoint},
		{name: "any method covers GET", a: api, b: And(api, get), want: RelationSuperset},
		{name: "GET and POST are disjoint", a: And(api, get), b: And(api, post), want: RelationDisjoint},
		{name: "wildcard host covers exact host", a: And(api, exactHost), b: And(api, wild), want: RelationSubset},
		{name: "exact host outside wildcard is disjoint", a: And(api, exactHost), b: And(api, atomPredicate(Atom{Dimension: dimHostname, Value: "*.other.org"})), want: RelationDisjoint},
		{name: "identical header atoms", a: headerA, b: headerA, want: RelationEqual},
		{name: "different exact header values are disjoint", a: headerA, b: headerB, want: RelationDisjoint},
		{name: "regex header is unknown", a: headerRegex, b: headerA, want: RelationUnknown},
		{name: "or subset of wider or", a: Or(api, apix), b: Or(api, apix, root), want: RelationSubset},
		{name: "api excluding v2 is subset of api", a: And(api, Not(apiV2)), b: api, want: RelationSubset},
		{name: "api excluding v2 is disjoint from v2", a: And(api, Not(apiV2)), b: apiV2, want: RelationDisjoint},
		{name: "unrelated opaque sets are unknown", a: headerA, b: pathAtom("Exact", "/x"), want: RelationUnknown},
		{name: "or of disjoint branches is disjoint from apix", a: Or(pathAtom("Exact", "/q"), pathAtom("Exact", "/r")), b: apiV2, want: RelationDisjoint},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Relate(tt.a, tt.b); got != tt.want {
				t.Fatalf("Relate(%s, %s) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
