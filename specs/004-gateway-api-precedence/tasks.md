---

description: "Task list for Gateway API Rule Precedence implementation"
---

# Tasks: Gateway API Rule Precedence

**Input**: Design documents from `/specs/004-gateway-api-precedence/`

**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Test tasks ARE included — the request explicitly requires the four divergence regression tests first (TDD: they MUST fail against the current additive scoring), plus permutation-determinism and non-regression tasks.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- Single Go module at repository root; domain logic in `internal/audit/`, rendering in `internal/render/`, CLI in `internal/cli/`
- Test fixtures in `testdata/manifests/`, goldens in `testdata/golden/`, Godog features in `testdata/features/`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish the non-regression baseline before any change (SC-005 reference point)

- [X] T001 Run `go test -race ./...` on the clean tree and confirm every existing test, golden file, and Godog feature passes; record this as the non-regression baseline for SC-005 (only the documented precedence corrections and finding wording may differ later)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

No foundational tasks required. Per plan.md, the change is confined to `internal/audit/precedence.go` (comparator) with call sites in `internal/audit/audit.go` unchanged; renderers keep consuming the single `audit.Result`. No new dependencies, no schema changes, no shared infrastructure to build.

**Checkpoint**: Baseline green - user story implementation can now begin

---

## Phase 3: User Story 1 - Correct winner between competing rules (Priority: P1) 🎯 MVP

**Goal**: Replace the additive specificity score (`ruleSpecificity`/`matchSpecificity`) with a deterministic, lexicographic comparator over each rule's best match: (path type rank, prefix length in segments, method presence, header count, query param count), followed by the existing tie-breaker chain (oldest creation timestamp, namespace/name alphabetical, rule index).

**Independent Test**: Run `go test -race ./internal/audit/ -run 'Precedence' -v` and verify each precedence criterion and each documented divergence scenario reports the spec-compliant winner.

### Tests for User Story 1 ⚠️

> **NOTE: Write these tests FIRST, run them, and confirm the four divergence tests (T002-T005) FAIL against the current additive scoring in `internal/audit/precedence.go` before implementing the comparator.**

- [X] T002 [US1] Add divergence regression test in `internal/audit/precedence_test.go`: a rule with Exact path `/a` MUST win over a rule with PathPrefix `/` plus a method and one header, because path match type is compared first (spec US1 scenario 1, SC-001)
- [X] T003 [US1] Add divergence regression test in `internal/audit/precedence_test.go`: a rule with PathPrefix `/api/v2` MUST win over a rule with PathPrefix `/api` by longer prefix (measured in path segments) regardless of creation timestamp (spec US1 scenario 2, SC-001)
- [X] T004 [US1] Add divergence regression test in `internal/audit/precedence_test.go`: a rule with one Exact match MUST win over a rule with two prefix matches, because rules are compared by their single best match and criteria MUST NOT be summed across matches (spec US1 scenario 3, FR-002, SC-001)
- [X] T005 [US1] Add divergence regression test in `internal/audit/precedence_test.go`: a rule with a method match MUST win over a rule with two header matches, because method is compared before header count (spec US1 scenario 4, SC-001)
- [X] T006 [US1] Add table-driven comparator tests in `internal/audit/precedence_test.go` covering: query param count as last normative criterion; RegularExpression ranking below Exact and PathPrefix and above no-path (FR-004); catch-all rule (no matches), match without path and explicit PathPrefix `/` being equal on the path criterion on HTTP routes (a catch-all falls through to tie-breakers against PathPrefix `/`, and loses to a longer prefix, a method, headers or query params), two catch-alls falling through to tie-breakers, and gRPC ranking unchanged; tie-breaker chain in order — oldest creation timestamp (when either rule has a missing or zero timestamp the timestamp step is skipped and namespace/name decides; when both are present the older wins), then namespace/name alphabetical, then rule index (FR-003); undecided when all criteria AND all tie-breakers are equal (FR-005)

### Implementation for User Story 1

