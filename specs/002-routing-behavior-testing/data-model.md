# Data Model: Routing Behavior Testing

The behavior tester has no persistent database. Its entities exist for one command
invocation and are derived from supplied feature files, manifest sets, and trace
results.

## Manifest Set

| Field | Meaning | Validation |
|---|---|---|
| name | Optional environment identifier | Unique when named; non-empty name before `=` |
| files | Ordered paths to rendered manifest documents | At least one readable input per set; names may repeat to add files |
| snapshot | Parsed Kubernetes resource snapshot | Loaded by `source.FilesSource`; duplicate resource identities rejected |

An unnamed set is supplied through repeated `-f`/`--file` flags. Named sets use
repeated `--manifests name=file` flags; repeat values with the same name append to
that set. At most one path across the full test invocation may be `-`.

## Feature and Scenario

| Field | Meaning | Validation |
|---|---|---|
| feature path | Gherkin file containing a feature | Readable, parseable, and has runnable scenarios |
| feature name | Human-readable suite grouping | Reported with scenario results |
| scenario name | Gherkin scenario title | Each scenario execution is reported |
| tags | Gherkin tags inherited from feature/rule/scenario | A tag matching a supplied named set selects that set |
| steps | Given/When/Then steps and optional table/examples data | Must match the fixed built-in vocabulary |
| example row | One expanded Scenario Outline execution | Reported separately for each row and environment |

Standard Gherkin Background steps apply to each applicable scenario. Feature
discovery is recursive and deterministic. An explicitly stated manifest set is a
consistency assertion for the environment currently executing the scenario; it
does not choose or change that environment.

## Trace Result

The trace result is the existing `model.Analysis`, with its existing nested
`EntryPoint`, `Route`, `Rule`, `Match`, `Filter`, `Destination`, `Service`,
`Endpoint`, and `Finding` values. It includes the source snapshot metadata. The
behavior layer interprets `ErrHostnameNotFound` with a populated analysis as a
normal result with zero matched routes. It does not derive or modify routing facts.

## Assertion Result

| Field | Meaning |
|---|---|
| expected | Human-readable expected value or predicate |
| actual | Value or trace summary obtained from `model.Analysis` |
| status | Passed, failed, or invalid (unsupported/ambiguous step or invalid input) |
| source location | Feature file and step location |
| suggestion | Closest registered step pattern for an undefined step, when identifiable |

Assertion messages use the shape `expected X, got Y`. Missing optional trace data is
reported as absent rather than treated as a successful default.

## Scenario Execution

A scenario execution is the pair of one expanded Gherkin scenario and one applicable
manifest set. Untagged scenarios create one execution per supplied set. A scenario
tagged with a supplied set's name runs only against that set. An explicit set step
must agree with the current execution. Each execution has one final outcome and
zero or more step-level assertion failures.

## Test Report

The console result groups executions by feature and shows each scenario and
environment status. The optional JUnit report contains one testcase per scenario
execution, preserving outline example and environment distinctions. Invalid feature
or input errors take exit-status precedence over assertion failures, while known
scenario results remain available in console output and report output where
possible.

## State and Validation Rules

1. Parse and validate command inputs before executing scenarios; never fall back to
   cluster access.
2. Load each manifest set with the shared source loader; reject more than one stdin
   path.
3. Execute applicable scenarios in a separate environment context for each set.
4. A step before a hostname trace is invalid if that step requires trace data.
5. A trace with no route is valid only as input to explicit not-found assertions;
   a positive route assertion fails with the actual absence.
6. Unsupported or ambiguous steps invalidate the run and include a closest-step
   suggestion when a candidate exists.
7. Final command status is 2 for invalid input or report errors, 1 for assertion
   failures, and 0 when all executed scenarios pass. The existing hostname command
   retains its own 0/1/2/3 mapping.
