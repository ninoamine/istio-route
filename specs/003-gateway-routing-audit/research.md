# Research: Gateway-Scoped Routing Audit

**Date**: 2026-11-03

## Domain Boundary and Package Naming

- **Decision**: Implement the audit domain in a new `internal/audit` package and
  expose a separate `analyze` Cobra subcommand from `internal/cli`.
- **Rationale**: The repository already has `internal/cli/analyze.go` for the
  hostname-oriented `AnalysisService` and `model.Analysis`. A separate audit package
  and result type prevent the gateway audit from conflating those concepts.
- **Alternatives considered**: Put all audit decisions in the existing CLI analysis
  service; rejected because the command layer must remain thin and the domain has
  different inputs, identity, findings, and exit behavior.

## Live Resource Discovery

- **Decision**: Add a targeted Gateway source path alongside, not in place of,
  `ClusterSource.Snapshot`. Discover Gateway, GatewayClass, HTTPRoute, GRPCRoute,
  and ReferenceGrant resource availability individually. Get the selected Gateway
  and its GatewayClass by exact identity. List only available HTTPRoute and
  GRPCRoute API families, filter parent references client-side, then fetch backend
  Services and their EndpointSlices by referenced namespace/name and service label.
  Fetch only ReferenceGrants in backend namespaces implicated by cross-namespace
  route references. For waypoint selectors, query Namespace and Service resources
  using `istio.io/use-waypoint` label selectors for the local waypoint name and its
  namespace-qualified form.
- **Rationale**: Kubernetes does not offer a server-side selector for Gateway API
  `parentRefs`. The user explicitly requires listing route families and filtering
  while forbidding an unrelated whole-cluster inventory. Label selectors and exact
  resource gets keep all other reads tied to the selected Gateway, referenced
  backends, or waypoint label.
- **Alternatives considered**:
  - Reuse the hostname `ClusterSource.Snapshot` unmodified; rejected because it
    lists unrelated Istio resources, Gateway resources, and grants and does not
    include GRPCRoutes.
  - List all Services, EndpointSlices, Namespaces, and Pods; rejected because it
    violates the requested perimeter and causes avoidable data transfer.
  - Use a server-side parentRef selector; rejected because Kubernetes has no such
    selector.
- **Unavailable resources**: If the Gateway/GatewayClass API family required to
  identify the target is missing, return a clear source error (exit 1). If an
  individual route or ReferenceGrant family is unavailable, preserve its unavailable
  status and continue with available resources, matching existing resource-family
  reporting.

## Offline Resources and Shared Resolution

- **Decision**: Load only supplied manifests through `FilesSource`, filter the
  resulting snapshot to the selected Gateway audit perimeter, and use exported
  hostname intersection helpers from `internal/resolve` where listener/route host
  attachment semantics overlap. Keep audit route acceptance and precedence checks
  in `internal/audit`.
- **Rationale**: `FilesSource` already supports multi-document YAML and typed
  Kubernetes/Gateway API objects. The hostname resolver intentionally omits
  rejected routes and is hostname-scoped, so it cannot be the audit orchestrator.
  Reusing shared host semantics while retaining audit-specific route inclusion
  avoids duplicating behavior or changing hostname trace results.
- **Alternatives considered**: Call the hostname resolver for a fabricated hostname;
  rejected because it would omit non-accepted and non-matching routes and would
  produce an inappropriate `model.Analysis`.

## Route Families and Matching Facts

- **Decision**: Use the existing Gateway API v1.6.3 typed client and `HTTPRoute` and
  `GRPCRoute` v1 resources. Match parent references by defaulted group/kind,
  referenced Gateway namespace/name, and optional listener section; retain routes
  even when `Accepted=False`. Represent a missing Accepted condition as unknown.
- **Rationale**: Existing dependencies and fake clients already use typed Gateway
  API v1 resources. Explicit parent identity and status preserve the distinction
  between a route targeting the Gateway and one accepted by its controller.
- **Alternatives considered**: Treat absent acceptance status as rejection;
  rejected because controller status may not have been reported yet and the
  clarification specifies an unknown state.

## Duplicates and Gateway API Precedence

