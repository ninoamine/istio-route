<!--
Sync Impact Report
Version change: unversioned -> 1.0.0 (first formal constitution)
Modified principles: none (the previous file contained only template placeholders)
Added principles: Quality Gates; Domain Architecture; Predictable CLI UX;
  Read-Only Cluster Access; Reproducible Releases
Added sections: Implementation Constraints; Delivery and Review Workflow
Removed sections: none
Follow-up TODO: confirm the original ratification date.
-->
# istio-route Constitution

## Core Principles

### I. Quality Gates
All Go code MUST be covered by table-driven tests where behavior has multiple cases. Tests
MUST use golden files for rendering. `internal/` MUST maintain at least 80% test coverage.
CI MUST run `golangci-lint`, `go vet`, and `go test -race`; all MUST pass before a change
is accepted. These gates make regressions measurable and keep behavior consistent.

### II. Domain Architecture
`cmd/istio-route/main.go` MUST remain minimal. Business logic MUST live in `internal/`,
organized by domain, including `resolver`, `model`, `render`, and `kube`. The CLI MUST use
Cobra, and Cobra commands MUST delegate rather than contain business logic. Kubernetes
dependencies MUST be accessed through interfaces so they can be tested with fakes. The
layout keeps domain behavior independently testable and infrastructure replaceable.

### III. Predictable CLI UX
The CLI MUST provide readable output by default and support `--output json|yaml|text|mermaid`.
It MUST honor `NO_COLOR` and disable color when stdout is not a TTY. Results MUST go to
stdout and logs or errors to stderr. Errors MUST be actionable, and documentation MUST
define exit codes: 0 for success, 1 for errors, 2 for usage errors, and 3 when a hostname
is not found. Consistent output and error handling make the CLI dependable in both
interactive and scripted use.

### IV. Read-Only Cluster Access
The CLI MUST NOT modify the Kubernetes cluster. Kubernetes access and required RBAC
permissions MUST be limited to `get` and `list`; the CLI MUST NOT request or invoke
write operations. This restriction prevents an inspection tool from changing the system
it observes.

### V. Reproducible Releases
Releases MUST use semantic versioning and GoReleaser. Each release MUST publish binaries
for Linux, macOS, and Windows on both amd64 and arm64. Predictable versions and complete
platform coverage make releases traceable and usable across supported systems.

## Implementation Constraints

Go code MUST follow Effective Go, Go Code Review Comments, and the Google Go Style Guide.
Functions that perform I/O MUST accept `context.Context` as their first parameter. Errors
MUST be wrapped with contextual information using the `%w` verb. Code MUST NOT panic
outside `main`. Logs MUST use `log/slog` on stderr; command results MUST remain on stdout.
A `utils` package MUST NOT be introduced; shared behavior belongs in the relevant domain
package.

## Delivery and Review Workflow

Changes MUST pass the quality gates in this constitution before merge. Reviewers MUST
verify the architecture, read-only access, CLI behavior, and release requirements when
those areas are affected. User-facing CLI options, output formats, and exit codes MUST be
documented. Release changes MUST preserve the required GoReleaser platforms and
architectures.

## Governance

This constitution is the governing standard for project design, implementation, testing,
and release. Amendments MUST update this document, explain their impact in the change, and
receive review for consistency with the principles. Amendments MUST update the version:
MAJOR for incompatible changes to existing principles, MINOR for new principles or
materially expanded requirements, and PATCH for clarifications that do not change
requirements. Reviewers MUST check relevant changes against this constitution before
approval; any exception MUST be explicitly justified and approved in the change.

**Version**: 1.0.0 | **Ratified**: TODO(RATIFICATION_DATE): original adoption date unknown | **Last Amended**: 2026-10-26
