# Tasks: Gateway-Scoped Routing Audit

**Input**: Design documents from `specs/003-gateway-routing-audit/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`,
`contracts/`, and `quickstart.md`

**Organization**: Tasks are grouped by the four independently testable user
stories. All tasks use the required checklist format and include the implementation
or validation file path.

## Phase 1: Setup

**Purpose**: Confirm the existing project environment can run the planned gates.

- [x] T001 Verify the Go 1.27 toolchain and configured `gofumpt`/`golangci-lint` commands against `go.mod` and `.golangci.yml` before modifying code.

## Phase 2: Foundational Types

**Purpose**: Define audit-owned result types and additive findings fields used by
every story.

- [X] T002 [P] Add table-driven serialization and validation tests for audit finding metadata in `internal/model/model_test.go`, including omission of empty audit-only fields from existing findings.
- [X] T003 Add optional `Category` and `SuggestedFix` fields to `model.Finding` in `internal/model/model.go`, with `omitempty` JSON/YAML tags so hostname trace output remains unchanged.
- [X] T004 [P] Add table-driven tests for audit result identity, category, route state, endpoint-check state, and target-found outcomes in `internal/audit/types_test.go`.
- [X] T005 Define audit request, result, route/rule, listener, waypoint-selection, endpoint-check, and typed outcome values in `internal/audit/types.go`; keep route states accepted/rejected/unknown and precedence states winner/partial-overlap/shadowed.
- [X] T006 Run the full blocking regression gate in `specs/003-gateway-routing-audit/quickstart.md`, including `gofumpt`, race tests, vet, lint, 80% internal coverage, existing binary Gherkin exit codes, and all four unchanged hostname goldens; do not begin US1 if any check fails.

## Phase 3: User Story 1 - Audit One Gateway or Waypoint (Priority: P1)

**Goal**: Resolve one Gateway API Gateway or ambient waypoint from offline or live
input, and render its scoped configuration without scanning unrelated resources.

**Independent Test**: Audit the healthy Gateway fixture and the Namespace-selected
waypoint fixture; verify target identity, class/listeners, linked routes, source
scope, endpoint-check availability, output formats, and no findings for the healthy
Gateway. Fake-client tests must prove that unrelated resources are not read.

### Tests for User Story 1

- [X] T007 [P] [US1] Add fake-client tests for Gateway, GatewayClass, GRPCRoute, Namespace, and selector-filtered Service get/list operations in `internal/kube/client_test.go`.
- [X] T008 [P] [US1] Add targeted-source tests for selected Gateway lookup, all-namespace route listing with client-side parentRef filtering, missing API families, and referenced-resource-only reads in `internal/source/gateway_test.go`.
- [X] T009 [P] [US1] Add audit-service tests for offline manifests, required namespace handling, absent/ambiguous target candidates, waypoint class recognition, and missing EndpointSlice status in `internal/audit/audit_test.go`.
- [X] T010 [P] [US1] Add table-driven CLI contract tests for `analyze` flags, exact argument count, invalid output, source errors, exit-code sentinels, and the reserved literal hostname in `internal/cli/audit_test.go`.
- [X] T011 [P] [US1] Add four-format golden tests for a healthy Gateway audit result in `internal/render/audit_test.go` and create `testdata/golden/gateway-audit-healthy.txt`, `testdata/golden/gateway-audit-healthy.json`, `testdata/golden/gateway-audit-healthy.yaml`, and `testdata/golden/gateway-audit-healthy.mmd`.

### Implementation for User Story 1

