# Feature Specification: Routing Behavior Testing

**Feature Branch**: `002-routing-behavior-testing`

**Created**: 2026-10-30

**Status**: Draft

**Input**: User description: "Add offline Gherkin scenario testing for rendered Istio and Gateway API routing manifests, including stdin and named environments, human-readable and JUnit results, and documented exit codes."

## Clarifications

### Session 2026-10-30

- Q: When multiple named manifest sets are supplied, how should a `Given the manifests for "prod"` step interact with environment tags and untagged scenarios? → A: Tags choose the environment; an explicit `Given` must match that environment. Untagged scenarios run for every set, and an untagged scenario with an explicit environment selector is invalid when there are multiple sets.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Validate rendered routing behavior (Priority: P1)

As a chart author, I can describe the expected behavior of a hostname in a readable
scenario and run it against already-rendered manifests, so that a routing change can
be checked before deployment without access to a cluster.

**Why this priority**: Cluster-free validation of the intended route is the core
capability and can be used independently of multi-environment comparison or CI reports.

**Independent Test**: Provide a rendered manifest set and a feature file with passing
and failing assertions about a hostname. Verify the scenario result reflects the
manifest trace, with failures explaining the expected and actual values.

**Acceptance Scenarios**:

1. **Given** manifests containing an Istio or Gateway API route for a hostname and a
   feature with supported assertions, **When** the author runs the test command,
   **Then** the scenario passes when every assertion matches the trace.
2. **Given** the same manifests and a scenario with an incorrect gateway, rule, or
   destination expectation, **When** the author runs the test command, **Then** the
   scenario fails and reports the expected value and what the trace actually found.
3. **Given** a scenario asserting that a hostname has no matching route, **When** no
   route matches, **Then** the assertion passes; when a route does match, **Then** it
   fails with the actual route information.
4. **Given** rendered manifests supplied through standard input, **When** the author
   runs the test command with a feature file or directory, **Then** the manifests are
   tested without the command rendering charts or accessing a cluster.

---

### User Story 2 - Check multiple environments in CI (Priority: P2)

As a platform engineer, I can supply separately named manifest sets and tag scenarios
for the environments they describe, so that one feature suite verifies staging and
production configuration without confusing their expected values.

**Why this priority**: Named inputs let teams reuse a single suite across rendered
configuration variants while preserving the simple single-input workflow.

**Independent Test**: Supply production and staging manifest sets with different
route values. Verify that an environment-tagged scenario executes only for its
environment and an untagged scenario executes for every supplied environment.

**Acceptance Scenarios**:

1. **Given** two or more named manifest sets and scenarios tagged with a matching
   environment name, **When** the suite runs, **Then** each tagged scenario runs only
   against its named set.
2. **Given** multiple manifest sets and an untagged scenario, **When** the suite
   runs, **Then** that scenario is evaluated separately against every set.
3. **Given** a single unnamed manifest set, **When** the suite runs, **Then** the
   author can test it without assigning an environment name.

---

### User Story 3 - Diagnose and report suite results (Priority: P3)

As a CI maintainer or feature author, I can understand failed expectations and send a
machine-readable report to my CI system, so that routing regressions are actionable
and visible in automated checks.

**Why this priority**: Clear failures reduce investigation time, while a standard
report allows existing CI systems to present scenario results.

**Independent Test**: Run a suite containing passing and failing scenarios with and
without a report destination. Verify per-feature and per-scenario console results,
expected-versus-actual failure details, and a valid report containing the scenario
outcomes when requested.

**Acceptance Scenarios**:

1. **Given** a suite with multiple features and scenarios, **When** it is run, **Then**
   console output identifies each feature and scenario as passed or failed.
2. **Given** a failed assertion, **When** its result is displayed, **Then** it states
   the expected value and the corresponding actual trace value.
3. **Given** a report destination, **When** the suite finishes, **Then** a JUnit XML
   report is written with scenario results suitable for CI ingestion.
4. **Given** an unsupported or ambiguous step, **When** the feature is evaluated,
   **Then** the run reports the problem and suggests the closest supported step.
5. **Given** a suite whose scenarios all pass, **When** the test command finishes,
   **Then** it returns exit status 0; if one or more assertions fail, **Then** it
   returns exit status 1.
6. **Given** an invalid feature, unsupported or ambiguous step, unreadable input, or
   unwritable report destination, **When** the test command finishes, **Then** it
   returns exit status 2, including when scenario assertions also fail. The existing
   hostname-trace command retains its current exit statuses.

### Edge Cases

- A feature file is malformed or unreadable.
- A feature directory contains no feature files or contains an unreadable feature.
- A manifest file or standard input is empty, malformed, or unreadable.
- Standard input is requested more than once in a single run.
- A test invocation mixes unnamed `-f`/`--file` inputs with named
  `--manifests` sets.
- A scenario explicitly selects an environment that was not supplied, or its
  environment selection conflicts with its tag.
- A hostname has no matching route, or a route has no endpoint data in the manifests.
- A scenario assertion refers to a finding severity or message not present in the
  trace.
- A report destination cannot be written.
- A suite contains both scenario assertion failures and invalid input or steps.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The test command MUST consume already-rendered Kubernetes manifests
  from files or standard input and MUST NOT render charts, connect to a cluster, send
  network traffic, or mutate resources.
- **FR-002**: The existing offline `-f`/`--file` input MUST accept `-` as standard
  input for the test command and for the existing hostname-trace command, without
  changing the existing command's behavior or exit codes. A test invocation MUST
  reject requests to consume standard input more than once.
