# Feature Specification: Gateway-Scoped Routing Audit

**Feature Branch**: `003-gateway-routing-audit`

**Created**: 2026-11-03

**Status**: Draft

**Input**: User description: "Add a read-only `analyze` command that audits one Gateway API Gateway or Istio ambient waypoint and its linked routes, destinations, and waypoint selections from live or offline configuration."

## Clarifications

### Session 2026-11-03

- Q: If `--namespace` is omitted, how should the command choose among gateways with the requested name? → A: Use the kubeconfig namespace when set; if it is unset, require the user to provide `--namespace`.
- Q: How should the audit assign severity to rejected routes, unused grants, duplicates, shadowed rules, and backends with no ready endpoints? → A: Errors: rejected routes, missing backends, missing or unprogrammed waypoints, and no-ready-endpoint winning backends. Warnings: duplicates, unused grants, shadowed rules, and no-ready-endpoint shadowed backends. Winning precedence results are informational.
- Q: If a higher-ranked rule overlaps only part of a lower-ranked rule’s matching traffic, should the lower-ranked rule still be shown as able to serve traffic outside that overlap? → A: Report partial overlap separately; mark a rule shadowed only when higher-precedence rules cover its entire match space.
- Q: If a route targets the gateway but has no reported acceptance condition, should the audit treat acceptance as unknown rather than rejected? → A: Treat missing acceptance status as unknown; include the route but do not report it as rejected.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Audit one gateway or waypoint (Priority: P1)

As a platform or SRE engineer, I can name one Gateway or ambient waypoint and receive
an audit of the routing configuration linked to it, so I can identify routing defects
without scanning or changing the rest of the cluster.

**Why this priority**: A scoped, actionable report is the core user value and must work
for both ordinary gateways and waypoints.

**Independent Test**: Audit a healthy gateway and a waypoint with linked manifests;
verify that only resources in their perimeter are included and that the healthy
gateway has no findings.

**Acceptance Scenarios**:

1. **Given** a unique gateway name, **When** the engineer runs `analyze`, **Then** the
   report identifies the Gateway or waypoint, its class and listeners, and the linked
   routing configuration.
2. **Given** an offline manifest set, **When** the engineer audits a gateway, **Then**
   only resources present in that set are analyzed and no cluster access is attempted.
3. **Given** a live cluster, **When** the engineer audits a gateway, **Then** the
   command reads the gateway, discovers routes that target it, and inspects only the
   related namespaces, services, endpoint slices, and grants.
4. **Given** a namespace or service selects a waypoint using an ambient waypoint
   label, **When** that waypoint is audited, **Then** the report includes the selector
   and identifies when its target waypoint is absent or not programmed.

### User Story 2 - Find orphaned or duplicate routes (Priority: P1)

As an engineer maintaining a gateway, I can see routes that are rejected, have
unresolvable backends, misuse reference grants, or duplicate another route, so I can
correct configuration that is unusable or redundant.

**Why this priority**: These configuration defects directly prevent intended routing
or make behavior difficult to maintain.

**Independent Test**: Audit manifests containing a rejected route, missing backend
service or port, unused ReferenceGrant, and exact duplicate; verify each finding names
the affected resources and explains the condition.

**Acceptance Scenarios**:

1. **Given** HTTPRoutes or GRPCRoutes that target the named gateway but are not
   accepted, **When** the engineer audits it, **Then** each route is reported with
   its rejection reason.
2. **Given** a route backend that references a missing service or port, **When** the
   engineer audits it, **Then** the affected route rule and backend are reported.
3. **Given** a ReferenceGrant relevant to the gateway's routes that authorizes no
   reference in use, **When** the engineer audits it, **Then** it is reported as
   unused.
4. **Given** two routes on the same listener with the same hostname and match
   conditions, **When** the engineer audits the gateway, **Then** the redundant
   route rules are identified as duplicates.

### User Story 3 - Understand precedence and destination health (Priority: P1)

As an engineer investigating traffic behavior, I can see which matching route rule
wins and which rules are shadowed, along with whether the chosen backend has ready
endpoints, so I can distinguish precedence problems from unavailable destinations.

