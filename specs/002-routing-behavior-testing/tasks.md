# Tasks: Routing Behavior Testing

**Input**: Design documents from `specs/002-routing-behavior-testing/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/](./contracts/)

**Tests**: Test tasks are included because the feature specification and supplied technical context explicitly require unit tests, fixture-backed integration tests, and stdin coverage.

**Organization**: Tasks are grouped by the three prioritized user stories. Each checklist item has a sequential ID, optional parallel marker, story label for story phases, and an exact target path.

## Phase 1: Setup

**Purpose**: Add the selected Gherkin dependency to the existing Go CLI project.

- [X] T001 Add `github.com/cucumber/godog` v0.16.0 to `go.mod` and update `go.sum`; keep the Go module version and existing dependencies unchanged.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Extend the shared offline input path so both the existing trace command and the new behavior runner can consume stdin without duplicating YAML decoding.

- [X] T002 [P] Add table-driven tests in `internal/source/files_test.go` proving `FilesSource` decodes multiple YAML documents from an injected reader for path `-`, reports reader failures, and observes context cancellation.
- [X] T003 Extend `source.FilesSource` in `internal/source/files.go` with an injectable `io.Reader` for `-`; route stdin and file contents through the same multi-document decoder and preserve file-based duplicate-resource validation.
- [X] T004 Add CLI tests in `internal/cli/analyze_test.go` proving the existing offline analysis receives stdin through its request and still returns its existing hostname-not-found sentinel and structured analysis.
- [X] T005 Extend `cli.Flags` and `defaultAnalysisService` in `internal/cli/root.go` and `internal/cli/analyze.go` to pass an injected stdin reader to `FilesSource`; preserve existing hostname invocation, default `os.Stdin` behavior, and existing exit statuses.

**Checkpoint**: Shared offline input accepts injected stdin; existing file and live-cluster tracing behavior remains unchanged.

---

## Phase 3: User Story 1 - Validate Rendered Routing Behavior (Priority: P1) 🎯 MVP

**Goal**: Run supported Gherkin scenarios against one offline manifest set and assert structured hostname-trace results, including explicit route absence.

**Independent Test**: Run passing and failing feature files against rendered Istio/Gateway API fixtures and stdin; confirm results reflect the existing trace, show expected versus actual values, and never initialize a cluster client.

### Tests for User Story 1

- [X] T006 [P] [US1] Add table-driven unit tests in `internal/behavior/fields_test.go` for resolving scalar, object, and indexed-list paths against `model.Analysis`; verify invalid paths and missing values fail explicitly.
- [X] T007 [P] [US1] Add table-driven unit tests in `internal/behavior/steps_test.go` for hostname-found/not-found assertions, gateway and route identity, ordered rule match, destination service/port, weighted split, and finding assertions using structured trace fixtures.
- [X] T008 [P] [US1] Add a passing Gherkin fixture in `testdata/features/routing-passes.feature` and matching rendered routing resources in `testdata/manifests/behavior.yaml`, including entry point, route, rules, weighted destinations, a finding, a Background step, and a Scenario Outline with at least two Examples rows.
- [X] T009 [P] [US1] Add a failing Gherkin fixture in `testdata/features/routing-fails.feature` whose incorrect route expectation exercises an `expected X, got Y` diagnostic from the actual fixture trace.

### Implementation for User Story 1

- [X] T010 [US1] Implement structured trace-field lookup in `internal/behavior/fields.go`; support dot-separated model fields and list indexes, preserve one-based entity ordinals in steps, and report absent data instead of substituting defaults.
- [X] T011 [US1] Implement scenario context and an injected trace function in `internal/behavior/runner.go`; retain populated `model.Analysis` when the trace returns the hostname-not-found sentinel so absence assertions can evaluate it.
- [X] T012 [US1] Implement the fixed single-set step definitions in `internal/behavior/steps.go` for tracing, found/not-found, matching entry points, route ordering, rule matches, destinations and weights, endpoints, and findings; format failures as `expected X, got Y` using structured results.
- [X] T013 [US1] Add the `test` Cobra child command in `internal/cli/test.go` and register it in `internal/cli/root.go`; accept repeatable `-f`/`--file`, `--junit`, and feature file/directory arguments without changing the existing positional hostname invocation.
- [X] T014 [US1] Wire `internal/cli/test.go` to invoke `internal/behavior` with only offline manifest files and the injected command stdin; reject missing inputs, unreadable feature paths, empty feature directories, repeated stdin, and mixed unnamed/named sets without creating a cluster source.
- [X] T015 [US1] Add command status sentinels and map test assertion failures to status 1 and invalid test inputs to status 2 in `internal/cli/test.go` and `cmd/istio-route/main.go`, while retaining hostname trace mappings 0/1/2/3.
- [X] T016 [US1] Add fixture-backed command integration tests in `internal/cli/test_test.go` invoking `test -f <fixture> <feature>`; verify a passing scenario returns 0, a failing assertion returns 1 with expected/actual output, Background runs for each scenario, each Outline example is independently reported, and a `-f -` run consumes injected stdin without cluster access.

**Checkpoint**: One unnamed manifest set can execute the P1 feature suite offline, including stdin and explicit hostname-not-found assertions.

---

## Phase 4: User Story 2 - Check Multiple Environments in CI (Priority: P2)

**Goal**: Evaluate tagged scenarios only against their named manifest set and repeat untagged scenarios for each supplied set.

**Independent Test**: Supply production and staging fixtures with different routes; confirm environment-tagged scenarios execute only for a matching set, untagged scenarios execute for both, and explicit manifest assertions only confirm the current set.

### Tests for User Story 2

- [ ] T017 [P] [US2] Add table-driven environment-selection tests in `internal/behavior/environments_test.go` covering matching tags, unmatched tags, untagged scenarios across all sets, repeated files per set, duplicate/empty names, unknown environments, and conflicting explicit manifest assertions.
- [ ] T018 [P] [US2] Add production and staging manifest fixtures in `testdata/manifests/behavior-prod.yaml` and `testdata/manifests/behavior-staging.yaml`, with deliberately different host routing or destination values.
- [ ] T019 [P] [US2] Add Gherkin fixtures in `testdata/features/environments.feature` covering `@prod`, `@staging`, and untagged scenarios, plus a matching explicit manifest-set assertion for each tagged scenario.

### Implementation for User Story 2

- [ ] T020 [US2] Implement manifest-set parsing and validation in `internal/behavior/environments.go`; group repeated `name=file` values, require non-empty names and paths, reject duplicate incompatible definitions, reject unnamed/named mixtures, and reject more than one stdin path.
- [ ] T021 [US2] Implement per-environment Godog tag expressions in `internal/behavior/runner.go` so `@name` scenarios run only for that named set, scenarios without any supplied environment tag run for every set, and non-environment tags remain ordinary filters.
- [ ] T022 [US2] Implement the explicit `Given the manifests for "name"` confirmation step in `internal/behavior/steps.go`; require it to match the current execution and reject unknown or conflicting names without switching manifest context.
- [ ] T023 [US2] Add a multi-environment integration test in `internal/cli/test_test.go` using both named fixture sets and `testdata/features/environments.feature`; assert per-environment execution counts and outcomes for tagged and untagged scenarios.

**Checkpoint**: One invocation can verify environment-specific and shared expectations without cross-environment state leakage.

---

## Phase 5: User Story 3 - Diagnose and Report Suite Results (Priority: P3)

**Goal**: Provide actionable per-scenario console output, closest-step suggestions, a single JUnit report across executions, and documented CI usage and exit behavior.

**Independent Test**: Run a suite with pass, assertion-failure, undefined-step, and environment-expanded scenarios; verify console summaries, expected/actual details, suggestions, the combined JUnit testcase set, and status precedence.

### Tests for User Story 3

- [ ] T024 [P] [US3] Add table-driven matcher tests in `internal/behavior/suggest_test.go` for undefined and ambiguous steps, exact and near matches, stable tie handling, and no suggestion when supported patterns are unrelated.
- [ ] T025 [P] [US3] Add JUnit writer tests in `internal/behavior/report_test.go` that decode generated XML and verify one testcase per scenario/example/environment execution, failures and errors, and correct counts.
- [ ] T026 [P] [US3] Add CLI tests in `internal/cli/test_test.go` for exit status 2 on malformed feature, unknown/ambiguous step, unreadable manifest, and unwritable report; verify invalid-input status takes precedence over assertion failures and old hostname exit codes are unchanged.

### Implementation for User Story 3

- [ ] T027 [US3] Implement a Godog formatter/event collector in `internal/behavior/formatter.go` that records feature, expanded scenario, step, failure, undefined-step, and ambiguous-step events with source locations across repeated environment runs.
- [ ] T028 [US3] Implement closest-supported-step suggestions in `internal/behavior/suggest.go` using the fixed pattern catalog from `internal/behavior/steps.go`; include candidate text in undefined- and ambiguous-step diagnostics and avoid suggesting unrelated patterns.
- [ ] T029 [US3] Implement human-readable per-feature and per-scenario reporting in `internal/behavior/report.go`; show environment when applicable and include each failed step's expected and actual trace information.
- [ ] T030 [US3] Implement a combined JUnit XML writer in `internal/behavior/report.go` with one testcase for every scenario execution, including outline examples and environment runs; propagate create, encode, and write errors to the CLI.
- [ ] T031 [US3] Wire formatter results and `--junit` output through `internal/cli/test.go`; write results to stdout, diagnostics to stderr, and return status 2 for invalid inputs/report errors before considering assertion-failure status 1.
- [ ] T032 [US3] Document `istio-route test`, single and named manifest inputs, stdin, environment tags, step vocabulary, JUnit output, and test-only exit codes in `README.md`; include GitHub Actions and GitLab CI artifact-publication examples in `specs/002-routing-behavior-testing/quickstart.md` and the complete fixed patterns in `specs/002-routing-behavior-testing/step-reference.md`.

**Checkpoint**: CI can ingest one combined report and users can identify both actual routing behavior and the closest supported step when a feature is invalid.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Validate the complete feature, including cross-story behavior and repository quality gates.

- [ ] T033 [P] Add an end-to-end performance test or benchmark in `internal/behavior/runner_test.go` for 100 scenarios evaluated against two manifest sets; measure scenario execution separately from manifest generation and feature discovery, and verify it completes within five seconds on a representative CI runner.
- [ ] T034 Run `gofumpt` on changed Go files in `internal/behavior/*.go`, `internal/cli/*.go`, `internal/source/*.go`, and `cmd/istio-route/*.go`; verify `go test -race ./...`, `go vet ./...`, and `golangci-lint run` and resolve any failures in those files.
- [ ] T035 Run every command and expected outcome in `specs/002-routing-behavior-testing/quickstart.md`; correct the quickstart and `README.md` if the actual CLI differs.
- [ ] T036 Measure `internal/` statement coverage with `go test -coverprofile=/tmp/istio-route-internal.cover` and `go tool cover`; verify coverage remains at or above the constitution's 80% minimum and add focused coverage tests in the relevant `internal/` package test files if it does not.
- [ ] T037 Add a golden console-report fixture in `testdata/golden/behavior-report.txt` and a comparison test in `internal/behavior/report_test.go`; verify stable per-feature and per-scenario output, including an expected-versus-actual failure.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies; Godog must be available before runner implementation.
- **Foundational (Phase 2)**: Depends on Setup; shared stdin support must be complete before the test command consumes stdin.
- **User Story 1 (Phase 3)**: Depends on Foundational; delivers the single-set MVP and core trace assertions.
- **User Story 2 (Phase 4)**: Depends on the P1 runner and step context in User Story 1.
- **User Story 3 (Phase 5)**: Depends on the scenario execution and environment result flow from User Stories 1 and 2.
- **Polish (Phase 6)**: Depends on all selected stories.

### User Story Dependencies

- **US1 (P1)**: Starts after Phase 2. No dependency on later stories.
- **US2 (P2)**: Requires the core scenario runner from US1; named sets and filtering extend its execution context.
- **US3 (P3)**: Requires scenario events and environment execution results from US1/US2 so it can produce complete console and JUnit reports.

### Parallel Opportunities

- **Setup**: T001 is independent.
- **Foundational**: T002 can be implemented alongside T004; T003 follows T002 and T005 follows T004.
- **US1 tests/fixtures**: T006, T007, T008, and T009 target distinct files and can be prepared in parallel; implementation starts after relevant tests exist.
- **US2 tests/fixtures**: T017, T018, and T019 target distinct files and can be prepared in parallel.
- **US3 tests**: T024, T025, and T026 target distinct files and can be prepared in parallel after the prior story interfaces stabilize.
- **Polish**: T033 can be prepared independently of docs validation; T034, T035, T036, and T037 run after implementation. T037's golden fixture/test can be developed independently of T036's coverage measurement.

### Parallel Example: User Story 1

```text
Task: T006 structured field lookup tests in internal/behavior/fields_test.go
Task: T007 step assertion tests in internal/behavior/steps_test.go
Task: T008 passing feature/manifests in testdata/features/routing-passes.feature and testdata/manifests/behavior.yaml
Task: T009 failing feature in testdata/features/routing-fails.feature
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 setup and Phase 2 stdin/trace foundation.
2. Implement Phase 3 for one unnamed manifest set, including structured trace
   assertions, passing/failing fixtures, and stdin integration.
3. Stop and validate US1 independently with its acceptance scenarios and race tests.
4. Deliver the single-set offline workflow before adding multi-environment behavior.

### Incremental Delivery

1. Setup plus foundation establishes the shared offline data path.
2. US1 delivers a useful cluster-free Gherkin trace validator.
3. US2 adds named manifest sets and tag-filtered repeated execution.
4. US3 adds actionable suggestions, consolidated reports, and CI documentation.
5. Run the cross-cutting quality gates and quickstart after the desired stories are complete.

### Execution Notes

- Tasks are ordered by dependency; `[P]` marks only work that can proceed on separate files without waiting on another incomplete task.
- Tests and fixtures are explicit because they are required by the supplied feature requirements and technical context.
- Keep all Kubernetes analysis in the existing source/resolver/diagnosis path. The behavior runner receives a trace function and must never initialize a cluster client.
