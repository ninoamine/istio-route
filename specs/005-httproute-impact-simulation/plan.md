# Implementation Plan: HTTPRoute Removal Impact Simulation

**Branch**: `005-httproute-impact-simulation` | **Date**: 2026-11-10 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/005-httproute-impact-simulation/spec.md`

## Summary

Add a read-only `simulate` subcommand that lists eligible HTTPRoutes or compares routing on one selected waypoint before and after excluding one namespaced HTTPRoute from an in-memory snapshot. Extract and reuse existing waypoint, parent-reference, hostname, and precedence primitives; add listener-specific attachment evidence and a conservative traffic-region comparison rather than building a second routing engine. Render one typed impact model as text, JSON, YAML, and Mermaid. Do not query pods, workloads, EndpointSlices, or application health; do not write to the cluster or manifests.

**Scope notes**: HBONE listeners (the only listener on typical ambient waypoints) are applicable listeners; restrictions that cannot be established yield unconfirmed attachments. Service-parented routes and non-HTTPRoute replacements are outside the report's coverage scope and are disclosed with a count.

## Technical Context

**Language/Version**: Go 1.27, as declared in `go.mod`.

**Primary Dependencies**: Existing Cobra, Kubernetes client-go and Gateway API types, Istio client types, `sigs.k8s.io/yaml`, and existing render/style helpers. No new dependency is planned.

**Storage**: Local YAML/JSON streams or read-only cluster data; ephemeral snapshots and results only. No generated or rewritten input manifests.

**Testing**: Table-driven Go tests and fake Kubernetes clients; deterministic golden files for each format; fixture and CLI end-to-end tests; `go test -race ./...`, `go vet ./...`, `golangci-lint run`, and `gofumpt` verification. Maintain at least 80% `internal/` coverage.

**Target Platform**: Existing Linux, macOS, and Windows CLI release targets.

**Project Type**: Cobra CLI with domain logic in `internal/`.

**Performance Goals**: No new latency or scale threshold is specified; preserve deterministic results. Measure realistic fixture runs during implementation, but do not claim a performance target that the spec does not set.

**Constraints**: `get`/`list` only; no pod/workload or endpoint-health analysis; local mode makes no cluster calls; source files and shared snapshots remain unchanged; preserve all existing commands and outputs; return undetermined rather than unsupported certainty.

**Scale/Scope**: One waypoint, one optional HTTPRoute, all relevant direct waypoint-parented HTTPRoutes across namespaces, and the selected waypoint's listeners. Result scope is the collected snapshot, not an assertion of cluster-wide completeness or a transactional live snapshot.

## Constitution Check

| Principle | Before research | After design | Evidence and gate |
|-----------|-----------------|--------------|-------------------|
| I. Quality Gates | PASS | PASS | Table-driven source, domain, and CLI tests; four-format goldens, race tests, vet, lint, formatting, and coverage gate are planned. |
| II. Domain Architecture | PASS | PASS | Thin Cobra adapter; pure simulation domain; existing domain primitives extracted for reuse; injectable read-only sources. `cmd/` remains minimal. |
| III. Predictable CLI UX | PASS | PASS | Existing flags, streams, color conventions, four formats and documented 0/1/2/3 status mapping; no findings-specific status for simulation. |
| IV. Read-Only Cluster Access | PASS | PASS | Narrow snapshot reader uses only `get`/`list`; fake-client action assertions prohibit write verbs and pod/endpoint access. No manifest writes. |
| V. Reproducible Releases | PASS | PASS | No change to release machinery or supported platforms. |

No exceptions or unjustified gate violations are planned. These are design checks, not claims that implementation tests have already passed.

## Project Structure

### Documentation (this feature)

```text
specs/005-httproute-impact-simulation/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── cli.md
└── tasks.md             # later /speckit-tasks output, not created here
```

### Source Code (repository root)

```text
cmd/istio-route/         # preserve main; map a route-not-found sentinel to 3
internal/cli/            # simulate adapter, service and command tests
internal/source/         # route-only live snapshot and existing file reader
internal/kube/           # existing reader/factory interfaces and fake clients
internal/audit/          # shared attachment and precedence primitives
internal/resolve/        # existing hostname matching primitives
internal/simulate/       # pure before/after evaluation, impact model and tests
internal/render/         # impact rendering, four-format golden tests
testdata/manifests/      # simulation fixtures, including explicit statuses
testdata/golden/         # simulation text/JSON/YAML/Mermaid goldens
deploy/rbac.yaml         # inspect existing read-only permissions; add no writes
README.md                # command, limitations, output, access and exit codes
```

**Structure Decision**: Keep I/O in existing source/client layers and add one domain-specific simulation package. Extract narrowly scoped existing helpers into shared domain APIs where needed; retain the existing audit adapter and behavior. Avoid a `utils` package, a second precedence implementation, and wholesale use of the existing audit result, which includes health checks and lacks required certainty and traffic partitions.

## Phase 0: Research

See [research.md](./research.md) for source findings, decisions, rejected alternatives, and correctness limitations. All technical choices needed to proceed are resolved; no external research or new dependency is required.

## Phase 1: Design

1. Define the command and typed output in [contracts/cli.md](./contracts/cli.md), including exact source selection, route identity, error paths, stable coverage labels, and uncertainty.
2. Introduce a targeted snapshot interface accepting `context.Context`: use `FilesSource.Snapshot` offline and a route-only cluster reader online. Fetch the waypoint, optional GatewayClass, all HTTPRoutes needed to establish selection and coverage, and namespace metadata only when needed to evaluate allowed-routes selectors. Never call the existing full `GatewaySnapshot` unchanged: it also reads endpoint and waypoint-selector data and filters away unattached routes needed to distinguish missing from ineligible.
3. Extract reusable waypoint and direct-parent identity matching from `internal/audit`; preserve `analyze` behavior. Build explicit per-listener attachment evidence using parent section/port, listener protocol and restrictions, hostname intersection, route namespaces, status parent/controller correspondence, and nonzero matching `observedGeneration`. Missing evidence is unconfirmed; a current rejection only excludes that listener.
4. Build independently owned before and after analysis inputs; remove only the selected HTTPRoute identity from the latter. Keep original objects immutable. Use a shared per-match precedence comparator and hostname primitives, with proven request-set partition/intersection operations, to calculate both routing views and their difference.
5. Classify each listener/traffic region only when supported by evidence: `unrouted`, `still-covered`, `no-change`, or `undetermined`. Preserve potentially changed winners, fallback references, and a clearly marked provisional evaluation for unknown attachment/class data. Describe exact host/path/method portions and symbolic exclusions; do not promote the audit's pairwise summary flags into proof of full coverage.
6. Render a single validated model across all formats. Emit eligible-route listings to stdout before a not-found or ineligible-target error, and errors to stderr; distinguish a valid simulation with impact from failure.
7. Cover every functional requirement and acceptance scenario with targeted tests, then run the repository quality gates. The implementation validation path and fixture expectations are in [quickstart.md](./quickstart.md).

## Verification Priorities

- Snapshot parity across file and fake-cluster sources; byte-identical local files and zero Kubernetes mutation actions even on failure. Assert no Pod, workload, EndpointSlice, or health reads and no cluster initialization in local mode.
- Namespace/name identity, direct versus Service parents, parent group/kind defaults, `sectionName`/port, listener hostname/kind/namespace restrictions, multiple waypoints/parents, HBONE-only waypoints, waypoints with no applicable listener, rejected/unknown/current/stale status and omitted local namespaces.
- Exact/prefix path segment boundaries (`/api` does not cover `/apix`), wildcard hostname intersections, independent listeners, all/specific methods, disjunctive matches, header/query conditions, multiple competing routes, union coverage, fully shadowed and partially shadowed routes, and deterministic input permutations.
- A rule-wide best match can be irrelevant to a particular request, and the audit's existing pairwise equivalence/coverage summaries are not a complete impact calculator. Use the best *matching* candidate for each region, preserve existing audit behavior, and explicitly mark any unprovable region undetermined.
- Same logical data in all four renderers; structured output round trips, list-on-error and exit-code tests, `NO_COLOR`/non-TTY, duplicate resources, source/RBAC failures and cancellation.

## Complexity Tracking

No constitution violations. A dedicated region-difference model is necessary because existing audit findings do not encode traffic subsets, listener certainty, or before/after winners; it reuses shared matching/precedence primitives rather than replacing the routing engine.
