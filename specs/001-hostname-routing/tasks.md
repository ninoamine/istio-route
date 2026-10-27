# Tasks: Hostname Traffic Routing

**Input**: Design documents from `specs/001-hostname-routing/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/cli.md,
quickstart.md

**Tests**: Tasks include table-driven tests, fake-client checks, and renderer golden
tests because these are required by the project constitution and design.

**Organization**: Tasks are grouped by user story. Every task is a checklist item,
has a unique sequential ID, and identifies the files to create or modify.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: The task can proceed in parallel with other tasks in its phase because it
  has no dependency on their unfinished files or behavior.
- **[Story]**: The user story the task implements; setup, foundational, and polish
  tasks do not use a story label.

## Phase 1: Setup

**Purpose**: Initialize the Go module and shared project tooling.

- [X] T001 [P] Initialize `go.mod` for module `github.com/ninoamine/istio-route` with Go 1.27 and the researched compatible versions of Istio client-go, Kubernetes client-go, Gateway API, Cobra, and Lip Gloss; resolve and record dependencies in `go.sum`.
- [X] T002 [P] Configure `golangci-lint` in `.golangci.yml` and GoReleaser platform targets in `.goreleaser.yaml` for Linux, macOS, and Windows on amd64 and arm64.
- [X] T003 [P] Create the GitHub Actions workflow at `.github/workflows/ci.yml` with Go 1.27 setup, formatting, lint, vet, test-race, coverage, and build jobs; run GoReleaser checks in CI and publish releases only for version tags.

---

## Phase 2: Foundational

**Purpose**: Establish shared data contracts, I/O boundaries, and command wiring
required by all user stories.

**Blocking rule**: Complete this phase before starting user-story implementation.

- [X] T004 Define the shared hostname analysis, source snapshot, routing tree, route rule, destination, endpoint, condition, and finding types in `internal/model/model.go` according to `data-model.md`.
- [X] T005 [P] Add table-driven validation and JSON/YAML serialization tests for model identities, optional status, weights, findings, and external ServiceEntry destinations in `internal/model/model_test.go`.
- [X] T006 Define context-aware read-only Kubernetes client interfaces and typed resource access boundaries in `internal/kube/client.go`; expose only get/list operations for supported resources, Services, EndpointSlices, and Pods.
- [X] T007 Define the common cluster/files snapshot interface and source error types in `internal/source/source.go`; preserve API group, version, kind, namespace, and name for every object.
- [X] T008 Create the Cobra command constructor and flag structs in `internal/cli/root.go`, configure the default `log/slog` handler to write to stderr, and keep command handlers limited to validation and delegation to the analysis service.
- [X] T009 Keep `cmd/istio-route/main.go` minimal by constructing the CLI and mapping returned errors to the documented exit codes without panicking outside `main`.

**Checkpoint**: Shared model, source boundaries, client interfaces, and command
entry point are ready.

---

## Phase 3: User Story 1 - Trace a hostname to workloads (Priority: P1) 🎯 MVP

**Goal**: Resolve matching entry points and routes through weighted destinations
to Services and actual endpoint Pods for both supported API families.

**Independent Test**: Run the CLI against the current Kubernetes context using a
read-only test identity and representative Istio and Gateway API resources; verify
entry point properties, API-ordered rules, destinations, weights, ServiceEntry
external details, Services, and actual endpoint Pod readiness.

### Tests for User Story 1

- [ ] T010 [P] [US1] Add table-driven exact, wildcard, namespace-qualified Istio host, and Gateway API hostname intersection cases in `internal/resolve/hostname_test.go`.
- [ ] T011 [P] [US1] Add routing-tree fixture assertions for Gateway/listener properties, VirtualService/HTTPRoute rule order, destinations, weights, and endpoints in `internal/resolve/resolve_test.go`.
- [ ] T012 [P] [US1] Add a golden end-to-end text report fixture for mixed Istio and Gateway API routes in `testdata/golden/mixed-routing.txt` and `internal/render/text_test.go`.

### Implementation for User Story 1

- [ ] T013 [US1] Implement normalized exact and wildcard hostname matching, including namespace-qualified Istio hosts and Gateway API listener/route hostname intersection, in `internal/resolve/hostname.go`.
- [ ] T014 [US1] Implement context-aware typed list operations for Istio Gateway, VirtualService, DestinationRule, ServiceEntry and Gateway API GatewayClass, Gateway, HTTPRoute, and ReferenceGrant, plus referenced backend Service, EndpointSlice, and endpoint-target Pod reads, in `internal/kube/routing.go` and `internal/kube/endpoints.go`.
- [ ] T015 [US1] Resolve Istio Gateway servers to VirtualServices, DestinationRule subsets, ordered matches/actions, weighted destinations, and ServiceEntry destinations in `internal/resolve/istio.go`.
- [ ] T016 [US1] Resolve GatewayClass/controller, Gateway listeners, HTTPRoute parent/listener attachment, ordered matches/filters/timeouts, backendRefs, and cross-namespace references in `internal/resolve/gateway_api.go`.
- [ ] T017 [US1] Interpret Service, EndpointSlice, and endpoint-target Pod objects returned by the kube client, associate actual endpoints with each resolved destination, and preserve readiness or unavailable status in `internal/resolve/endpoints.go`.
- [ ] T018 [US1] Assemble matching entry points, routes, rules, destinations, workloads, and source availability into the shared analysis tree in `internal/resolve/resolve.go`.
- [ ] T019 [US1] Render the default text tree with entry point details, ordered numbered rules, per-backend weights, explicit `[Istio]`/`[Gateway API]` node labels, and required tree structure in `internal/render/text.go`.
- [ ] T020 [US1] Implement the end-to-end analysis service that uses the current Kubernetes context by default, obtains the resource snapshot through the cluster source and kube interfaces, resolves the hostname, and returns the routing tree to the Cobra handler in `internal/cli/analyze.go` and `internal/source/cluster.go`.

**Checkpoint**: The MVP can explain a hostname from a supplied resource snapshot
through its supported routes and actual in-cluster endpoints.

---

## Phase 4: User Story 2 - Diagnose binding and route problems (Priority: P2)

**Goal**: Explain route conditions and identify missing, invalid, or conflicting
bindings and destinations with actionable findings.

**Independent Test**: Run table-driven fixtures for every warning class in FR-015
and binding failure in FR-014; verify finding severity, related resource, and
plain-language reason.

### Tests for User Story 2

- [ ] T021 [P] [US2] Add table-driven tests for HTTPRoute Accepted/ResolvedRefs, Gateway Programmed, and plain-language hostname/parent/listener/backend failures in `internal/diagnose/conditions_test.go`.
- [ ] T022 [P] [US2] Add table-driven tests for unbound VirtualServices, missing DestinationRule subsets and ReferenceGrants, invalid weights, wildcard shadowing, and cross-family hostname conflicts in `internal/diagnose/warnings_test.go`.

### Implementation for User Story 2

- [ ] T023 [US2] Map Gateway and HTTPRoute condition types, statuses, reasons, messages, and missing status to model condition values and plain-language explanations in `internal/diagnose/conditions.go`.
- [ ] T024 [US2] Detect missing HTTPRoute parents/listeners, hostname non-intersection, unresolved backend references, and unauthorized cross-namespace references in `internal/diagnose/references.go`.
- [ ] T025 [US2] Detect unbound Istio VirtualServices, absent subset DestinationRules, and missing or non-authorizing ReferenceGrants in `internal/diagnose/istio_gateway.go`.
- [ ] T026 [US2] Detect Istio weight totals other than 100, all-zero Gateway API backend weights, wildcard overlap/shadowing, unmatched hostname, and mixed Istio/Gateway API hostname conflicts in `internal/diagnose/warnings.go`.
- [ ] T027 [US2] Attach stable codes, severity, related resource identity, and actionable explanations to findings and order them deterministically in `internal/diagnose/diagnose.go`.
- [ ] T028 [US2] Render `✔`/`✘` condition markers and append warning lines at the end with the `⚠` prefix in `internal/render/text.go`.

**Checkpoint**: A matching but unhealthy or unresolved route produces a useful
diagnostic report; no-match remains distinct for exit code 3.

---

## Phase 5: User Story 3 - Inspect live or offline configuration (Priority: P3)

**Goal**: Support cluster configuration selection and offline multi-file YAML
inspection, while gracefully skipping unavailable API families.

**Independent Test**: Run the same fixture resources through file input and injected
cluster fakes; confirm shared routing findings, no cluster initialization in file
mode, correct namespace/context selection, and informational handling of missing
CRD families.

### Tests for User Story 3

- [ ] T029 [P] [US3] Add file-source tests for repeated YAML files, multi-document manifests, duplicate identities, malformed YAML, and absent status in `internal/source/files_test.go`.
- [ ] T030 [P] [US3] Add fake-client tests for context/namespace scope, read-only list/get calls, and absent Istio or Gateway API discovery in `internal/source/cluster_test.go`.
- [ ] T031 [P] [US3] Add CLI tests proving `-f/--file` mode does not initialize a cluster client and cluster flags select kubeconfig, context, and namespace in `internal/cli/root_test.go`.

### Implementation for User Story 3

- [ ] T032 [US3] Load one or more YAML files, including multi-document files, into typed/unstructured objects with preserved GVK and actionable parse errors in `internal/source/files.go`.
- [ ] T033 [US3] Extend the default cluster source to honor explicit kubeconfig, context, and optional namespace selection while using injected context-aware read-only clients in `internal/source/cluster.go`.
- [ ] T034 [US3] Discover supported API groups before reads; represent absent Istio or Gateway API CRDs as skipped-family informational messages and continue with available families in `internal/kube/discovery.go`.
- [ ] T035 [US3] Add a fake-client assertion that source queries fetch only referenced backend Services, their EndpointSlices, and endpoint-target Pods, and never list unrelated Services or Pods, in `internal/source/cluster_test.go`.
- [ ] T036 [US3] Wire repeated `-f/--file`, `--kubeconfig`, `--context`, and `-n/--namespace` flags to the appropriate source in `internal/cli/root.go`.

**Checkpoint**: Users can produce the same analysis from live cluster reads or
offline manifests, and unavailable API families do not abort the remaining analysis.

---

## Phase 6: User Story 4 - Choose a useful report format (Priority: P3)

**Goal**: Provide JSON, YAML, and Mermaid output alongside the default colored
text tree, with stable output streams and color behavior.

**Independent Test**: Render one shared analysis fixture to each format and compare
golden files; verify semantic parity, stdout/stderr separation, and no ANSI sequences
when `NO_COLOR` is set or stdout is not a terminal.

### Tests for User Story 4

- [ ] T037 [P] [US4] Add JSON, YAML, and Mermaid golden fixtures for the same mixed-API analysis and verify identical routing facts and findings in `internal/render/render_test.go`.
- [ ] T038 [P] [US4] Add text renderer tests for explicit origin labels on every resource, rule, filter, and destination node, ordered weights, condition markers, final warning placement, `NO_COLOR`, and non-TTY output in `internal/render/text_test.go`.
- [ ] T039 [P] [US4] Add CLI tests verifying result-only stdout and diagnostic/log-only stderr for success, skipped API family, warning, and error cases in `internal/cli/root_test.go`.

### Implementation for User Story 4

- [ ] T040 [US4] Implement JSON serialization of the shared routing tree and findings in `internal/render/json.go`.
- [ ] T041 [US4] Implement YAML serialization of the same routing tree and findings in `internal/render/yaml.go`.
- [ ] T042 [US4] Implement Mermaid graph output that preserves resource relationships without inventing cross-resource evaluation order in `internal/render/mermaid.go`.
- [ ] T043 [US4] Add a renderer selector for `text`, `json`, `yaml`, and `mermaid`, rejecting unsupported formats as usage errors in `internal/render/render.go`.
- [ ] T044 [US4] Apply Lip Gloss styles to text origin/status labels and tree nodes while respecting explicit `--no-color`, `NO_COLOR`, and non-terminal stdout in `internal/render/style.go`.
- [ ] T045 [US4] Wire `--output` and `--no-color` flags to the renderer and keep all report bytes on stdout in `internal/cli/root.go`.

**Checkpoint**: Each supported output format can render one report model consistently;
text formatting obeys the output contract and terminal color rules.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Close quality, security, performance, documentation, and release gaps.

- [ ] T046 [P] Document CLI examples, exit codes, read-only permissions, source modes, output formats, CRD skip behavior, and Secret handling in `README.md`.
- [ ] T047 [P] Add representative Istio, Gateway API, mixed-family, missing-ReferenceGrant, and endpoint/status manifest fixtures in `testdata/manifests/istio.yaml`, `testdata/manifests/gateway-api.yaml`, `testdata/manifests/mixed.yaml`, `testdata/manifests/missing-reference-grant.yaml`, and `testdata/manifests/endpoints.yaml`.
- [ ] T048 Add least-privilege ClusterRole and ClusterRoleBinding manifests granting only `get`/`list` for required routing resources, Services, EndpointSlices, and Pods in `deploy/rbac.yaml`; verify `internal/kube/` has no Secret reads or Kubernetes write methods.
- [ ] T049 Add a live-cluster performance harness and documented procedure in `testdata/performance/generate.go`, `testdata/performance/run.sh`, and `testdata/performance/README.md` that loads the exact SC-010 workload (10,000 Services across 100 namespaces, 100 Istio VirtualServices, 100 Gateway API HTTPRoutes, exactly one matching route from each family, and two referenced Services with three ready Pod endpoints each), runs 10 warm-ups plus 100 sequential hostname analyses on three workers provisioned with 4 vCPU and 8 GiB each, records cluster/worker versions and sequential timings, calculates p95, and fails unless p95 is below 10 seconds; fixture provisioning is by the harness and the CLI under test remains read-only.
- [ ] T050 Run `gofumpt`, `go vet ./...`, `golangci-lint run`, `go test -race ./...`, and `go test ./internal/... -cover`; enforce at least 80% internal coverage in `.github/workflows/ci.yml`.
- [ ] T051 Validate GoReleaser configuration and cross-build Linux, macOS, and Windows binaries for amd64 and arm64 using `.goreleaser.yaml`.
- [ ] T052 Run every end-to-end scenario in `specs/001-hostname-routing/quickstart.md` and record any implementation gaps in `specs/001-hostname-routing/`.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies; establishes module and automation configuration.
- **Foundational (Phase 2)**: Depends on Setup and blocks all user stories.
- **User Story 1 (Phase 3)**: Depends on the shared model, source interfaces, kube
  interfaces, and CLI skeleton from Phase 2.
- **User Story 2 (Phase 4)**: Depends on the routing model and route resolution from
  US1; it adds findings to the shared tree.
- **User Story 3 (Phase 5)**: Depends on the shared source interface and analysis
  service. File and cluster providers can be implemented independently once those
  contracts exist.
- **User Story 4 (Phase 6)**: Depends on the model and baseline tree output from US1;
  condition and warning decorations integrate with US2.
- **Polish (Phase 7)**: Depends on the desired user stories being implemented.

### User Story Dependencies

- **US1 (P1)**: Starts after Phase 2; includes an end-to-end invocation
  against the current Kubernetes context so the MVP is usable before US3 adds
  explicit source-selection flags and offline manifests.
- **US2 (P2)**: Follows US1 because it adds diagnostics to resolved route trees.
- **US3 (P3)**: Extends the default cluster path with explicit kubeconfig/context/
  namespace options and adds offline YAML input; it can proceed alongside US2 after
  US1's source and analysis contracts are stable.
- **US4 (P3)**: Depends on US1's tree and integrates with US2 findings; renderer
  formats can be developed in parallel by separate owners after model stabilization.

### Parallel Opportunities

- Setup tasks T002 and T003 can run in parallel with each other and T001.
- Foundational model tests T005 can run alongside interface and command scaffolding
  after T004's data shape is agreed; kube/source/CLI boundaries are separate files.
- US1 hostname matcher tests T010 and tree fixture/golden preparation T011-T012 can
  be prepared independently; Istio and Gateway API resolver implementations can split
  after the shared model and source snapshot contract are stable.
- US2 condition tests T021 and warning tests T022 are independent; corresponding
  diagnostic components can be developed in separate files.
- US3 file-source, cluster-source, and CLI tests T029-T031 can be worked on in
  parallel after Phase 2; the file loader and cluster provider are independent.
- US4 format serializers T040-T042 are independent once the model is stable.
- Documentation, fixture authoring, and performance harness preparation T046-T049
  can run in parallel, while performance execution requires a provisioned live
  three-worker cluster. CI enforcement and release validation follow their
  respective configuration tasks.

## Parallel Example: User Story 1

```text
After Phase 2, split work by distinct files:
1. Implement exact/wildcard hostname matching in internal/resolve/hostname.go.
2. Implement Istio traversal in internal/resolve/istio.go.
3. Implement Gateway API traversal in internal/resolve/gateway_api.go.
4. Prepare the mixed-API expected output in testdata/golden/mixed-routing.txt.
Integrate after the shared model and source contract are stable.
```

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Setup and Foundational phases.
2. Complete US1 to produce an end-to-end default-cluster invocation and routing
   tree through both supported API families.
3. Validate the independent US1 scenarios, including endpoint and ServiceEntry
   representations, before expanding scope.

### Incremental Delivery

1. Setup + Foundation → stable module, common model, interfaces, and CLI skeleton.
2. US1 → route trace from entry points to destinations/workloads (MVP).
3. US2 → health conditions and actionable warnings.
4. US3 → cluster selection, offline YAML input, and missing-CRD graceful behavior.
5. US4 → complete JSON/YAML/Mermaid renderers and terminal color behavior.
6. Polish → quality gates, coverage, scale validation, docs, and release builds.
