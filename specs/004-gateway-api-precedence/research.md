# Research: Gateway API Rule Precedence

**Date**: 2026-11-07 | **Feature**: [spec.md](./spec.md)

Code inspected before planning: `internal/audit/precedence.go` (current scoring,
`rulesOverlap`, `rulesCover`), `internal/audit/audit.go` (call sites),
`internal/audit/types.go` (`Rule`, `Match`, `PrecedenceState`), and the design notes in
`specs/003-gateway-routing-audit/research.md` ("Duplicates and Gateway API Precedence").

No NEEDS CLARIFICATION items remain; all decisions below are resolved.

## Lexicographic comparator replaces additive score

- **Decision**: Replace `ruleSpecificity`/`matchSpecificity` (additive weights 60/40/20/10/12/18)
  with a lexicographic comparison over a match key tuple, compared in order:
  1. **Path type rank**: Exact > PathPrefix > RegularExpression > no-path. On HTTP routes a missing path is
     the Gateway API default PathPrefix "/", so no-path (0) only applies to gRPC matches.
  2. **Prefix length**: number of path segments, compared only between two PathPrefix
     matches (Gateway API "longest prefix" is segment-based). Exact-vs-Exact and
     regex-vs-regex are equal on this criterion regardless of value.
  3. **Method presence**: a match with a method outranks one without.
  4. **Header count**: more header matches outranks fewer.
  5. **Query param count**: more query param matches outranks fewer.
- **Rationale**: This is the normative Gateway API v1.x precedence order (FR-001). A
  lexicographic tuple cannot produce the current score's inversions (prefix+method+header
  beating Exact, two headers beating a method, multi-match sums beating a single Exact).
- **Alternatives considered**: Retune the additive weights; rejected because any additive
  scheme admits cross-criterion compensation, which the spec forbids. Compare criteria
  summed per rule; rejected by FR-002 (best match only).

## Best match per rule, never a sum

- **Decision**: Each rule is represented by its single best match, selected by applying the
  same lexicographic tuple to the rule's own matches and taking the maximum. A rule with no
  matches (catch-all) is ranked as one empty match: on HTTP routes it equals PathPrefix "/"
  (equal to a match with no path and to an explicit "/") and loses only to a longer
  prefix, a method, headers, or query params, otherwise falling through to the tie-breakers;
  on gRPC routes it maps to the zero tuple.
- **Rationale**: FR-002 and the clarification session: matches never accumulate.
- **Alternatives considered**: Compare rules match-by-match pairwise; rejected because the
  Gateway API winner is determined by the most specific matching rule, i.e. its best match.

## Tie-breakers and the undecided outcome

- **Decision**: Keep the existing tie-breaker chain unchanged: oldest creation timestamp
  (skipped when either rule has a missing or zero timestamp), then namespace/name alphabetical
  (`identityKey`), then rule index. When two overlapping rules are equal on all normative
  criteria AND all tie-breakers, the outcome is **undecided**: no winner is invented, both
  rules keep their current state, and an explicit informational finding is emitted on both
  rules (FR-005).
- **Rationale**: Matches the clarification that undecided only occurs when every
  tie-breaker is also equal; preserves determinism without fabricating a winner.
- **Alternatives considered**: Fall back to input order; rejected because it violates
  FR-010 (input-order independence).

## Centralized comparator consumed by all formats

- **Decision**: Implement one comparator in `internal/audit/precedence.go`, e.g.
  `compareRulePrecedence(left, right ruleRef) (outcome, criterion)`, returning the
  three-way outcome plus the criterion that decided it (path type, prefix length, method,
  headers, query params, creation timestamp, namespace/name, rule index, or undecided).
  `applyPrecedence` and `precedenceFindings` both consume it; findings name the competing
  rule and the deciding criterion (FR-008). Renderers (`internal/render`: text, JSON, YAML,
  Mermaid) keep reading the single `audit.Result`, so all formats present identical results
  by construction (FR-011).
- **Rationale**: One comparison implementation eliminates divergent rankings between
  state assignment and finding generation, and between output formats.
- **Alternatives considered**: Per-format rendering of precedence detail; rejected because
  it duplicates logic and risks format drift.

## Order-independent state resolution

- **Decision**: Resolve each rule's final state from the set of its pairwise outcomes, not
  from iteration order: a rule is **shadowed** if at least one higher-precedence rule both
  overlaps and covers it (`rulesCover`); else **partial-overlap** if at least one
  higher-precedence rule overlaps it; else **winner**. Undecided pairs assign no state.
- **Rationale**: The current nested-loop last-write-wins assignment can depend on input
  order when a rule is shadowed by one competitor and partially overlaps another; FR-010
  requires byte-identical results for any input permutation.
- **Alternatives considered**: Sort refs before comparing; rejected as insufficient —
  sorting fixes iteration order but pairwise overwrite semantics can still mask a shadowed
  state with a later partial-overlap.

## Overlap and coverage logic unchanged

- **Decision**: `rulesShareTraffic`, `rulesOverlap`, `rulesEquivalent`, `rulesCover`,
  `matchCovers`, `pathValuesOverlap`, `pathValuesCovered` are not modified (FR-007). Only
  the winner determination changes. GRPCRoute match fields (service/method) are not part of
  the normative tuple; GRPCRoute rules compare on the same five criteria (path/method/
  headers/query params) and the absence of GRPC-specific criteria is documented as a
  limitation (FR-009), superseding the 003 note "apply match specificity first".
- **Rationale**: Minimal blast radius; SC-005 (no outcome changes beyond the documented
  precedence corrections).
- **Alternatives considered**: Extend the tuple with GRPC service/method; rejected because
  the spec scopes this iteration to HTTPRoute-style matches.

## Testing strategy

- **Decision**:
  - **Table-driven unit tests** for the comparator: one case per criterion, per divergence
    scenario from SC-001 (Exact vs prefix+method+header, short vs long prefix, multi-match
    vs Exact, method vs two headers), per tie-breaker, regex ranking, catch-all, undecided.
  - **Permutation property tests**: for each fixture, run the audit over all (or a bounded
    random set of) permutations of input route/rule order and assert byte-identical
    serialized results (SC-002).
  - **Offline fixtures**: new manifests under `testdata/manifests/` per scenario; golden
    files for all four formats under `testdata/golden/`; update
    `testdata/features/gateway-audit-overlap.feature` for the new finding wording.
- **Rationale**: Satisfies Constitution I (table-driven, golden files, `-race`) and the
  spec's independent-test requirements.
- **Alternatives considered**: Live-cluster tests; rejected because precedence behavior is
  identical offline and live (spec assumption) and offline fixtures are deterministic.
