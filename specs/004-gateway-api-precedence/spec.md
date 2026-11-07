# Feature Specification: Gateway API Rule Precedence

**Feature Branch**: `004-gateway-api-precedence`

**Created**: 2026-11-07

**Status**: Draft

**Input**: User description: "Make Gateway API rule precedence reliable in istio-route's Gateway audit. Replace the weighted additive specificity score with a deterministic, lexicographic comparison of the normative Gateway API precedence criteria (Exact path, longest Prefix path, method, header count, query param count), compared per best match rather than summed across a rule's matches, so the audit identifies the correct winning rule, partial overlaps and shadowed rules without false positives."

## Clarifications

### Session 2026-11-07

- Q: When can the "undecided" outcome in FR-005 happen given FR-003's tie-breakers? → A: Only when all tie-breakers are also equal (identical or missing creation timestamps, same namespace/name identity, same rule index).
- Q: How is a rule's "best match" selected when it has multiple matches? → A: By applying the same lexicographic criteria (Exact > longest Prefix > method > header count > query param count) to the rule's own matches.
- Q: How should a RegularExpression path match rank in the lexicographic comparison? → A: Below Exact and PathPrefix, above no-path (which on HTTP routes is only reachable via gRPC matches, since a missing HTTP path defaults to PathPrefix "/"; preserves the current relative order; implementation-specific per the Gateway API spec).
- Q: How is "longest Prefix path" measured? → A: By number of path segments, per the Gateway API specification's definition of longest prefix match.
- Q: How does a catch-all rule (no matches) rank in the comparison? → A: On HTTP routes a missing path defaults to PathPrefix "/" (Gateway API default), so a catch-all rule, a match with no path, and an explicit PathPrefix "/" are equal on the path criteria (pathRank PathPrefix, 0 segments). A catch-all loses only to a match with a longer prefix, a method, headers, or query params; on equal keys it falls through to the tie-breakers. gRPC matches have no path and are unchanged.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Correct winner between competing rules (Priority: P1)

A platform engineer runs the Gateway audit on a set of routes whose rules compete for the same traffic. The audit ranks the rules using the normative Gateway API precedence order — Exact path match first, then longest Prefix path match, then method, then number of header matches, then number of query param matches — comparing each rule by its single best match rather than by summing criteria across all of a rule's matches. The engineer sees the rule that would actually receive the traffic identified as the winner.

**Why this priority**: This is the core defect. The current additive score produces artificial ties and inversions (e.g., a prefix rule with method and header bonuses beating an Exact path rule; a rule with two prefix matches beating a rule with one Exact match; two headers beating a method). Every other finding (partial overlap, shadowed) depends on the winner being correct.

**Independent Test**: Can be fully tested by running the audit on offline fixtures covering each precedence criterion and each documented divergence scenario, and verifying the spec-compliant rule is reported as the winner.

**Acceptance Scenarios**:

1. **Given** a rule with Exact path `/a` and a rule with PathPrefix `/` plus a method and one header, **When** the audit runs, **Then** the Exact path rule wins because path match type is compared first.
2. **Given** a rule with PathPrefix `/api` and a rule with PathPrefix `/api/v2`, **When** the audit runs, **Then** the `/api/v2` rule wins by longer prefix regardless of creation timestamp.
3. **Given** a rule with two prefix matches and a rule with one Exact match, **When** the audit runs, **Then** the Exact rule wins because rules are compared by their best match, not by a sum across matches.
4. **Given** a rule with a method match and a rule with two header matches, **When** the audit runs, **Then** the method rule wins because method is compared before header count.
5. **Given** two rules that are equal on every normative criterion, **When** the audit runs, **Then** the tie is broken by oldest creation timestamp, then namespace/name alphabetically, then rule index.

---

### User Story 2 - Trustworthy shadowed and partial-overlap findings (Priority: P2)