- [X] T012 [US1] Add typed get/list methods for Gateway, GatewayClass, GRPCRoute, Namespace, and label-filtered Service operations to `internal/kube/client.go`; expose only get/list operations through `kube.Reader`.
- [X] T013 [US1] Implement the targeted live Gateway snapshot path in `internal/source/gateway.go` and `internal/source/cluster.go`; discover API families, get only the selected Gateway/class, list HTTPRoutes and GRPCRoutes for client-side parentRef filtering, and fetch only referenced backends, EndpointSlices, grants, Namespaces, and Services selected by both waypoint label forms.
- [X] T014 [US1] Implement offline Gateway selection and scope filtering using `source.FilesSource` in `internal/audit/source.go`; require `--namespace` when no kubeconfig namespace is available, retain parentRef-targeting rejected/status-unknown routes, and report missing EndpointSlices as skipped.
- [X] T015 [US1] Implement audit orchestration and Gateway/waypoint/listener/route/backend result construction in `internal/audit/audit.go`; do not call the hostname-scoped `AnalysisService` or include Istio-native routing resources.
- [X] T016 [US1] Add audit-specific text, JSON, YAML, and Mermaid renderers in `internal/render/audit.go`; preserve hostname renderers and show the Gateway -> routes -> Services relationship and endpoint-check status.
- [X] T017 [US1] Add the separate Cobra `analyze` command with local flags and audit-service delegation in `internal/cli/audit.go`; register it without changing root hostname parsing or existing `test` flags in `internal/cli/root.go`.
- [X] T018 [US1] Map gateway-not-found, invalid-usage, findings-present, and runtime/source errors to exit codes 3, 2, 4, and 1 in `cmd/istio-route/main.go`, preserving root and test exit mappings.
- [X] T019 [US1] Add only required `get`/`list` permissions for Gateway and GatewayClass gets, GRPCRoutes, Namespaces, and EndpointSlices to `deploy/rbac.yaml`; retain every existing permission and do not add Secret or write access.
- [X] T020 [US1] Add the healthy gateway and Namespace-selected waypoint manifest fixtures in `testdata/manifests/gateway-audit-healthy.yaml` and `testdata/manifests/gateway-audit-waypoint.yaml`.
- [X] T021 [US1] Run the full blocking regression gate in `specs/003-gateway-routing-audit/quickstart.md`, including `gofumpt`, race tests, vet, lint, 80% internal coverage, existing binary Gherkin exit codes, and all four unchanged hostname goldens; do not begin US2 if any check fails.

## Phase 4: User Story 2 - Find Orphaned or Duplicate Routes (Priority: P1)

**Goal**: Identify rejected routes, unresolved backend Services/ports,
ReferenceGrant problems, invalid waypoint selections, and exact duplicate rules.

**Independent Test**: Audit isolated fixtures for each orphan condition and an exact
HTTPRoute/GRPCRoute duplicate; assert stable finding code, severity, category,
involved resources, explanation, suggested fix, and ordering.

### Tests for User Story 2

- [X] T022 [P] [US2] Add table-driven tests for explicit rejection versus missing Accepted status, missing backend Service/port, missing or unused ReferenceGrant, and missing/unprogrammed waypoint in `internal/audit/orphaned_test.go`.
- [X] T023 [P] [US2] Add table-driven exact-duplicate tests for listener, hostname, path, headers, method, query, and GRPC service/method matches in `internal/audit/duplicated_test.go`.
- [X] T024 [P] [US2] Add fixture-backed audit integration tests for rejected route, missing backend Service, unused ReferenceGrant, and missing waypoint label in `internal/audit/audit_test.go`.

### Implementation for User Story 2

- [X] T025 [US2] Implement the orphaned checker in `internal/audit/orphaned.go`; treat missing acceptance as unknown and report explicit rejection with reason, missing Service/port, unused or missing grant, and missing/unprogrammed waypoint with severities from `specs/003-gateway-routing-audit/spec.md`; aggregate and sort findings by severity then category in `internal/audit/audit.go`.
- [X] T026 [US2] Implement exact duplicate comparison in `internal/audit/duplicated.go`; compare route kind, listener scope, hostname, and equivalent supported match fields, including fully redundant rules.
- [X] T027 [US2] Add rejected-route, missing-backend, unused-grant, duplicate, and missing-waypoint fixtures in `testdata/manifests/gateway-audit-rejected.yaml`, `testdata/manifests/gateway-audit-missing-backend.yaml`, `testdata/manifests/gateway-audit-unused-grant.yaml`, `testdata/manifests/gateway-audit-duplicate.yaml`, and `testdata/manifests/gateway-audit-missing-waypoint.yaml`.
- [X] T028 [US2] Run the full blocking regression gate in `specs/003-gateway-routing-audit/quickstart.md`, including `gofumpt`, race tests, vet, lint, 80% internal coverage, existing binary Gherkin exit codes, and all four unchanged hostname goldens; do not begin US3 if any check fails.

## Phase 5: User Story 3 - Understand Precedence and Destination Health (Priority: P1)

**Goal**: Identify route winners, partial overlaps, fully shadowed rules, and
deficient destinations with severity reflecting whether the route can receive
traffic.

**Independent Test**: Audit overlapping and no-ready-endpoint fixtures; verify the
most-specific/oldest/name-ordered winner, partial overlap remains eligible,
full-shadow state only when the entire match space is covered, zero ready addresses,
and higher severity for a deficient winner.

