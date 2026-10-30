# Implementation Plan: Routing Behavior Testing

**Branch**: `002-routing-behavior-testing` | **Date**: 2026-10-30 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/002-routing-behavior-testing/spec.md` and technical context supplied for planning.

## Summary

Add a Cobra `test` subcommand that evaluates fixed-vocabulary Gherkin scenarios
against offline rendered manifests. Reuse the current manifest loader and hostname
resolution/diagnosis path, including its structured `model.Analysis` result; do not
reimplement routing interpretation. The existing hostname command remains a
positional invocation and retains its current exit statuses. Use Godog's embedded
`TestSuite` with one tag-filtered execution per named manifest set, human-readable
formatting, and a combined JUnit report covering all environment executions.

## Technical Context

**Language/Version**: Go 1.27.0 (module version).

**Primary Dependencies**: Cobra v1.10.2 and existing resolver/source/model packages;
Godog v0.16.0 for Gherkin parsing and execution. Godog v0.16.0 supports embedding
through `godog.TestSuite`, `Options.Paths`, `Options.Tags`, `Options.Strict`,
`Options.Format`, `Options.Output`, and optional `embed.FS`; it declares Go 1.18 as
its minimum.

**Storage**: Rendered manifests and feature files are read from disk or standard
input. The optional JUnit report is written to a user-selected file. No persistent
application storage.

**Testing**: Table-driven Go unit tests, fixture-backed integration tests, and
standard-library race-enabled tests. Required verification: `go test -race ./...`,
`go vet ./...`, `golangci-lint run`, and `gofumpt` on changed Go files.

**Target Platform**: Existing GoReleaser targets: Linux, macOS, and Windows on amd64
and arm64.

**Project Type**: Go command-line application using Cobra; command wiring in
`internal/cli`, behavior execution in `internal/behavior`, business analysis in
existing domain packages.

**Performance Goals**: Up to 100 scenarios against two manifest sets complete in
five seconds or less on a representative CI runner, excluding manifest generation
and feature discovery.

**Constraints**: Strictly offline and read-only; never initialize a Kubernetes
client for the test command. Keep the existing hostname invocation and exit codes
0/1/2/3 unchanged. Consume stdin at most once per invocation. Fixed built-in steps
only; no user-supplied step code. The current trace returns `model.Analysis` and
may also return `cli.ErrHostnameNotFound`; that sentinel must be interpreted as a
valid trace outcome for not-found assertions, not as a behavior-runner failure.
Inject stdin through the existing offline source path so CLI integration tests do
not replace process-global stdin.

**Scale/Scope**: One unnamed manifest set or multiple named sets; each set may
combine files. Feature inputs may be files or recursively discovered directories.
Each scenario outline example and each applicable environment execution is an
independently reported scenario result.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle or constraint | Plan response | Gate |
|---|---|---|
| Quality gates and table-driven tests | Add unit and fixture-backed integration coverage, golden console-report tests, and a measured `internal/` coverage check that enforces the 80% threshold; run race tests, vet, linter, and formatting. | PASS |
| Minimal `cmd/istio-route/main.go`; Cobra delegates to `internal/` | Add a Cobra subcommand in `internal/cli`; keep orchestration and scenario assertions in `internal/behavior`. | PASS |
| Context-aware I/O and wrapped errors; no panics | Pass `context.Context` through manifest, feature, and report I/O; wrap causes and propagate output failures. | PASS |
| Read-only; no cluster calls for offline operation | The test command accepts rendered files/stdin only and uses the existing offline loader and analysis path. It must not construct `ClusterSource`. | PASS |
| Predictable CLI output and documented exit codes | Keep human results on stdout and diagnostics on stderr. Map test assertion failures to status 1 and invalid test inputs to status 2 in the existing process exit boundary; leave the hostname mapping unchanged. Document both command contracts. | PASS |
| No `utils` package | Put Gherkin behavior in the `behavior` domain package and share existing source/model logic directly. | PASS |
| Reproducible releases | Godog is embedded in the binary; no runtime service or external executable is required. Existing GoReleaser targets remain unchanged. | PASS |

No constitution exceptions or complexity violations are proposed.

## Project Structure

### Documentation (this feature)

```text
specs/002-routing-behavior-testing/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── step-reference.md
└── contracts/
    ├── test-command.md
    └── step-language.md
```

### Source Code (repository root)

```text
cmd/istio-route/
└── main.go                         # Existing process/exit-code boundary
internal/
├── behavior/
│   ├── runner.go                   # Godog suite setup and environment executions
│   ├── steps.go                    # Fixed vocabulary and structured assertions
│   ├── suggest.go                  # Undefined-step suggestion matching
│   ├── report.go                   # Aggregate JUnit XML for all environments
│   └── *_test.go
├── cli/
│   ├── root.go                      # Register test child without changing hostname args
│   ├── test.go                      # Test flags, validation, and exit mapping
│   └── *_test.go
├── source/
│   ├── files.go                     # Extend existing loader to read "-" from injected stdin
│   └── *_test.go
└── [existing diagnose/, model/, render/, resolve/, kube/, source/ packages]

testdata/
├── manifests/
│   ├── behavior-prod.yaml
│   └── behavior-staging.yaml
└── features/
    ├── routing-passes.feature
    └── routing-fails.feature
```

**Structure Decision**: This is one existing Go CLI project. The dedicated
`internal/behavior` package owns the Gherkin runner and assertion language. Cobra
continues to own argument parsing only. The existing `source`, `resolve`, and
`diagnose` packages remain the sole manifest and routing-analysis implementations.
The CLI passes the behavior package a narrow trace dependency so the behavior
package does not import `internal/cli` and create an import cycle.

## Complexity Tracking

No constitution violations require an exception.