A platform engineer reviews audit findings to clean up dead or conflicting routing configuration. A rule is reported as shadowed only when a higher-precedence rule both wins the precedence comparison and fully covers the lower rule's match set; partial overlaps are reported distinctly. Each finding names the competing rule and states which criterion decided the outcome, so the engineer can act on findings without second-guessing false positives.

**Why this priority**: Wrong winners today cause shadowed-route-rule findings to be attached to the wrong rule, sending engineers to modify healthy configuration. Explainability turns the audit from a noisy report into a diagnostic tool.

**Independent Test**: Can be fully tested with fixtures containing a fully covered (shadowed) rule, a partially overlapping rule, and a clear winner, verifying each finding's state, wording, and identified competing rule.

**Acceptance Scenarios**:

1. **Given** a rule fully covered by a higher-precedence overlapping rule, **When** the audit runs, **Then** it is reported as shadowed and the finding names the covering rule and the deciding criterion.
2. **Given** a rule that overlaps a higher-precedence rule but is not fully covered by it, **When** the audit runs, **Then** it is reported as partial-overlap, not shadowed.
3. **Given** two overlapping rules equal on all normative criteria and all tie-breakers (identical or missing creation timestamps, same identity, same rule index), **When** the audit runs, **Then** no winner is invented and the outcome is surfaced explicitly as undecided.

---

### User Story 3 - Deterministic, consistent results everywhere (Priority: P3)

A platform engineer runs the same audit repeatedly — with resources listed in different orders by the cluster or by different manifest files — and in text, JSON, YAML, and Mermaid outputs. The ranking, findings, and states are identical in every run and every format.

**Why this priority**: Non-deterministic or format-dependent results erode trust and break automation that consumes JSON/YAML output.

**Independent Test**: Can be fully tested by running the audit over permutations of the same input resources and diffing the results, and by comparing the same audit across all four output formats.

**Acceptance Scenarios**:

1. **Given** the same set of routes in any input order, **When** the audit runs, **Then** the reported winners, shadowed rules, partial overlaps, and findings are identical.
2. **Given** one audit result, **When** it is rendered as text, JSON, YAML, and Mermaid, **Then** all four formats present the same rules, states, and findings.

---

### Edge Cases

- Two rules tie on all normative criteria and share the same creation timestamp: fall back to namespace/name alphabetical order, then rule index.
- A rule has multiple matches with different characteristics: the rule is represented by its best match only (selected by the same lexicographic criteria); matches never accumulate.
- A rule uses a RegularExpression path match: the Gateway API specification leaves regex precedence implementation-specific; regex ranks below Exact and PathPrefix and above no-path (gRPC matches only; a missing HTTP path is PathPrefix "/"), and this is documented as a limitation.
- Rules with unsupported match types or route kinds: they are excluded from precedence comparison and the limitation is documented, rather than silently misranked.
- Two rules overlap but neither covers the other and precedence is undecided (equal on all criteria and tie-breakers): both remain eligible; no shadowed finding is produced.
- A rule with no matches (catch-all) competes with a matched rule: the catch-all is ranked as PathPrefix "/" with no method, headers, or query params, so it loses to a match with a longer prefix, a method, headers, or query params, and otherwise (e.g. against an explicit PathPrefix "/") falls through to the tie-breakers, as do two catch-all rules.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The audit MUST rank competing rules by a deterministic, lexicographic comparison of the normative Gateway API precedence criteria in specification order: Exact path match, longest Prefix path match (measured by number of path segments), method, number of header matches, number of query param matches. For HTTP routes a missing path (including a rule with no matches) MUST be ranked as PathPrefix "/"; gRPC matches have no path.
- **FR-002**: Each rule MUST be compared by its single best match; criteria MUST NOT be summed or otherwise combined across a rule's matches. A rule's best match is the match that ranks highest under the same lexicographic criteria (Exact path, longest Prefix path, method, header count, query param count).
- **FR-003**: When rules are equal on all normative criteria, the audit MUST break the tie by oldest creation timestamp, then namespace/name alphabetical order, then rule index. If either rule has a missing or zero creation timestamp, the timestamp step MUST be skipped entirely (a missing timestamp is never treated as oldest) and the tie MUST be broken by namespace/name; when both rules have timestamps, the older one wins.
- **FR-004**: RegularExpression path matching MUST rank below Exact and PathPrefix and above no-path (gRPC matches only) in the lexicographic comparison (preserving the current relative order), and that handling MUST be documented as implementation-specific per the Gateway API specification.
- **FR-005**: When two overlapping rules are equal on all normative criteria AND on all tie-breakers (identical or missing creation timestamps, same namespace/name identity, same rule index), the audit MUST surface the outcome explicitly as undecided and MUST NOT invent a winner.
- **FR-006**: The audit MUST keep per-listener and per-hostname analysis and the existing rule states: winner, partial-overlap, shadowed.
- **FR-007**: Overlap and coverage logic MUST remain unchanged; a rule MUST be reported as shadowed only when both the precedence comparison and the coverage check justify it.
- **FR-008**: Findings MUST state which criterion decided the outcome and MUST identify the competing rule.
- **FR-009**: Limitations for unsupported route types or match types MUST be documented.
- **FR-010**: Audit results MUST be identical regardless of the order of input resources.
- **FR-011**: Text, JSON, YAML, and Mermaid outputs MUST present consistent results for the same audit.
- **FR-012**: Existing commands, output formats, and exit codes MUST be preserved unless a change is explicitly documented.