### Tests for User Story 3

- [X] T029 [P] [US3] Add table-driven precedence tests for match specificity, older creation timestamp, namespace/name ties, exact duplicates, partial overlap, and full coverage by multiple higher-ranked rules in `internal/audit/precedence_test.go`.
- [X] T030 [P] [US3] Add table-driven deficient-destination tests for zero ready addresses, mixed ready/unready addresses, absent offline EndpointSlices, and winning-error versus shadowed-warning severity in `internal/audit/deficient_test.go`.
- [X] T031 [P] [US3] Add overlap and zero-ready-endpoint fixtures in `testdata/manifests/gateway-audit-overlap.yaml` and `testdata/manifests/gateway-audit-no-ready-endpoints.yaml`.

### Implementation for User Story 3

- [X] T032 [US3] Implement deterministic Gateway API match-specificity and overlap classification in `internal/audit/precedence.go`; order ties by oldest creation time then namespace/name, report partial overlap separately, and mark shadowed only when higher-precedence matches cover the full match space.
- [X] T033 [US3] Implement ready-address counting and deficient-destination findings in `internal/audit/deficient.go`; distinguish skipped offline EndpointSlice checks from observed zero-ready endpoints and assign error to a winning route and warning to a shadowed route.
- [X] T034 [US3] Include winner, partial-overlap, shadowed, and deficient states in audit rendering and Mermaid styling in `internal/render/audit.go`, without modifying `internal/render/mermaid.go` hostname output.
- [X] T035 [US3] Run the full blocking regression gate in `specs/003-gateway-routing-audit/quickstart.md`, including `gofumpt`, race tests, vet, lint, 80% internal coverage, existing binary Gherkin exit codes, and all four unchanged hostname goldens; do not begin US4 if any check fails.

## Phase 6: User Story 4 - Use the Audit in Automation and Scenario Tests (Priority: P2)

**Goal**: Audit gateways from fixed-vocabulary offline scenarios, assert findings and
precedence, and document the new command and scenario language.

**Independent Test**: Run all ten required gateway-audit scenarios; assert healthy,
rejected, missing backend, unused grant, duplicate, winner/shadowed, deficient
destination, selected waypoint, missing waypoint, and unknown gateway behavior.
Confirm each audit output format has a golden and existing behavior/JUnit output
remains unchanged.

### Tests for User Story 4

- [X] T036 [P] [US4] Add behavior-runner tests for the optional audit callback, independent audit result state, finding assertions by code/severity/category/resource and absence, route winner/shadowed assertions, and unknown-gateway assertions in `internal/behavior/steps_test.go`.
- [X] T037 [P] [US4] Add CLI behavior tests proving audit scenarios use only offline manifests and preserve existing step suggestions and JUnit XML fields in `internal/cli/test_test.go` and `internal/behavior/suggest_test.go`.
- [X] T038 [P] [US4] Add the ten requested scenarios to `testdata/features/gateway-audit-overlap.feature`, using the corresponding audit fixtures and asserting category/resource/severity/absence and winner/shadowed state.

### Implementation for User Story 4

- [X] T039 [US4] Add a narrow audit function/result dependency to `behavior.Environment` and scenario state in `internal/behavior/environments.go` and `internal/behavior/runner.go`; keep `internal/behavior` independent of `internal/cli`.
- [X] T040 [US4] Register the audit, gateway-found, finding, and precedence step patterns in `internal/behavior/steps.go`; preserve every existing pattern and closest-step suggestion behavior.
- [X] T041 [US4] Wire each offline manifest set to the audit callback in `internal/cli/test.go`; preserve existing trace callback behavior, scenario result handling, and JUnit XML format.
- [X] T042 [US4] Add audit step syntax and semantics to `specs/002-routing-behavior-testing/step-reference.md` and `specs/003-gateway-routing-audit/contracts/audit-steps.md`, including identity syntax and missing-gateway assertions.
- [X] T043 [US4] Document analyze flags, source scoping, RBAC additions, output formats, exit codes, and the reserved `analyze` hostname in `README.md`; keep existing command examples and contracts intact.
- [X] T044 [US4] Run the full blocking regression gate in `specs/003-gateway-routing-audit/quickstart.md`, including `gofumpt`; run all ten new audit scenarios, compare the four new audit golden files, and verify 100 scenarios across two manifest sets remain within five seconds.