- **FR-003**: The test command MUST accept one unnamed set through `-f`/`--file`
  or multiple named sets through repeatable `--manifests <name>=<file>` arguments,
  but MUST NOT accept unnamed and named sets together. Repeated arguments with the
  same name MUST contribute files to the same set. A set MAY consist of multiple
  manifest files.
- **FR-004**: The test command MUST accept feature files and directories containing
  feature files. Directories MUST be searched recursively for feature files in a
  deterministic order. An empty feature directory, an input with no runnable
  scenarios, and unreadable inputs MUST be reported as invalid input rather than
  silently ignored.
- **FR-005**: The scenario language MUST support standard Gherkin Feature,
  Background, Scenario, Scenario Outline, Examples, and tags, using only the fixed
  built-in step vocabulary. Each expanded Scenario Outline example MUST be evaluated
  as a separate scenario execution.
- **FR-006**: The built-in steps MUST let scenarios state the expected named
  manifest set and trace a hostname using the same offline trace behavior as the
  existing command. When a scenario is run for a named set, an explicit manifest
  set step MUST match that set.
- **FR-007**: A scenario tagged with a supplied environment name MUST run only
  against that environment. An untagged scenario MUST run against every supplied
  environment. When multiple named sets are supplied, an untagged scenario MUST
  NOT select only one set through a step and an explicit set-selection step in it
  MUST be reported as invalid. A single unnamed manifest set MUST be supported.
- **FR-008**: The built-in vocabulary MUST allow assertions for all facts exposed by
  the existing hostname trace, including resource origin and identity; matching
  entry points and gateway/listener details such as class, controller, conditions,
  protocol, port, and TLS; attached route identity, order, and binding conditions;
  ordered rules, match conditions, and filters; destinations, ports, subsets,
  weights, traffic splits, and external destination details; and endpoint and
  workload readiness information when present.
- **FR-009**: The built-in vocabulary MUST allow assertions about findings by
  presence or absence, severity, and message, and MUST allow assertions that a
  hostname is or is not found.
- **FR-010**: Unsupported or ambiguous steps MUST be reported as invalid feature
  input and MUST include the closest supported step as a suggestion when one can be
  identified.
- **FR-011**: Default console output MUST report pass or fail for each feature and
  scenario. Each failed expectation MUST identify the expected result and the actual
  information found by the trace.
- **FR-012**: The test command MUST optionally write a JUnit XML report containing
  each scenario execution, including each expanded example, to a user-selected
  file. Report creation failures MUST be reported as output errors and use the
  invalid-input/output exit status.
- **FR-013**: The test command MUST document exit status 0 for all scenarios passing,
  1 when one or more scenario assertions fail, and 2 for invalid features, unknown
  or ambiguous steps, unreadable input, or report-output errors. Invalid input or
  output errors MUST take precedence when a run also has scenario failures. These
  test-command statuses MUST NOT change existing hostname-trace exit statuses 0,
  1, 2, or 3.
- **FR-014**: The README MUST explain how to run behavior tests, provide a pipeline
  example, describe supported inputs, reporting, and exit statuses. A reference page
  in the feature's spec folder MUST document the complete fixed step vocabulary and
  examples.

### Key Entities

- **Manifest set**: A collection of already-rendered resources, optionally named
  for an environment, against which scenarios are evaluated.
- **Feature**: A Gherkin document describing a routing behavior and its scenarios.
- **Scenario**: A named set of steps and expectations, optionally expanded from
  examples and selected for environments by tags.
- **Trace result**: The hostname routing facts and findings derived from one
  manifest set.
- **Assertion result**: The outcome of comparing one scenario expectation with a
  trace result, including expected and actual values when it fails.
- **Test report**: A human-readable console summary and, when requested, a JUnit XML
  representation of scenario outcomes.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A chart author can run one command against pre-rendered manifests and
  receive a pass/fail result without cluster access or chart rendering.
- **SC-002**: A suite of up to 100 scenarios evaluated against two manifest sets
  completes in five seconds or less on a representative CI runner, excluding the
  time required to render the input manifests.
- **SC-003**: Every category of fact displayed by the existing hostname trace is
  assertable using the documented built-in vocabulary, including routes, ordered
  rules, destinations, endpoint data when available, findings, and hostname absence.
- **SC-004**: Every scenario in the suite appears with a pass or fail result in
  console output and, when requested, in the JUnit report.
- **SC-005**: In a usability check, at least 90% of representative chart authors can
  identify the expected and actual routing behavior from a failure report without
  consulting raw manifests.

## Assumptions

- An environment tag is recognized as an environment selector when its name matches
  a supplied named manifest set; other tags remain ordinary scenario labels.
- Multiple matching environment tags cause the scenario to be evaluated once for
  each matching supplied environment. When a scenario is evaluated for a named
  environment, an explicit manifest-set step confirms the current set and MUST
  match it; it does not override tag selection.
- When one unnamed manifest set is supplied, scenarios run against that set; a
  named environment tag or explicit set-selection step cannot be satisfied by an
  unnamed set.
- A missing hostname is a valid trace result that can satisfy an explicit
  not-found expectation; it is not itself a test-command error.
- Scenario assertion failures are distinct from invalid feature or input errors.
  An invalid run takes exit-status precedence but still reports any known assertion
  outcomes.
- Performance is measured after manifest generation and feature-file discovery.
