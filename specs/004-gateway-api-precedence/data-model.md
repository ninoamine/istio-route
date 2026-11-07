# Data Model: Gateway API Rule Precedence

**Date**: 2026-11-07 | **Feature**: [spec.md](./spec.md)

No new persisted or serialized entities. The change is confined to how existing
`internal/audit` entities are compared. Field-level definitions live in
`internal/audit/types.go`; this document captures the comparison model.

## Existing entities (unchanged shape)

- **Route** (`audit.Route`): identity (kind/namespace/name), hostnames, parent refs,
  acceptance, ordered `Rules`. Kinds: `HTTPRoute`, `GRPCRoute` (others rejected by
  validation — documented limitation, FR-009).
- **Rule** (`audit.Rule`): `Index` (1-based), `ListenerNames`, ordered `Matches`,
  `CreatedAt`, `Precedence` state. States unchanged: `winner`, `partial-overlap`,
  `shadowed` (FR-006).
- **Match** (`audit.Match`): optional `Path` (`Exact` | `PathPrefix` | `RegularExpression`
  + value), `Method`, `Headers []StringMatch`, `QueryParams []StringMatch`, optional
  `GRPC` (not part of the precedence tuple — limitation, FR-009).
- **Finding** (`model.Finding`): code, severity, message, resources, category
  (`precedence`), suggested fix. Message wording changes to name the competing rule and
  the deciding criterion (FR-008); no field changes.

## Derived comparison keys (new, internal only)

- **Match key**: lexicographic tuple derived from one `Match`:
  `(pathRank, prefixSegments, hasMethod, headerCount, queryParamCount)`
  - `pathRank`: Exact=3, PathPrefix=2, RegularExpression=1, no-path=0 (FR-004). On HTTP routes a missing path is ranked as PathPrefix "/" (PathPrefix=2, 0 segments); no-path=0 applies only to gRPC matches.
  - `prefixSegments`: segment count of the path value; significant only when both compared
    matches are PathPrefix (FR-001).
  - `hasMethod`: method string non-empty.
  - `headerCount` / `queryParamCount`: lengths of the respective slices.
- **Rule key**: the maximum match key over the rule's matches under the same lexicographic
  order; a rule with no matches (catch-all) is ranked as one empty match: on HTTP routes that equals PathPrefix "/" with no method, headers, or query params; on gRPC routes it maps to the zero key (FR-002).
- **Precedence outcome**: `left wins | right wins | undecided` plus the **deciding
  criterion** ∈ {path type, prefix length, method, headers, query params, creation
  timestamp, namespace/name, rule index, undecided} (FR-003, FR-005, FR-008).

## Validation and state rules

- Rule state resolution (order-independent): `shadowed` iff some higher-precedence rule
  overlaps and covers it; else `partial-overlap` iff some higher-precedence rule overlaps
  it; else `winner`. Undecided pairs assign no state and produce an informational finding
  on both rules (FR-005, FR-007, FR-010).
- `PrecedenceState.Validate()` unchanged; no new exported states ("undecided" is a finding,
  not a rule state).