- **Decision**: Define a route rule's comparison scope by the listener(s) to which
  its parent reference applies, hostname, and API match conditions. Detect exact
  duplicates by equivalent hostname and path/header/method/query (or the matching
  GRPCRoute service/method/header fields). Apply match specificity first, then
  earliest creation timestamp, then namespace/name ordering. Report an overlap as
  partial unless the union of higher-precedence matches covers the lower rule's
  whole match space; only fully covered rules are marked shadowed.
- **Rationale**: This preserves the user's stated precedence order and avoids
  incorrectly declaring a partially overlapping route dead. Stable namespace/name
  ordering and explicit listener scope make results deterministic.
- **Alternatives considered**:
  - Treat any overlap as complete shadowing; rejected because the lower rule may
    receive requests outside the overlap.
  - Report only duplicate matches; rejected because the feature requires precedence
    for overlapping, non-identical routes as well.

## Waypoint Selection

- **Decision**: Treat an ambient waypoint as a Gateway API Gateway whose class is
  `istio-waypoint`. Resolve `istio.io/use-waypoint` selectors against the waypoint's
  own namespace/name and the namespace-qualified waypoint form. A selector for a
  missing waypoint or a waypoint without a true Programmed condition is an
  orphaned finding.
- **Rationale**: The selected Gateway identity and class provide the target; the
  selector labels identify exactly which Namespaces and Services attach to it.
  Label-filtered discovery avoids a whole-cluster resource inventory.
- **Alternatives considered**: Treat every Gateway as a waypoint; rejected because
  ambient selection is explicitly tied to waypoint class and labels.

## Findings and Compatibility

- **Decision**: Reuse `model.Finding` and add optional, `omitempty` category and
  suggested-fix fields. Keep the four finding categories as stable constants in
  `internal/audit`. Sort findings by severity (error, warning, info), category in
  canonical order (orphaned, duplicated, precedence, deficient-destination), then
  stable resource/code keys.
- **Rationale**: Existing findings serialize only their current fields when optional
  audit fields are empty. This supports an audit-specific category and fix while
  preserving existing hostname golden output. A stable sort supports repeatable
  golden reports and scenarios.
- **Alternatives considered**: Replace `model.Finding` with an incompatible audit
  finding type; rejected because the user requires reuse. Add non-optional fields;
  rejected because that would change existing hostname JSON/YAML.

## Endpoint Health

- **Decision**: Count ready endpoint addresses only from EndpointSlices belonging to
  the referenced Service. If offline input has no EndpointSlices, expose an explicit
  skipped/unavailable health-check state and do not claim success. A route winning
  for traffic with zero ready addresses is an error; a fully shadowed route with the
  same deficiency is a warning.
- **Rationale**: The audit perimeter includes Services and EndpointSlices but not
  Pods. EndpointSlice readiness is sufficient for the required check and avoids
  unrelated Pod reads. The severity policy follows the clarification.
- **Alternatives considered**: Fetch Pods to infer readiness; rejected because
  readiness is already expressed by endpoint conditions and Pods are outside the
  requested audit perimeter.

## CLI and Behavior-Test Integration

- **Decision**: Add an analyze command with command-local flags and an injectable
  audit service. Keep the existing hostname command and test command constructors
  and argument contracts intact. Extend `internal/behavior` through an optional
  audit callback/result dependency; the behavior package must not import
  `internal/cli`. Preserve existing step suggestion behavior and JUnit serialization.
- **Rationale**: Cobra gives subcommands precedence over positional root arguments,
  and the literal hostname `analyze` is the documented exception. A narrow
  dependency lets Gherkin tests inspect offline audit results without coupling
  behavior to CLI errors or command construction.
- **Alternatives considered**: Route audit scenarios through the hostname trace;
  rejected because they would not model Gateway identity, audit finding categories,
  or route winners. Import `internal/cli` from behavior; rejected because it reverses
  the existing dependency direction and risks a cycle.

## Quality and Regression Strategy

- **Decision**: Add fixture-backed table tests and new output goldens for each
  format; never modify existing feature 001/002 `.feature` or golden files. After
  each code-changing implementation phase, run the full race test, vet, lint,
  existing binary-based Gherkin commands, and exact root-output golden diffs before
  starting the next phase.
- **Rationale**: Shared model/source/resolve/behavior changes have broad regression
  risk, and the user explicitly sets a blocking validation gate.
- **Alternatives considered**: Defer full regression checks until implementation
  completion; rejected because it would let a compatibility break propagate across
  later work.
