package simulate

import "strings"

// Relation describes how two traffic regions relate. Unknown means the
// algebra could not prove any other relation; callers must treat it as undetermined.
type Relation int

const (
	RelationUnknown Relation = iota
	RelationEqual
	RelationSubset
	RelationSuperset
	RelationDisjoint
)

// maxTerms bounds disjunctive normal form expansion; larger inputs are Unknown.
const maxTerms = 256

// term is a conjunction of positive atoms and negated atoms.
type term struct {
	pos []Atom
	neg []Atom
}

// Relate compares the regions described by a and b. Equal, Subset, Superset,
// and Disjoint are proven; anything else is Unknown.
func Relate(a, b Predicate) Relation {
	if a.String() == b.String() {
		return RelationEqual
	}
	ta, okA := toTerms(a, false)
	tb, okB := toTerms(b, false)
	if !okA || !okB {
		return RelationUnknown
	}
	if len(ta) == 0 || len(tb) == 0 {
		return relateEmpty(len(ta), len(tb))
	}

	sub := covers(ta, tb)
	sup := covers(tb, ta)
	switch {
	case sub && sup:
		return RelationEqual
	case sub:
		return RelationSubset
	case sup:
		return RelationSuperset
	}
	if allDisjoint(ta, tb) {
		return RelationDisjoint
	}
	return RelationUnknown
}

func relateEmpty(lenA, lenB int) Relation {
	switch {
	case lenA == 0 && lenB == 0:
		return RelationEqual
	case lenA == 0:
		return RelationSubset
	default:
		return RelationSuperset
	}
}