- [X] T007 [US1] Implement the lexicographic comparison keys in `internal/audit/precedence.go`: a match key tuple `(pathRank, prefixSegments, hasMethod, headerCount, queryParamCount)` with `pathRank` Exact=3, PathPrefix=2, RegularExpression=1, no-path=0 (HTTP: a missing path is ranked as PathPrefix `/`; no-path applies to gRPC only) and `prefixSegments` (segment count of the path value) significant only when both compared matches are PathPrefix; a rule key as the maximum match key over the rule's matches under the same lexicographic order, with a no-match catch-all rule ranked as one empty match (PathPrefix `/` on HTTP routes, zero key on gRPC routes); delete `ruleSpecificity` and `matchSpecificity` (FR-001, FR-002, FR-004)
- [X] T008 [US1] Rewrite `compareRulePrecedence` in `internal/audit/precedence.go` to compare rule keys lexicographically and then apply the tie-breaker chain, returning `(outcome, criterion)` where outcome is `left wins | right wins | undecided` and criterion is one of `path type`, `prefix length`, `method`, `headers`, `query params`, `creation timestamp`, `namespace/name`, `rule index`, `undecided` (FR-001, FR-003, FR-005, FR-008); update the `hasOverlappingRule` call site in the same file
- [X] T009 [US1] Rework `applyPrecedence` in `internal/audit/precedence.go` to resolve each rule's final state from the set of its pairwise outcomes instead of nested-loop last-write-wins: `shadowed` iff at least one higher-precedence rule both overlaps and covers it (`rulesCover`), else `partial-overlap` iff at least one higher-precedence rule overlaps it, else `winner`; undecided pairs assign no state; do not modify `rulesShareTraffic`, `rulesOverlap`, `rulesEquivalent`, `rulesCover`, `matchCovers`, `pathValuesOverlap`, `pathValuesCovered` (FR-006, FR-007, FR-010)

**Checkpoint**: `go test -race ./internal/audit/` passes; the four divergence tests now report the spec-compliant winner - User Story 1 is fully functional and testable independently

---

## Phase 4: User Story 2 - Trustworthy shadowed and partial-overlap findings (Priority: P2)

**Goal**: Findings name the competing rule and state which criterion decided the outcome; shadowed is reported only when a higher-precedence rule both wins and fully covers; equal-on-everything overlapping rules surface an explicit undecided informational finding instead of an invented winner.

**Independent Test**: Run `go test -race ./internal/audit/ -run 'Finding' -v` with fixtures/cases containing a fully covered (shadowed) rule, a partially overlapping rule, and an undecided pair, verifying each finding's state, wording, and identified competing rule.

### Tests for User Story 2 ⚠️

> **NOTE: Write these tests FIRST and ensure they FAIL before implementation (current messages name neither the competing rule nor the deciding criterion; no undecided finding exists).**

- [X] T010 [US2] Add finding-wording tests in `internal/audit/precedence_test.go`: a shadowed finding MUST name the covering rule as `<kind> <namespace>/<name> rule <index>` and the deciding criterion; a partially overlapping rule MUST be reported as `partial-overlap-route-rule`, not shadowed; a winning finding MUST name the deciding criterion; codes (`shadowed-route-rule` warning, `partial-overlap-route-rule` info, `winning-route-rule` info), category (`precedence`), and suggested fixes stay unchanged (FR-008, spec US2 scenarios 1-2)
- [X] T011 [US2] Add an undecided-outcome test in `internal/audit/precedence_test.go`: two overlapping rules equal on all normative criteria AND all tie-breakers (identical or missing creation timestamps, same namespace/name identity, same rule index) MUST produce no winner, no shadowed/partial-overlap state, and an `undecided-route-rule-overlap` informational finding on BOTH rules (FR-005, spec US2 scenario 3)

### Implementation for User Story 2

