# Implementation Plan: Hostname Traffic Routing

**Branch**: `001-hostname-routing` | **Date**: 2026-10-26 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/001-hostname-routing/spec.md` and
implementation choices supplied by the user.

## Summary

Build a read-only Go CLI that explains routing for one hostname across Istio and
Gateway API resources. A source layer loads either live cluster data or offline YAML;
a resolver builds a shared routing model; a diagnostics layer reports binding,
reference, and hostname conflicts; and renderers produce text, JSON, YAML, or Mermaid.
The design uses Cobra, Istio and Kubernetes clients, Gateway API types, `log/slog`,
and Lip Gloss. Cluster reads remain least-privilege and destination workload details
are fetched for referenced Services only.

## Technical Context

**Language/Version**: Go 1.27; Go module directive `go 1.27.0`.

**Primary Dependencies**: Cobra; `istio.io/client-go` networking/v1 types;
`k8s.io/client-go`; `sigs.k8s.io/gateway-api` typed API and fake clientsets;
`log/slog`; Lip Gloss. Align Kubernetes modules at v0.36.1 initially, then verify
the complete module graph and generated client compatibility before implementation
is accepted.

**Storage**: None. Offline input is read from user-supplied YAML manifests.

**Testing**: Table-driven unit tests; Kubernetes, Istio, and Gateway API fake
clientsets; offline fixtures in `testdata/`; golden output tests for every renderer;
`go test -race ./...`; at least 80% coverage for `internal/`.

**Target Platform**: Linux, macOS, and Windows on amd64 and arm64; release artifacts
are built with GoReleaser.

**Project Type**: Go command-line application.

**Performance Goals**: At least 95% of hostname analyses complete in under 10 seconds
in the dedicated live-cluster workload defined by SC-010: 10,000 Services across
100 namespaces, 100 Istio VirtualServices, 100 Gateway API HTTPRoutes, exactly one
matching route from each family, and two referenced Services with three ready Pod
endpoints each; 10 warm-ups followed by 100 sequential measurements on three workers
provisioned with 4 vCPU and 8 GiB each. The fixture harness may provision the test
resources; the CLI remains read-only.

**Constraints**: Cluster interactions are read-only and limited to Kubernetes
`get`/`list`. Offline mode makes no cluster connection. Results go to stdout;
diagnostics and logs go to stderr. Honor `NO_COLOR` and disable color for
non-terminal stdout. Do not fetch Secret data.

**Scale/Scope**: Up to 10,000 Services in the stated response-time validation
scenario; report only the referenced Services and their actual endpoints and Pods.
Support the Istio Gateway, VirtualService, DestinationRule, ServiceEntry and Gateway
API GatewayClass, Gateway, HTTPRoute, ReferenceGrant families in the specification.

## Constitution Check

| Gate | Plan decision | Status |
|------|---------------|--------|
| Minimal `cmd/`; domain logic in `internal/` | Cobra wiring stays in `cmd/istio-route/main.go`; implementations are split into source, kube, resolve, model, render, and diagnose packages. | Pass |
| Kubernetes I/O behind interfaces | `source` and `kube` expose context-aware interfaces; clientsets and fakes are injected. | Pass |
| Test and quality gates | Table-driven tests, renderer goldens, `-race`, 80% `internal/` coverage, gofumpt, go vet, and golangci-lint are required in CI. | Pass |
| Read-only permissions | Only list/get operations are used; no write client methods or permissions are included. | Pass |
| CLI output and errors | Four output formats; stdout contains results only; `log/slog` and diagnostics go to stderr; color follows `NO_COLOR` and TTY state; exit codes are documented. | Pass |
| Go style and error handling | Follow project Go conventions, pass `context.Context` first to I/O functions, wrap errors, and do not panic outside `main`. | Pass |
| Release requirements | GoReleaser releases SemVer binaries for Linux, macOS, Windows on amd64 and arm64. | Pass |

## Project Structure

### Documentation (this feature)

```text
specs/001-hostname-routing/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
└── contracts/
    └── cli.md
```

### Source Code (repository root)

```text
cmd/
└── istio-route/
    └── main.go
internal/
├── cli/
├── diagnose/
├── kube/
├── model/
├── render/
├── resolve/
└── source/
testdata/
├── manifests/
└── golden/
deploy/
└── rbac.yaml
.github/
└── workflows/
    └── ci.yml
```

**Structure Decision**: Use one Go CLI project with a minimal Cobra entry point and
domain packages under `internal/`. Cobra command construction and argument/flag
validation live in `internal/cli`; commands delegate all analysis behavior to
package services. `kube` owns client construction and typed reads;
`source` presents a common cluster/files input boundary; `resolve` turns the
host-filtered resource snapshot into the routing model; `diagnose` emits findings;
and `render` formats that model. Fakes and golden files support contract-level
verification without a live cluster.

## Complexity Tracking

No constitution violations or additional projects are proposed.