**Why this priority**: Knowing the effective rule and destination health explains
whether a configured route can actually receive traffic.

**Independent Test**: Audit overlapping routes and a winning route whose backend has
no ready endpoints; verify the winner, shadowed rules, backend health, and relative
severity.

**Acceptance Scenarios**:

1. **Given** overlapping route rules, **When** the engineer audits their gateway,
   **Then** the report identifies the winning rule according to match specificity,
   creation age, and namespace/name order, and marks rules that never receive traffic
   as shadowed.
2. **Given** a route backend service whose endpoint slices contain zero ready
   addresses, **When** the engineer audits the gateway, **Then** a deficient
   destination finding identifies the route, rule, backend, and reason.
3. **Given** an unhealthy backend on the winning route and an unhealthy backend on a
   shadowed route, **When** the findings are compared, **Then** the winning route's
   deficiency has higher severity.
4. **Given** offline manifests without EndpointSlices, **When** the engineer audits
   destinations, **Then** endpoint health is reported as skipped or unavailable,
   not as a passing health check.

### User Story 4 - Use the audit in automation and scenario tests (Priority: P2)

As an engineer or CI maintainer, I can select a report format and assert audit results
in offline scenarios, so that gateway changes can be reviewed consistently by people
and automated checks.

**Why this priority**: Multiple output formats and repeatable assertions make the
audit useful in terminals, pipelines, and regression suites.

**Independent Test**: Run representative audit scenarios for healthy, rejected,
missing-backend, unused-grant, duplicate, precedence, destination-health, waypoint,
and unknown-gateway cases; verify finding assertions and each supported output format.

**Acceptance Scenarios**:

1. **Given** an audit result, **When** the engineer requests text, JSON, YAML, or
   Mermaid output, **Then** each format conveys the same findings, and Mermaid shows
   the gateway-to-route-to-backend relationships and marks winning, shadowed, and
   deficient routes.
2. **Given** an offline feature scenario, **When** it audits a gateway, **Then** the
   scenario can assert finding code, severity, category, involved resource, absence
   of a finding, and whether a route wins or is shadowed.
3. **Given** a healthy gateway with no findings, **When** the engineer runs the audit,
   **Then** the command exits successfully; when findings are present, **Then** it
   returns the documented findings exit status.
4. **Given** an unknown gateway or a name ambiguous across namespaces, **When** the
   engineer runs the audit, **Then** the command reports not-found or lists the
   candidate namespaces clearly.

### Edge Cases

- The gateway or waypoint is absent from offline manifests or unavailable in the
  selected live cluster.
- A gateway name matches resources in multiple namespaces and no namespace is given.
- The kubeconfig has no default namespace and the user omits `--namespace`.
- Gateway API resource families are unavailable because their CRDs are not installed.
- A route references a gateway but has no acceptance status, a rejected status, or a
  listener-specific acceptance result.
- Route matching conditions partially overlap, are fully redundant, or tie on
  specificity and creation time.
- A backend service exists but the referenced port does not, or a cross-namespace
  backend is not authorized by a ReferenceGrant.
- No EndpointSlices are provided offline, or supplied slices have no ready addresses.
- Waypoint labels use a namespace-qualified waypoint name, identify a nonexistent
  waypoint, or identify one that is not programmed.
- A live source denies access or encounters a read or parse error.
- Output selection or command arguments are invalid.
- The hostname root command receives the literal word `analyze`, which is reserved
  for the subcommand; other hostnames retain existing parsing behavior.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The CLI MUST provide `istio-route analyze [flags] <gateway-name>` for
  one Gateway API Gateway or Istio ambient waypoint. It MUST support namespace
  selection with `-n`/`--namespace`, offline manifests with `-f`/`--file` including
  `-` for standard input, `--kubeconfig`, `--context`, `--output text|json|yaml|mermaid`,
  and `--no-color`.
- **FR-002**: The audit MUST be scoped to the named gateway and resources linked to
  it. It MUST NOT perform a whole-cluster resource scan.
- **FR-003**: The audit MUST include the Gateway, its GatewayClass and listeners,
  HTTPRoutes and GRPCRoutes whose parent references target it (including rejected
  routes), route backend services and their EndpointSlices, and ReferenceGrants
  relevant to references made by those routes. For a waypoint, it MUST also include
  namespaces and services selecting it through ambient waypoint labels, including
  labels that qualify a waypoint in another namespace.