// covers reports whether every term of left is provably inside some term of right.
func covers(left, right []term) bool {
	for _, l := range left {
		found := false
		for _, r := range right {
			if termSubset(l, r) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func allDisjoint(left, right []term) bool {
	for _, l := range left {
		for _, r := range right {
			if !termDisjoint(l, r) {
				return false
			}
		}
	}
	return true
}

// termSubset proves l ⊆ r: each positive atom of r is implied by some positive
// atom of l, and each negated atom of r excludes l.
func termSubset(l, r term) bool {
	for _, rp := range r.pos {
		if !anyAtom(l.pos, func(lp Atom) bool {
			return atomRelation(lp, rp) == RelationEqual || atomRelation(lp, rp) == RelationSubset
		}) {
			return false
		}
	}
	for _, rn := range r.neg {
		excluded := anyAtom(l.pos, func(lp Atom) bool { return atomRelation(lp, rn) == RelationDisjoint }) ||
			anyAtom(l.neg, func(ln Atom) bool {
				return atomRelation(rn, ln) == RelationEqual || atomRelation(rn, ln) == RelationSubset
			})
		if !excluded {
			return false
		}
	}
	return true
}

// termDisjoint proves l ∩ r = ∅ from a disjoint positive pair, or from a
// positive region of one term lying inside a negated atom of the other.
func termDisjoint(l, r term) bool {
	for _, lp := range l.pos {
		if anyAtom(r.pos, func(rp Atom) bool { return atomRelation(lp, rp) == RelationDisjoint }) {
			return true
		}
	}
	for _, rn := range r.neg {
		if anyAtom(l.pos, func(lp Atom) bool {
			return atomRelation(lp, rn) == RelationEqual || atomRelation(lp, rn) == RelationSubset
		}) {
			return true
		}
	}
	for _, ln := range l.neg {
		if anyAtom(r.pos, func(rp Atom) bool {
			return atomRelation(rp, ln) == RelationEqual || atomRelation(rp, ln) == RelationSubset
		}) {
			return true
		}
	}
	return false
}

func anyAtom(atoms []Atom, match func(Atom) bool) bool {
	for _, a := range atoms {
		if match(a) {
			return true
		}
	}
	return false
}

// atomRelation compares two atoms of the same dimension. Only provable
// relations are returned; everything else is Unknown.
func atomRelation(a, b Atom) Relation {
	if a.Dimension != b.Dimension || a.Name != b.Name {
		return RelationUnknown
	}
	if a.canonical() == b.canonical() {
		return RelationEqual
	}
	switch a.Dimension {
	case dimHostname:
		return hostRelation(a.Value, b.Value)
	case dimPath:
		return pathRelation(a, b)
	case dimMethod:
		return RelationDisjoint
	default:
		if a.MatchType == matchExact && b.MatchType == matchExact {
			return RelationDisjoint
		}
		return RelationUnknown
	}
}

func hostRelation(a, b string) Relation {
	aWild, bWild := strings.HasPrefix(a, "*."), strings.HasPrefix(b, "*.")
	switch {
	case !aWild && !bWild:
		return RelationDisjoint
	case !aWild && bWild:
		if strings.HasSuffix(a, b[1:]) {
			return RelationSubset
		}
		return RelationDisjoint
	case aWild && !bWild:
		if strings.HasSuffix(b, a[1:]) {
			return RelationSuperset
		}
		return RelationDisjoint
	}
	switch {
	case strings.HasSuffix(a, b[1:]):
		return RelationSubset
	case strings.HasSuffix(b, a[1:]):
		return RelationSuperset
	}
	return RelationUnknown
}

func pathRelation(a, b Atom) Relation {
	if a.MatchType == matchExact && b.MatchType == matchExact {
		return RelationDisjoint
	}
	if a.MatchType == matchPathPrefix && b.MatchType == matchPathPrefix {
		switch {
		case prefixCovers(b.Value, a.Value):
			return RelationSubset
		case prefixCovers(a.Value, b.Value):
			return RelationSuperset
		}
		return RelationDisjoint
	}
	if a.MatchType == matchExact && b.MatchType == matchPathPrefix {
		if prefixCovers(b.Value, a.Value) {
			return RelationSubset
		}
		return RelationDisjoint
	}
	if a.MatchType == matchPathPrefix && b.MatchType == matchExact {
		if prefixCovers(a.Value, b.Value) {
			return RelationSuperset
		}
		return RelationDisjoint
	}
	return RelationUnknown
}

// prefixCovers reports whether path-prefix outer covers path p on segment boundaries,
// so "/api" covers "/api/v2" but not "/apix".
func prefixCovers(outer, p string) bool {
	outer = strings.TrimSuffix(outer, "/")
	if outer == "" {
		return true
	}
	return p == outer || strings.HasPrefix(p, outer+"/")
}

// toTerms expands p (negated when neg is true) into disjunctive normal form.
func toTerms(p Predicate, neg bool) ([]term, bool) {
	switch {
	case p.Atom != nil:
		if neg {
			return []term{{neg: []Atom{*p.Atom}}}, true
		}
		return []term{{pos: []Atom{*p.Atom}}}, true
	case p.Not != nil:
		return toTerms(*p.Not, !neg)
	case len(p.All) > 0:
		return expand(p.All, neg, !neg)
	case len(p.Any) > 0:
		return expand(p.Any, neg, neg)
	}
	return nil, false
}

// expand builds the union (conjunction: false) or intersection (true) of children.
// conjunction is the effective operator after applying negation.
func expand(children []Predicate, neg, conjunction bool) ([]term, bool) {
	if !conjunction {
		var out []term
		for _, child := range children {
			ts, ok := toTerms(child, neg)
			if !ok {
				return nil, false
			}
			out = append(out, ts...)
		}
		return out, len(out) <= maxTerms
	}
	out := []term{{}}
	for _, child := range children {
		ts, ok := toTerms(child, neg)
		if !ok {
			return nil, false
		}
		var next []term
		for _, x := range out {
			for _, y := range ts {
				next = append(next, term{
					pos: append(append([]Atom{}, x.pos...), y.pos...),
					neg: append(append([]Atom{}, x.neg...), y.neg...),
				})
			}
		}
		if len(next) > maxTerms {
			return nil, false
		}
		out = next
	}
	return out, true
}
