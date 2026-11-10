package simulate

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ninoamine/istio-route/internal/audit"
)

const (
	dimHostname = "hostname"
	dimPath     = "path"
	dimMethod   = "method"
	dimHeader   = "header"
	dimQuery    = "query"

	matchPathPrefix = "PathPrefix"
	matchExact      = "Exact"
)

// FromMatch returns the traffic region matched by one HTTPRoute match on host.
// A missing path is PathPrefix "/" and a missing method matches any method.
func FromMatch(host string, match audit.Match) Predicate {
	var atoms []Predicate
	if host != "" && host != "*" {
		atoms = append(atoms, atomPredicate(Atom{Dimension: dimHostname, Value: host}))
	}

	pathType, pathValue := matchPathPrefix, "/"
	if match.Path != nil {
		if match.Path.Type != "" {
			pathType = match.Path.Type
		}
		if match.Path.Value != "" {
			pathValue = match.Path.Value
		}
	}
	atoms = append(atoms, atomPredicate(Atom{Dimension: dimPath, MatchType: pathType, Value: pathValue}))

	if match.Method != "" {
		atoms = append(atoms, atomPredicate(Atom{Dimension: dimMethod, Value: match.Method}))
	}
	for _, header := range match.Headers {
		atoms = append(atoms, atomPredicate(Atom{
			Dimension: dimHeader, Name: header.Name, MatchType: defaultMatchType(header.Type), Value: header.Value,
		}))
	}
	for _, query := range match.QueryParams {
		atoms = append(atoms, atomPredicate(Atom{
			Dimension: dimQuery, Name: query.Name, MatchType: defaultMatchType(query.Type), Value: query.Value,
		}))
	}
	return And(atoms...)
}

// And returns the intersection of the given regions. Zero children is the universe.
func And(children ...Predicate) Predicate {
	return combine(children, true)
}

// Or returns the union of the given regions. Zero children is the empty region.
func Or(children ...Predicate) Predicate {
	return combine(children, false)
}

// Not returns the complement of p. Double negation collapses to p.
func Not(p Predicate) Predicate {
	if p.Not != nil {
		return *p.Not
	}
	return Predicate{Not: &p}
}

// String returns a deterministic canonical form used for comparison and goldens.
func (p Predicate) String() string {
	switch {
	case p.Atom != nil:
		return p.Atom.canonical()
	case len(p.All) > 0:
		return joinCanonical(p.All, " && ")
	case len(p.Any) > 0:
		return joinCanonical(p.Any, " || ")
	case p.Not != nil:
		return "!" + p.Not.String()
	}
	return ""
}

func (a Atom) canonical() string {
	switch a.Dimension {
	case dimHostname:
		return "host=" + a.Value
	case dimMethod:
		return "method=" + a.Value
	case dimPath:
		return "path:" + a.MatchType + "=" + a.Value
	default:
		return fmt.Sprintf("%s[%s]:%s=%s", a.Dimension, a.Name, a.MatchType, a.Value)
	}
}

func atomPredicate(a Atom) Predicate {
	return Predicate{Atom: &a}
}

func universe() Predicate {
	return atomPredicate(Atom{Dimension: dimPath, MatchType: matchPathPrefix, Value: "/"})
}

func defaultMatchType(matchType string) string {
	if matchType == "" {
		return matchExact
	}
	return matchType
}

func combine(children []Predicate, conjunction bool) Predicate {
	var flat []Predicate
	for _, child := range children {
		switch {
		case conjunction && len(child.All) > 0:
			flat = append(flat, child.All...)
		case !conjunction && len(child.Any) > 0:
			flat = append(flat, child.Any...)
		default:
			flat = append(flat, child)
		}
	}

	unique := make(map[string]Predicate, len(flat))
	for _, child := range flat {
		unique[child.String()] = child
	}
	keys := make([]string, 0, len(unique))
	for key := range unique {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	list := make([]Predicate, 0, len(keys))
	for _, key := range keys {
		list = append(list, unique[key])
	}

	switch {
	case len(list) == 1:
		return list[0]
	case len(list) == 0 && conjunction:
		return universe()
	case len(list) == 0:
		return Not(universe())
	case conjunction:
		return Predicate{All: list}
	default:
		return Predicate{Any: list}
	}
}

func joinCanonical(children []Predicate, separator string) string {
	parts := make([]string, 0, len(children))
	for _, child := range children {
		parts = append(parts, child.String())
	}
	return "(" + strings.Join(parts, separator) + ")"
}