- [X] T012 [US2] Update `precedenceFindings` in `internal/audit/precedence.go` so every precedence finding message names the competing rule (`<kind> <namespace>/<name> rule <index>`) and the criterion that decided the outcome, consuming the `(outcome, criterion)` comparator result; keep finding codes, severities, category, suggested fixes, and the JSON/YAML schema (same fields, same `omitempty` behavior) unchanged (FR-008, contracts/cli.md)
- [X] T013 [US2] Emit the new `undecided-route-rule-overlap` finding (severity info, category `precedence`) on both rules of an undecided overlapping pair in `internal/audit/precedence.go`, and guarantee no shadowed or partial-overlap state is assigned without a decided winner (FR-005, FR-007)

**Checkpoint**: `go test -race ./internal/audit/` passes; findings are explainable and undecided pairs are explicit - User Story 2 is independently testable

---

## Phase 5: User Story 3 - Deterministic, consistent results everywhere (Priority: P3)

**Goal**: Identical ranking, findings, and states across any input order and across text, JSON, YAML, and Mermaid outputs.

**Independent Test**: Run `go test -race ./internal/audit/ -run 'Permutation' -v` (byte-identical results across input permutations) and `go test -race ./internal/render/... ./internal/cli/...` (four-format golden consistency).

### Tests for User Story 3 ⚠️

- [X] T014 [P] [US3] Create offline fixture manifests `testdata/manifests/gateway-audit-precedence-exact-vs-prefix.yaml`, `testdata/manifests/gateway-audit-precedence-prefix-length.yaml`, `testdata/manifests/gateway-audit-precedence-best-match.yaml`, `testdata/manifests/gateway-audit-precedence-method-vs-headers.yaml`, `testdata/manifests/gateway-audit-precedence-shadowed.yaml`, `testdata/manifests/gateway-audit-precedence-partial-overlap.yaml`, and `testdata/manifests/gateway-audit-precedence-undecided.yaml`, each with a Gateway and the HTTPRoutes for its scenario (mirroring the style of `testdata/manifests/gateway-audit-overlap.yaml`)
- [X] T015 [US3] Add a permutation property test in a new file `internal/audit/precedence_permutation_test.go`: for every `testdata/manifests/gateway-audit-precedence-*.yaml` fixture, run the audit over all permutations (or a bounded random set with a fixed seed) of input route/rule order and assert byte-identical serialized results (SC-002, FR-010)
- [X] T016 [US3] Add a cross-format consistency golden test in `internal/render/audit_test.go` rendering one precedence fixture audit as text, JSON, YAML, and Mermaid against new goldens `testdata/golden/gateway-audit-precedence.txt`, `.json`, `.yaml`, and `.mmd`, asserting all four formats present the same rules, states, and findings (SC-004, FR-011)

### Implementation for User Story 3

- [X] T017 [US3] Make audit result construction fully order-independent wherever T015 surfaces dependence (for example deterministic ordering of `Findings` assembled in `internal/audit/audit.go` and `internal/audit/precedence.go`), without changing overlap/coverage logic (FR-007, FR-010)
- [ ] T018 [US3] Non-regression sweep: run `go test -race ./...` including the Godog suite in `internal/cli/test_test.go` and confirm versus the T001 baseline that existing audit scenarios show no outcome changes beyond the documented precedence corrections and finding wording (SC-005); update `testdata/features/gateway-audit-overlap.feature` only where the new finding wording or corrected winners require it

**Checkpoint**: Permutation and cross-format tests pass; only documented diffs versus baseline - User Story 3 is independently testable

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Documentation and quality gates affecting the whole feature