## Phase 7: Polish and Cross-Cutting Validation

**Purpose**: Verify documentation, security scope, compatibility, and readiness after
all stories are integrated.

- [X] T045 [P] Review `deploy/rbac.yaml` against the pre-feature rules and verify changes only add necessary get/list permissions, including no Secret access or write verbs.
- [X] T046 [P] Review `README.md`, `specs/003-gateway-routing-audit/plan.md`, and `specs/003-gateway-routing-audit/quickstart.md` for consistent command flags, namespace defaults, output formats, and exit-code meanings.
- [X] T047 Run the final `go test -race ./...`, `go vet ./...`, `golangci-lint run`, formatting, 80% internal coverage, all existing binary Gherkin cases, all four existing hostname golden diffs, all new audit scenarios/goldens, and the 100-scenario/five-second performance check using `specs/003-gateway-routing-audit/quickstart.md`.

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies; verifies the existing Go toolchain and checks.
- **Foundational (Phase 2)**: Depends on setup; defines additive finding/result types and ends with the full regression gate. Blocks every user story.
- **User Story 1 (Phase 3)**: Depends on the foundation; establishes source acquisition, audit result construction, rendering, command wiring, and read permissions.
- **User Story 2 (Phase 4)**: Depends on US1 result/source fields; adds orphaned and duplicate category behavior.
- **User Story 3 (Phase 5)**: Depends on US1 route/rule/backend representation; adds precedence and endpoint health.
- **User Story 4 (Phase 6)**: Depends on US1-3 audit results; adds Gherkin integration and user documentation.
- **Polish (Phase 7)**: Depends on all four stories and includes final compatibility/security validation.

### User Story Dependencies

- **US1 (P1)**: Independent after foundational types; can be demonstrated with a healthy Gateway and a selected waypoint.
- **US2 (P1)**: Requires US1's route, backend, grant, and waypoint result data. It does not depend on US3 precedence.
- **US3 (P1)**: Requires US1's listener/rule/backend model. It is independent of US2 checker implementation.
- **US4 (P2)**: Requires the complete audit result and category findings from US1-3 so scenarios can assert stable outcomes.

### Within Each User Story

- Write table-driven tests before implementing the corresponding checker/source/CLI behavior.
- Complete one category or source increment before running that phase's full blocking regression gate.
- Do not proceed to the next phase after any test, lint, vet, golden, exit-code, or performance gate fails.

### Parallel Opportunities

- **US1**: T007-T011 can be authored in parallel because they target separate kube/source/audit/CLI/render test files; implementation tasks remain sequenced behind their relevant tests.
- **US2**: T022-T024 can be authored in parallel; the orphan and duplicate checkers touch separate files after the shared result types are stable.
- **US3**: T029-T031 can be authored in parallel; precedence and deficient-destination checkers have separate tests and implementation files.
- **US4**: T036-T038 can be authored in parallel after the audit result contract is stable; README and step-reference edits are independent of behavior code.
- **Polish**: T045 and T046 can be reviewed in parallel; T047 is the final blocking gate.

## Parallel Example: User Story 1

```text
Task: T007 kube typed-read tests in internal/kube/client_test.go
Task: T008 targeted source tests in internal/source/gateway_test.go
Task: T009 audit orchestration tests in internal/audit/audit_test.go
Task: T010 CLI contract tests in internal/cli/audit_test.go
Task: T011 audit renderer/golden tests in internal/render/audit_test.go
```

## Parallel Example: User Story 3

```text
Task: T029 precedence tests in internal/audit/precedence_test.go
Task: T030 destination health tests in internal/audit/deficient_test.go
Task: T031 overlap and endpoint manifests in testdata/manifests/
```

## Implementation Strategy

1. Establish additive model/result types and prove existing hostname/test behavior
   remains unchanged.
2. Deliver US1 as the first working increment: one target, scoped source loading,
   report rendering, and documented findings/not-found exit behavior.
3. Add orphaned/duplicate checks, then precedence/endpoint-health checks, with each
   phase gated before proceeding.
4. Add offline Gherkin audit steps and complete the required ten-case fixtures,
   scenarios, four-format goldens, and documentation.
5. Finish only after the full regression suite, exact old golden comparisons,
   read-only RBAC review, and behavior performance target pass.

**Suggested MVP**: US1, including a healthy offline Gateway audit, the read-only
targeted live path, all four render formats, and Gateway/waypoint selection. US2-US4
build on that minimal independently usable audit.