- **FR-004**: The audit MUST NOT include Istio VirtualServices, DestinationRules, or
  AuthorizationPolicies, or sidecar-specific analysis.
- **FR-005**: In live mode, the audit MUST read the named gateway, list applicable
  route families, retain routes whose parent references target that gateway, and
  read only the namespaces, services, endpoint slices, and grants referenced by the
  discovered routes and waypoint labels. Live access MUST be read-only and use only
  get/list permissions; it MUST NOT read Secrets or modify resources.
- **FR-006**: In offline mode, the audit MUST analyze only the supplied manifests. If
  the Gateway or waypoint is absent, it MUST report that it was not found. If
  EndpointSlices are absent, endpoint-health evaluation MUST be explicitly reported
  as skipped or unavailable rather than passed.
- **FR-007**: The audit MUST explain when Gateway API resource families are
  unavailable because their CRDs are missing, consistent with the existing CLI's
  unavailable-family reporting.
- **FR-008**: The audit MUST report orphaned routes that the named gateway explicitly
  rejects and include the available rejection reason. A route without an acceptance
  condition MUST remain in the audit with acceptance shown as unknown, not be
  reported as rejected. The audit MUST report backend references to a missing
  service or port, relevant ReferenceGrants that authorize no reference in use, and
  namespace or service waypoint selectors that point to an absent or unprogrammed
  waypoint.
- **FR-009**: The audit MUST report duplicate route rules on the same listener when
  their hostname and path, header, method, and query match conditions are identical,
  including fully redundant rules.
- **FR-010**: For every set of overlapping route rules, the audit MUST identify the
  winning rule and rules that are shadowed and never receive traffic. Precedence MUST
  use match specificity, then the oldest creation timestamp, then namespace/name
  order. The audit MUST distinguish partial overlap from full shadowing: a rule MUST
  be marked shadowed only when higher-precedence rules cover its entire match space.
  When overlap is partial, the audit MUST identify the higher-precedence rule for
  intersecting requests while showing that the lower-precedence rule remains eligible
  for requests outside the overlap.
- **FR-011**: The audit MUST identify a deficient destination when a backend service
  has zero ready addresses across its EndpointSlices. The finding MUST identify the
  route, rule, backend, and reason. A deficient destination on the winning route MUST
  have higher severity than one on a shadowed route.
- **FR-012**: Every finding MUST have a stable kebab-case code, severity (`info`,
  `warning`, or `error`), category, involved resources identified by kind, namespace,
  and name, an explanation, and a suggested fix. Existing finding codes MUST be
  reused when the condition is the same. Rejected routes, missing backends, missing
  or unprogrammed waypoints, and no-ready-endpoint winning backends MUST be errors.
  Duplicates, unused grants, shadowed rules, and no-ready-endpoint shadowed backends
  MUST be warnings. A winning precedence result MUST be informational. Findings
  MUST be ordered by severity and then category.
- **FR-013**: The audit MUST support text, JSON, YAML, and Mermaid output. Mermaid
  output MUST draw gateway-to-route-to-backend-service relationships and mark winning,
  shadowed, and deficient routes. Text output MUST follow existing color conventions
  and honor `NO_COLOR` and non-terminal output.
- **FR-014**: An omitted namespace MUST default to the kubeconfig namespace when one
  is set. If the kubeconfig has no default namespace, the command MUST require
  `--namespace`; omitting it MUST be reported as invalid usage. A name that is
  unknown MUST produce a clear not-found message. A name shared by gateways in
  multiple namespaces MUST fail with a clear message listing candidates when lookup
  is not namespace-scoped.
- **FR-015**: The analyze command MUST return exit code 0 when there are no findings,
  1 for runtime, source, or parse errors, 2 for invalid usage, 3 when the gateway is
  not found, and 4 when findings are present.
- **FR-016**: Adding the subcommand MUST preserve existing hostname-trace and test
  command arguments, flags, output, and exit codes: hostname tracing MUST retain
  codes 0, 1, 2, and 3, and the test command MUST retain codes 0, 1, and 2. The word
  `analyze` is reserved as a subcommand name; parsing of all other hostnames MUST
  remain unchanged.