- [ ] T019 [P] Document the precedence limitations in `README.md`: RegularExpression path matches rank below Exact/PathPrefix and above no-path (implementation-specific per the Gateway API specification, FR-004); GRPCRoute rules are compared on path/method/header/query-param criteria only (GRPC service/method fields do not influence precedence); route kinds other than HTTPRoute/GRPCRoute are excluded from precedence comparison (FR-009, contracts/cli.md)
- [ ] T020 Document the normative lexicographic precedence order (Exact path, longest Prefix path by segments, method, header count, query param count, then creation timestamp, namespace/name, rule index), the finding wording change naming the competing rule and deciding criterion, and the new `undecided-route-rule-overlap` info code in `README.md` (FR-008, FR-012)
- [ ] T021 Run the quality gates from the repository root: `gofumpt -l .`, `go vet ./...`, `golangci-lint run`, and `go test -race ./...` must all pass, with `internal/` coverage at or above 80% (Constitution I)
- [ ] T022 Run every `specs/004-gateway-api-precedence/quickstart.md` validation scenario end-to-end (unit tests, permutation tests, four-format CLI runs over the new fixtures with exit code 0, golden files, quality gates) and confirm the expected results

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: No tasks; Phase 1 baseline is the only gate - BLOCKS all user stories
- **User Story 1 (Phase 3)**: Depends on Phase 1 baseline only
- **User Story 2 (Phase 4)**: Depends on User Story 1 - finding wording consumes the comparator's `(outcome, criterion)` result from T008
- **User Story 3 (Phase 5)**: Depends on User Stories 1 AND 2 - permutation and cross-format tests assert the final comparator behavior and final finding wording; fixture creation (T014) is the exception and can start any time after Phase 1
- **Polish (Phase 6)**: Depends on all user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Setup (Phase 1) - No dependencies on other stories
- **User Story 2 (P2)**: Requires US1's comparator signature; independently testable once US1 lands
- **User Story 3 (P3)**: Requires US1+US2 behavior to be final; independently testable once they land

### Within Each User Story

- Tests MUST be written and FAIL before implementation (the four divergence tests T002-T005 fail against the current additive scoring)
- Comparison keys (T007) before the comparator (T008) before state resolution (T009)
- Story checkpoint green before moving to the next priority

### Parallel Opportunities

- T014 (fixtures) can run in parallel with all of Phase 3 and Phase 4 (different files, no code dependencies)
- T019 can run in parallel with T020 only if applied as separate non-overlapping edits; otherwise sequence them (both edit `README.md`)
- All test tasks within a story share `internal/audit/precedence_test.go` and MUST run sequentially (same file)
- No user story can start in parallel with another in this feature: US2 needs the US1 comparator, US3 needs final US1+US2 behavior

---

## Parallel Example: User Story 3

```bash
# Fixture creation (T014) in parallel with User Story 1/2 code work:
Task: "Create offline fixture manifests testdata/manifests/gateway-audit-precedence-*.yaml"

# After US1+US2 land, the two new test files are independent:
Task: "Add permutation property test in internal/audit/precedence_permutation_test.go"
Task: "Add cross-format consistency golden test in internal/render/audit_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (baseline green)
2. Complete Phase 3: User Story 1 - divergence tests fail, then comparator makes them pass
3. **STOP and VALIDATE**: `go test -race ./internal/audit/` - the correct winner is identified for every precedence criterion
4. Deploy/demo if ready: the core defect is fixed

### Incremental Delivery

1. Setup → baseline captured
2. User Story 1 → correct winners (MVP) → validate independently
3. User Story 2 → explainable findings + explicit undecided → validate independently
4. User Story 3 → determinism + format consistency + non-regression → validate independently
5. Polish → docs + quality gates + quickstart validation
6. Each story adds value without breaking previous stories

### Parallel Team Strategy

Not recommended for this feature beyond T014: the change is confined to one package and US2/US3 build directly on US1's comparator. A second contributor can own the T014 fixtures and the T019/T020 documentation while the comparator lands.

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story is independently completable and testable at its checkpoint
- Verify the four divergence tests FAIL before implementing the comparator (TDD gate)
- Commit after each task or logical group
- Stop at any checkpoint to validate the story independently
- Overlap/coverage logic (`rulesShareTraffic`, `rulesOverlap`, `rulesEquivalent`, `rulesCover`, `matchCovers`, `pathValuesOverlap`, `pathValuesCovered`) MUST NOT be modified (FR-007)
- CLI surface (commands, `--output` formats, exit codes 0/1/2/3) MUST NOT change (FR-012)
- Avoid: vague tasks, same file conflicts, cross-story dependencies that break independence
