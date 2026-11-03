# Implementation Plan: Gateway-Scoped Routing Audit

**Branch**: `003-analyse-mesh` | **Feature**: `003-gateway-routing-audit` | **Date**: 2026-11-03 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/003-gateway-routing-audit/spec.md`
and implementation choices supplied by the user.

## Summary

Add a read-only `analyze` subcommand backed by a new `internal/audit` package. The
package will produce an audit-specific result containing a selected Gateway or
ambient waypoint, its listener and route rules, backend and waypoint-selection
facts, endpoint-check availability, and findings that reuse `model.Finding`.
Offline analysis will use `source.FilesSource`; live analysis will use a new
targeted `ClusterSource` path that reads only the selected Gateway, linked route
families, and resources referenced by those routes or waypoint labels. Four
category checkers will produce orphaned, duplicated, precedence, and deficient
destination findings. Additive audit rendering and fixed-vocabulary behavior steps
will expose the result without changing hostname trace or existing test behavior.

## Technical Context

**Language/Version**: Go 1.27.0, matching the existing `go.mod` directive.

**Primary Dependencies**: Existing Cobra, Kubernetes client-go, Gateway API v1.6.3,
Godog, source, resolve, model, and render packages. No new runtime dependency is
planned.

**Storage**: None. Input is read from existing live cluster resources or supplied
manifest files/stdin; audit output is emitted to stdout.

**Testing**: Table-driven unit tests, fake Kubernetes clients, offline manifest
fixtures, Gherkin scenarios, and golden outputs for text/JSON/YAML/Mermaid.
Preserve the project's 80% `internal/` coverage threshold and run `go test -race
./...`, `go vet ./...`, `golangci-lint run`, and formatting checks. The existing
100-scenario/two-manifest-set behavior-test performance target remains five seconds
or less.

**Target Platform**: Existing GoReleaser platforms: Linux, macOS, and Windows on
amd64 and arm64.

**Project Type**: Go command-line application. Cobra wiring belongs in
`internal/cli`; audit decisions belong in `internal/audit`; Kubernetes reads remain
behind `internal/kube` interfaces and `internal/source`.

**Performance Goals**: Retain the existing target of 100 Gherkin scenarios against
two manifest sets in five seconds or less. Live requests must avoid listing unrelated
Services, EndpointSlices, Pods, or other resource families. Routes are listed only
because Kubernetes has no server-side selector for `parentRefs`; the client filters
those route-family results immediately.

**Constraints**: Strictly read-only, with only get/list operations and no Secret
reads. Do not edit existing feature 001/002 golden files or existing `.feature`
files. Shared behavior changes are additive; hostname trace fields, findings, output,
arguments, and exit codes remain compatible. The root hostname `analyze` remains
reserved by Cobra command dispatch. New permissions are additive and limited to
get/list.

**Scale/Scope**: One named Gateway API Gateway or Istio ambient waypoint per audit.
Inspect only its listeners, parentRef-linked HTTPRoutes/GRPCRoutes, route backends,
relevant ReferenceGrants, and (for waypoints) matching Namespace/Service selectors.
Do not analyze Istio VirtualServices, DestinationRules, AuthorizationPolicies, or
sidecar-specific configuration.

## Constitution Check

| Principle or constraint | Plan response | Gate |
|---|---|---|
| Quality gates and coverage | Add table-driven category/source/CLI tests, behavior scenarios, renderer goldens, maintain at least 80% `internal/` coverage, and run race tests, vet, lint, and formatting. | PASS |
| Minimal command entry point; domain logic in `internal/` | Keep `cmd/istio-route/main.go` minimal. Put checker logic in new `internal/audit`, live reads in the existing source/kube boundary, and command flags/render dispatch in `internal/cli`. | PASS |
| Kubernetes I/O behind interfaces | Add narrowly scoped read methods to `kube.Reader` and targeted-source operations; fake clients verify call scope and that no writes occur. | PASS |
| Context-aware I/O and wrapped errors; no panics | Thread `context.Context` through source, audit, and render/report I/O; wrap causes and return errors. | PASS |
| Read-only cluster access | Use only get/list methods; never read Secret objects or mutate resources. RBAC changes add only the exact required get/list permissions. | PASS |
| Predictable CLI output and documented exit codes | Keep report content on stdout and diagnostics on stderr. Add analyze-specific output, color handling, and exit mapping without changing trace/test contracts. | PASS |
| No `utils` package | Keep shared behavior in its existing domain package; no generic utility package is introduced. | PASS |
| Reproducible releases | Add no runtime service or external binary dependency; existing GoReleaser targets remain unchanged. | PASS |

No constitution exception or unjustified gate violation is proposed.

## Project Structure

### Documentation (this feature)

```text
specs/003-gateway-routing-audit/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
└── contracts/
    ├── analyze-command.md
    └── audit-steps.md