### Key Entities

- **Route rule**: A rule of a route resource attached to a Gateway listener/hostname; has an ordered list of matches, a creation timestamp, a namespace/name identity, and a rule index.
- **Match**: A single match clause of a rule; characterized by path match type (Exact, PathPrefix, RegularExpression) and value, method, header matches, and query param matches.
- **Precedence outcome**: The result of comparing two competing rules — left wins, right wins, or undecided — plus the criterion that decided it.
- **Rule state**: The audit classification of a rule relative to its competitors: winner, partial-overlap, or shadowed.
- **Finding**: An audit observation attached to a rule, carrying its state, the competing rule, the deciding criterion, a severity, and a suggested fix.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of the documented precedence scenarios (Exact vs. prefix+method+header, short vs. long prefix, multi-match vs. Exact, method vs. multiple headers) produce the specification-compliant winner.
- **SC-002**: Audit results are byte-identical across all permutations of input resource order for every test fixture.
- **SC-003**: Zero shadowed-rule findings are reported where the precedence comparison or the coverage check does not justify them (no false-positive shadowed findings in the test corpus).
- **SC-004**: Text, JSON, YAML, and Mermaid renderings of the same audit agree on 100% of rules, states, and findings.
- **SC-005**: Existing audit scenarios show no changes in outcome beyond the documented precedence corrections.

## Assumptions

- The target is the precedence semantics defined by the current stable Gateway API specification (v1.x) for HTTPRoute: Exact path before longest Prefix path (measured by number of path segments), then method, then header count, then query param count. If a different API version is targeted, the ordered criteria list is the single point of change.
- The route kinds and match types supported in this iteration are the ones the audit already analyzes (HTTPRoute-style matches: path, method, headers, query params). Other route kinds (e.g., GRPCRoute, TLSRoute, TCPRoute) remain unsupported for precedence and are documented as a limitation.
- "Undecided" outcomes surface as an explicit informational finding on both competing rules, leaving both in their current state; no shadowed or partial-overlap state is assigned without a decided winner.
- The earlier design note in `specs/003-gateway-routing-audit/research.md` ("apply match specificity first") is superseded by this feature: "specificity" is now defined strictly as the normative ordered criteria, not an additive score.
- Offline manifest analysis and live cluster analysis share the same precedence behavior; this feature changes only the comparison, not data collection.
