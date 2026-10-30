# Research: Routing Behavior Testing

## Godog embedding and execution

**Decision**: Embed Godog v0.16.0 in the `istio-route` binary and invoke the
library-level `godog.TestSuite{Options: ...}.Run()` API from the behavior runner.
Use `Options.Paths` for user-provided feature files, `Options.Tags` for selecting
scenarios per environment, `Options.Strict` so undefined and ambiguous steps fail,
and `Options.Format`/`Options.Output` for result formatting. There is no need to
embed user feature files in the executable; `embed.FS` is an optional Godog input
mechanism, not the user-facing source.

**Rationale**: The binary must accept user-authored files at runtime, while Gherkin
parsing and execution should be embedded with no external executable. Godog
provides an in-process runner and the standard language features requested by the
spec. Version 0.16.0 supports Go 1.18+, compatible with this project's Go 1.27
module.

**Alternatives considered**:

- Shell out to the `godog` executable: rejected because the feature must ship in a
  single CLI binary and work in CI without installing another tool.
- Implement Gherkin parsing and execution in the project: rejected because it
  duplicates a mature runner and increases language-compatibility risk.

**Sources**:

- [Godog v0.16.0 release](https://github.com/cucumber/godog/releases/tag/v0.16.0)
- [Godog README: tags and embedded feature files](https://github.com/cucumber/godog/blob/v0.16.0/README.md)
- [Godog TestSuite API](https://github.com/cucumber/godog/blob/v0.16.0/run.go)
- [Godog Options](https://github.com/cucumber/godog/blob/v0.16.0/internal/flags/options.go)

## Environment tag filtering

**Decision**: Run the feature suite once per manifest set. For named environment
`prod`, select scenarios tagged `@prod` plus scenarios without any supplied
environment tag; repeat for each environment. An explicit manifest-set step
validates the current execution's set and does not override the tag filter. Use
Godog's tag-expression grammar, not custom parsing of scenario text.

**Rationale**: A scenario can assert against one stable manifest set per run.
Untagged scenarios are repeated for each set, while environment-tagged scenarios
are excluded from other environments. Repeated suite execution also makes each
environment's scenario outcomes independently reportable.

**Alternatives considered**:

- Choose an environment from a step at runtime: rejected because an untagged
  scenario is required to run against every set, and steps cannot reliably clone
  a scenario execution.
- Run all scenarios against one mutable environment context: rejected because
  changing the context during execution can leak state between scenarios.

## Undefined-step suggestions

**Decision**: Register a custom Godog formatter that records each `Undefined`
callback with scenario context. Compare the step text with the fixed supported
step-pattern catalog and print the closest candidates in the invalid-input
diagnostic.

**Rationale**: Godog's public formatter interface reports undefined steps, while
its built-in snippets generate new-step scaffolding rather than ranking similar
supported steps. Capturing events preserves Godog's own parser and strict failure
semantics; a bounded matcher over the project's own catalog can add actionable
suggestions without enabling custom steps.

**Alternatives considered**:

- Use Godog's generated snippets as suggestions: rejected because they show
  implementation templates, not the closest supported vocabulary item.
- Ignore suggestions: rejected because FR-010 requires a closest-step suggestion
  when identifiable.

**Sources**:

- [Godog Formatter interface](https://github.com/cucumber/godog/blob/v0.16.0/formatters/fmt.go)
- [Godog formatter registration](https://github.com/cucumber/godog/blob/v0.16.0/fmt.go)
- [Godog built-in snippet generation](https://github.com/cucumber/godog/blob/v0.16.0/internal/formatters/fmt_base.go)

## Human-readable output and JUnit aggregation

**Decision**: Use Godog's human-readable formatter for each environment execution.
Use a small project-owned JUnit XML aggregator for the final `--junit` path, with
one test case per scenario execution (including each outline example and each
environment run).

**Rationale**: Godog's built-in JUnit formatter is suitable for a single suite,
but the feature intentionally evaluates scenarios in multiple separate
environment-filtered runs. A single aggregation writer guarantees one valid report
containing all executions and avoids repeatedly overwriting the user's report
path. The output remains standard JUnit XML for CI systems.

**Alternatives considered**:

- Point each environment's built-in JUnit formatter at the same path: rejected
  because later runs can overwrite earlier results.
- Create temporary per-environment JUnit files and merge them: rejected as
  unnecessary filesystem lifecycle and failure handling when results can be
  aggregated from the runner's scenario events.
- Use only Godog's built-in JUnit formatter: suitable if the feature were one
  execution; insufficient by itself for the required combined report.

**Sources**:

- [Godog JUnit formatter](https://github.com/cucumber/godog/blob/v0.16.0/internal/formatters/fmt_junit.go)
- [Godog formatter registration and output](https://github.com/cucumber/godog/blob/v0.16.0/run.go)

## Offline manifest input

**Decision**: Extend `source.FilesSource` with an injected `io.Reader` for stdin.
Interpret the single path `-` as standard input and decode it through the same
multi-document YAML/JSON decoder used for files. The test command rejects repeated
stdin references; the existing hostname command retains its current exit-code
mapping and file behavior.

**Rationale**: Reusing the existing loader keeps Kubernetes object decoding,
resource validation, and duplicate identity detection consistent. Injecting the
reader keeps I/O testable and avoids hardwiring `os.Stdin` into the source layer.

**Alternatives considered**:

- Add a separate stdin YAML parser: rejected because it would duplicate document
  decoding and could diverge from file behavior.
- Let the behavior runner read stdin itself: rejected because it would bypass the
  shared source loader and complicate testing.

**Source**: Existing implementation in `internal/source/files.go`.

## Trace reuse and not-found behavior

**Decision**: Keep `resolve.Resolve` and `diagnose.Diagnose` as the sole routing
interpretation path. Provide the behavior runner a narrow trace function backed by
the current offline analysis composition. When that function returns a populated
analysis with `ErrHostnameNotFound`, retain the analysis as a valid trace outcome
so explicit absence assertions can pass.

**Rationale**: The existing analyzer already returns both structured analysis and
the not-found sentinel. Treating the sentinel as data in the test assertion layer
preserves exit code 3 for the old hostname command and makes not-found testable
without duplicating route semantics.

**Alternatives considered**:

- Reimplement hostname matching in steps: rejected because route behavior could
  diverge from the existing trace.
- Change the existing command to stop returning the not-found sentinel: rejected
  because that changes its documented exit code.