```

### Source Code (repository root)

```text
cmd/istio-route/
└── main.go
internal/
├── audit/
│   ├── audit.go                # Request, Result, orchestration, typed outcomes
│   ├── source.go               # Offline/live source selection and targeted fetch flow
│   ├── orphaned.go             # Rejected route, unresolved backend/grant, waypoint checks
│   ├── duplicated.go           # Exact duplicate route-rule checks
│   ├── precedence.go           # Winner, partial overlap, and full-shadow checks
│   ├── deficient.go            # Ready-address and backend-port health checks
│   └── *_test.go
├── cli/
│   ├── audit.go                # New analyze command, distinct from hostname service
│   ├── root.go                 # Add subcommand without changing hostname parsing
│   └── *_test.go
├── kube/
│   ├── client.go               # Add typed Gateway/GRPCRoute/Namespace read methods
│   └── *_test.go
├── model/
│   ├── model.go                # Add optional Finding category/suggested-fix fields
│   └── *_test.go
├── render/
│   ├── audit.go                # Audit-only render entry point and formats
│   └── *_test.go
├── source/
│   ├── cluster.go              # Add targeted Gateway snapshot/fetch path
│   └── *_test.go
└── behavior/
    ├── runner.go               # Carry the optional narrow audit dependency
    ├── environments.go         # Add audit callback to scenario environment
    ├── steps.go                # Register audit assertions without CLI imports
    └── *_test.go
testdata/
├── manifests/                  # Healthy, rejected, missing backend/grant, duplicate, overlap,
│                               # deficient endpoint, and waypoint fixtures
├── features/                   # Ten audit Gherkin cases, including unknown gateway
└── golden/                     # Audit text, JSON, YAML, and Mermaid goldens
deploy/
└── rbac.yaml                   # Add only necessary read permissions
README.md                       # Command, RBAC, exit-code, and reserved-name documentation
specs/002-routing-behavior-testing/
├── step-reference.md           # New audit step reference entries
└── [existing regression assets remain byte-for-byte unchanged]
```

**Structure Decision**: Extend the existing single Go CLI project. The existing
`internal/cli/analyze.go` implements the hostname `AnalysisService`; it is not the
new subcommand or audit domain. Add the separate subcommand in
`internal/cli/audit.go` to avoid confusing that hostname service with gateway
auditing. New business rules live in `internal/audit`, with one checker per finding
category. Use additive source and resolver helpers rather than changing the
hostname trace pipeline. Use a distinct audit renderer so existing text/Mermaid
output and JSON/YAML representations remain unchanged.

The implementation documentation must add the new command and supported audit
steps to `README.md` and `specs/002-routing-behavior-testing/step-reference.md`.
Fixtures and scenarios must cover the ten cases in FR-020; do not alter any existing
feature file or golden expected content.

## Implementation and Regression Gates

Implementation tasks must group work into coherent code-changing phases. Each phase
that changes code ends with the full regression gate below, and a failure blocks the
next phase. New tests are added before shared behavior changes. Existing golden files
and existing `.feature` files are immutable regression inputs.

1. Run `go test -race ./...`, then `go vet ./...`, then `golangci-lint run`, and
   formatting checks. Also verify the 80% `internal/` coverage requirement.
2. Build the binary and run the existing Gherkin cases with exact exit statuses:

   ```sh
   go build -o istio-route ./cmd/istio-route
   ./istio-route test -f testdata/manifests/behavior.yaml testdata/features/routing-passes.feature
   ./istio-route test -f testdata/manifests/gateway-api.yaml testdata/features/gateway-api.feature
   ./istio-route test --manifests prod=testdata/manifests/behavior-prod.yaml --manifests staging=testdata/manifests/behavior-staging.yaml testdata/features/environments.feature
   ./istio-route test -f testdata/manifests/behavior.yaml testdata/features/routing-fails.feature
   ```

   The first three commands must exit 0; the last must exit 1.
3. Run the existing root hostname command on `mixed.yaml` in text, JSON, YAML, and
   Mermaid formats and diff stdout byte-for-byte against
   `testdata/golden/mixed-routing.{txt,json,yaml,mmd}`. Each command must exit 0.
4. Time the existing 100-scenario/two-manifest-set behavior workload after adding
   audit steps; it must remain within five seconds.

## Complexity Tracking

No constitution violations require an exception.
