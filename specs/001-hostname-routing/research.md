# Research: Hostname Traffic Routing

**Date**: 2026-10-26

## Go Toolchain

- **Decision**: Use Go 1.27, with `go 1.27.0` in `go.mod` and CI selecting a current
  Go 1.27 patch release.
- **Rationale**: The requested version is released and supported. The Go release
  history lists Go 1.27.3 as released on 2026-10-19.
- **Alternatives considered**: Go 1.26; rejected because the user explicitly selected
  Go 1.27.
- **Source**: [Go release history](https://go.dev/doc/devel/release).

## Kubernetes Client Version Alignment

- **Decision**: Start with Kubernetes module versions v0.36.1 consistently across
  `client-go`, `api`, `apimachinery`, and generated Gateway API clients.
- **Rationale**: Gateway API v1.6.3 declares Kubernetes v0.36.1 dependencies.
  Istio client-go v1.31.3 declares Kubernetes v0.35.0 dependencies. Go module
  minimal-version selection can raise the Istio transitive requirements to v0.36.1.
  Pinning the direct Kubernetes client to v0.36.1 avoids a mixed generated-client
  baseline. Validate imports, fakes, and `go test ./...` against the resolved module
  graph before implementation is considered complete.
- **Alternatives considered**:
  - Kubernetes v0.35.0: rejected because Gateway API v1.6.3 requires v0.36.1.
  - Kubernetes v0.37.1: newer, but the selected Gateway API release declares v0.36.1;
    do not select it without confirming compatibility for the complete module graph.
- **Sources**:
  - [Istio client-go v1.31.3 module metadata](https://proxy.golang.org/istio.io/client-go/@v/v1.31.3.mod)
  - [Gateway API v1.6.3 module metadata](https://proxy.golang.org/sigs.k8s.io/gateway-api/@v/v1.6.3.mod)
  - [Kubernetes client-go v0.37.1 module metadata](https://proxy.golang.org/k8s.io/client-go/@v/v0.37.1.mod)

## Gateway API Type and Fake Client

- **Decision**: Add `sigs.k8s.io/gateway-api` as a direct dependency and use its
  generated typed Gateway API resources and fake clientset for supported stable
  resource versions.
- **Rationale**: `k8s.io/client-go` and Istio client-go do not provide typed Gateway
  API resources. A typed API client keeps route and status handling type-safe and
  enables fake-client tests. If the CRDs are absent from a cluster, treat API
  discovery/NoMatch as an informational family skip, not as a route-analysis error.
- **Alternatives considered**: Dynamic unstructured access only; rejected as the
  primary approach because it weakens type checking and adds manual conversion for
  known Gateway API types. Dynamic access may still be used for generic manifest
  loading.

## Dependency Versions

Use these versions as researched starting points and record the final resolved graph
in `go.mod`/`go.sum` during implementation:

| Module | Starting version | Compatibility note |
|--------|------------------|--------------------|
| `istio.io/client-go` | v1.31.3 | Declares Go 1.25 and Kubernetes v0.35.0 |
| `sigs.k8s.io/gateway-api` | v1.6.3 | Declares Go 1.26 and Kubernetes v0.36.1 |
| `k8s.io/client-go` | v0.36.1 | Align direct Kubernetes dependencies to Gateway API |
| `github.com/spf13/cobra` | v1.10.2 | Compatible with Go 1.27 |
| `github.com/charmbracelet/lipgloss` | v1.1.0 | Compatible with Go 1.27 |

The version metadata was retrieved from the [Go module proxy](https://proxy.golang.org/).
The latest `client-go` metadata at research time was v0.37.1; it is not selected
because the chosen Gateway API release is based on v0.36.1.

## Cluster Read Strategy

- **Decision**: List supported routing resources once per source snapshot, then
  resolve destination Services and their EndpointSlices/Pods only for referenced
  backends. Discover supported API families before list operations; report missing
  Gateway API or Istio CRDs as informational skips.
- **Rationale**: The spec sets a 10,000-Service scale target. Fetching all Pods and
  Endpoints unrelated to a hostname would increase API calls and work without
  improving the report. EndpointSlices represent actual Service endpoints; Pod
  readiness is read only for endpoint Pod references.
- **Alternatives considered**: Fetch and index all Services, EndpointSlices, and Pods
  in all namespaces for every query; rejected as unnecessary cluster-wide data
  transfer.

## Offline YAML Input

- **Decision**: Read one or more YAML files, including multi-document files, into
  Kubernetes objects with GroupVersionKind preserved; aggregate them into the same
  source snapshot contract used by cluster mode.
- **Rationale**: One resolver and diagnostics pipeline can then serve both modes.
  Offline output only reflects objects and status included in the supplied files.
- **Alternatives considered**: Separate offline resolution logic; rejected because it
  would duplicate matching and increase behavioral drift.

## Test and CI Strategy

- **Decision**: Use typed Kubernetes, Istio, and Gateway API fake clientsets for
  cluster-source tests; manifest fixtures for files-source tests; table tests for
  matching and diagnostics; golden files for every renderer; GitHub Actions for
  lint, tests with race detection and coverage enforcement, and build; GoReleaser
  runs on version tags.
- **Rationale**: This follows the project constitution and the user's requested CI.
  Fake clients make source behavior deterministic, while offline fixtures exercise
  shared resolution without a cluster.
- **Alternatives considered**: Require an integration cluster in every CI run;
  rejected because it makes core validation slower and depends on dedicated
  infrastructure. The SC-010 live-cluster performance harness is a required
  maintainer validation; it records its environment and fails on a p95 miss. It is
  not replaced by fake-client timing.

The performance harness uses the SC-010 dataset: 10,000 Services in 100 namespaces,
100 Istio VirtualServices, and 100 Gateway API HTTPRoutes; exactly one matching route
per API family refers to two Services, each with three ready Pod endpoints. The
remaining routes use non-matching hostnames. It runs 10 warm-ups and 100 sequential
measurements on three workers provisioned with 4 vCPU and 8 GiB each.