- **FR-017**: Existing hostname-trace finding codes, JSON/YAML field names, and text
  and Mermaid output MUST remain unchanged. Shared behavior and finding-model
  extensions MUST be additive.
- **FR-018**: Existing hostname-trace golden files and feature 001 scenarios, and
  existing feature 002 scenarios, MUST pass unchanged. Existing expected content
  MUST NOT be edited. The test command's extension MUST preserve existing step
  meanings and closest-step suggestions, and MUST preserve existing JUnit report
  format.
- **FR-019**: The test command's fixed step vocabulary MUST allow scenarios to audit
  a gateway and assert findings by code, severity, category, involved resource, and
  absence, as well as assert which route wins or is shadowed. The step reference and
  README MUST document the new steps, including the reference at
  `specs/002-routing-behavior-testing/step-reference.md`.
- **FR-020**: Test fixtures and scenarios MUST cover a healthy gateway with no
  findings, a rejected route, a missing backend service, an unused ReferenceGrant,
  an exact duplicate, overlapping winning and shadowed routes, a winning route with
  no ready endpoint addresses, a waypoint selected by a namespace label, a waypoint
  label pointing to a missing waypoint, and an unknown gateway name. Manifest
  fixtures MUST be placed under `testdata/manifests` and scenarios under
  `testdata/features`. Golden files MUST cover each supported output format.
- **FR-021**: Any required RBAC additions, including access to namespaces or
  GRPCRoutes, MUST be documented in the deployment RBAC and README. RBAC changes MUST
  only add get/list permissions and MUST NOT remove existing permissions or broaden
  permissions beyond get/list.
- **FR-022**: The README MUST document the analyze command, its flags, output formats,
  exit codes, RBAC requirements, and the reserved `analyze` hostname limitation.

### Key Entities

- **Gateway or waypoint**: The named entry point being audited, including its
  namespace, class, listeners, and programming state.
- **Route rule**: An HTTPRoute or GRPCRoute parented to the audited gateway, with its
  listener association, hostname, matching conditions, acceptance, and backend
  references.
- **Backend destination**: A referenced service and port, with endpoint readiness
  where EndpointSlices are available.
- **Waypoint selection**: A namespace or service label that selects an ambient
  waypoint, including its namespace qualification and target status.
- **ReferenceGrant**: A cross-namespace authorization relevant to route references,
  including whether the authorization is used.
- **Finding**: An actionable audit result with stable code, severity, category,
  involved resources, explanation, and suggested fix.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All ten required audit cases (healthy, rejected, missing backend,
  unused grant, duplicate, precedence, deficient destination, selected waypoint,
  missing waypoint, and unknown gateway) produce the expected result in automated
  offline scenarios.
- **SC-002**: For a report with findings, all four output formats identify the same
  finding codes, severities, categories, and involved resources.
- **SC-003**: Every overlap case in the test scenarios identifies exactly the
  expected winner and all expected shadowed rules.
- **SC-004**: Every existing feature 001 and feature 002 scenario and every existing
  hostname-trace golden output passes without changes to its expected content.
- **SC-005**: Users can distinguish a clean audit, an audit with findings, invalid
  usage, an execution error, and a missing gateway by the documented exit code and
  an actionable message.

## Assumptions

- `analyze` is a new top-level subcommand, consistent with the existing `test`
  command, and therefore takes precedence over interpreting the literal hostname
  `analyze`.
- A missing gateway in offline mode uses the documented gateway-not-found result;
  malformed manifests remain source or parse errors.
- Endpoint health can be assessed only from supplied EndpointSlices offline and from
  the linked services' EndpointSlices in live mode.
- A route counts as attached to the audit perimeter when its parent reference
  identifies the named gateway, regardless of acceptance status.
- The existing Finding model and rendering conventions remain the compatibility
  baseline; new audit findings extend them without changing existing hostname-trace
  output.
- Existing live-mode namespace and kubeconfig conventions are reused; gateway
  lookup requires an explicit namespace when the kubeconfig does not provide one.
