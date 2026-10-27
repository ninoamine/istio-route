# Feature Specification: Hostname Traffic Routing

**Feature Branch**: `001-hostname-routing`

**Created**: 2026-10-26

**Status**: Draft

**Input**: User description: "A read-only CLI that takes a hostname and explains its Kubernetes traffic routing across Istio and Gateway API resources, including entry points, rules, destinations, workloads, binding health, warnings, cluster and offline modes, and text/JSON/YAML/Mermaid output."

## Clarifications

### Session 2026-10-26

- Q: Which Pods should the report show as being behind a destination Service? → A:
  Show actual Service endpoints and their Pod status, distinguishing ready from not
  ready.
- Q: How should the report represent a ServiceEntry that matches or is referenced by a
  route? → A: Show it as an external destination with configured host, ports, location,
  and resolution; Kubernetes Service and Pod details do not apply.
- Q: Should `[Istio]` or `[Gateway API]` appear on every tree node, or can child nodes
  inherit the origin label from their gateway branch? → A: Every node explicitly
  includes its origin label.
- Q: What response-time target should hostname analysis meet when inspecting a cluster
  with 10,000 Services? → A: p95 under 10 seconds.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Trace a hostname to workloads (Priority: P1)

As a cluster operator, I can enter a hostname and see how a request can travel from
its cluster entry point through routing rules to destination Services and pods, so I
can understand and troubleshoot the route without manually correlating resources.

**Why this priority**: The end-to-end explanation is the CLI's primary value and is
useful even before advanced warnings or alternate output formats are added.

**Independent Test**: Provide known Istio and Gateway API resource sets for a hostname
and verify the report shows the entry points, ordered rules, weighted destinations,
Services, actual Service endpoint Pods, and their relevant readiness status.

**Acceptance Scenarios**:

1. **Given** a hostname covered by an Istio Gateway and VirtualService, **When** the
   operator inspects it, **Then** the report shows the matching server, listener
   details, VirtualService rules in their defined order, DestinationRule subsets and
   weights, and destination Services with their actual endpoint Pods and readiness.
2. **Given** a hostname covered by a Gateway API Gateway and HTTPRoute, **When** the
   operator inspects it, **Then** the report shows the GatewayClass and controller,
   matching listener details, attached route rules and filters in their defined order,
   backend weights, and destination Services with their actual endpoint Pods and
   readiness.
3. **Given** both API families define routes for the hostname, **When** the operator
   inspects it, **Then** both paths are shown with every text node labeled by origin
   and the possible conflict is warned about.

---

### User Story 2 - Diagnose binding and route problems (Priority: P2)

As a cluster operator, I can see whether routes are attached and usable, along with
plain-language warnings, so I can identify why traffic may not reach its intended
destination.

**Why this priority**: A resource inventory alone does not explain common failures;
binding conditions and actionable warnings turn the output into a troubleshooting aid.

**Independent Test**: Supply fixtures covering rejected routes, missing parents,
missing grants, unmatched hostnames, invalid weights, and overlapping host patterns;
verify each diagnosis is represented with the related resource and an understandable
explanation.

**Acceptance Scenarios**:

1. **Given** an HTTPRoute with parent references and Accepted or ResolvedRefs
   conditions, **When** it is inspected, **Then** the report shows each parent and
   condition and explains failed conditions in plain language.
2. **Given** a Gateway with a Programmed condition, **When** an attached route is
   inspected, **Then** the report shows the condition and explains when the Gateway is
   not programmed.
3. **Given** an HTTPRoute references a Gateway or listener that does not exist, or a
   cross-namespace backend without an authorizing ReferenceGrant, **When** the hostname
   is inspected, **Then** a warning identifies the unresolved reference and its cause.
4. **Given** no route matches the hostname, **When** it is inspected, **Then** the
   report explains that no route was found and the CLI returns the documented
   hostname-not-found exit status.

---

### User Story 3 - Inspect live or offline configuration (Priority: P3)

As an operator or developer, I can inspect a live cluster or a set of YAML manifests,
so I can troubleshoot installed configuration or review a proposed configuration
without requiring cluster access.

**Why this priority**: Supporting both modes makes the explanation useful during
incident response and before configuration is deployed.

**Independent Test**: Run the same representative resource set as offline manifests
and in a test cluster, then verify equivalent configuration findings while live-only
status is reported only when available.

**Acceptance Scenarios**:

1. **Given** a reachable cluster, **When** the operator supplies kubeconfig, context,
   and optionally namespace, **Then** the CLI inspects the selected context and
   namespace, defaulting to all namespaces.
2. **Given** one or more YAML manifest files, **When** the operator uses offline mode,
   **Then** the CLI reports findings from those manifests without connecting to or
   modifying a cluster.
3. **Given** one supported resource family is not installed in a live cluster, **When**
   the hostname is inspected, **Then** the CLI skips that family with an informational
   message and still reports resources from available families.
4. **Given** offline manifests do not include pod resources or live status, **When**
   the route is inspected, **Then** the report distinguishes unavailable status from
   a failed or absent workload.

---

### User Story 4 - Choose a useful report format (Priority: P3)

As an operator, I can view a readable colored tree or select a machine-readable or
diagram format, so I can use the explanation interactively or in downstream tooling.

**Why this priority**: Multiple output forms make the same route analysis usable in
terminals, automation, and documentation.

**Independent Test**: For the same input resources, verify that text, JSON, YAML, and
Mermaid outputs represent the same route facts and warnings.

**Acceptance Scenarios**:

1. **Given** a valid hostname analysis, **When** no output format is selected, **Then**
   the CLI prints a readable tree with colored origin labels when color is enabled.
2. **Given** the operator selects JSON, YAML, or Mermaid, **When** analysis completes,
   **Then** the CLI emits that format to standard output without mixing logs or errors
   into the result.
3. **Given** `NO_COLOR` is set or standard output is not a terminal, **When** text is
   printed, **Then** the output contains no color control sequences.
4. **Given** the operator uses the default text output, **When** the hostname has
   matching resources, **Then** the report follows the structure in Expected Output
   Examples: a tree with each node labeled `[Istio]` or `[Gateway API]`, rules numbered
   in evaluation order, weights shown per backend, status conditions marked `✔` or
   `✘`, and warnings listed at the end with the `⚠` prefix.

## Expected Output Examples

The default text output for a hostname served by both API families follows this
structure:

```text
shop.example.com
├─ [Istio] Gateway istio-system/public-gw  :443 HTTPS (TLS SIMPLE, secret shop-tls)
│  └─ [Istio] VirtualService shop/shop-vs
│     ├─ [Istio] 1. prefix /api  → api-svc.shop:8080  subset v1 (90%) · v2 (10%)
│     └─ [Istio] 2. (default)    → web-svc.shop:80 (100%)
└─ [Gateway API] Gateway infra/edge-gw (class: istio)  listener https :443  ✔ Programmed
   └─ [Gateway API] HTTPRoute shop/shop-route  ✔ Accepted ✔ ResolvedRefs
      ├─ [Gateway API] 1. path prefix /v2  header x-canary=true
      │     [Gateway API] filter: URLRewrite /v2 → /
      │     [Gateway API] → api-v2.shop:8080 (100%)
      └─ [Gateway API] 2. (default)
            [Gateway API] → web.shop:80 (100%)
⚠ shop.example.com is served by both an Istio VirtualService and a Gateway API HTTPRoute
```

### Edge Cases

- A hostname may be an exact name or covered by a wildcard such as `*.example.com`.
  Wildcards are considered for Istio hosts, Istio Gateway servers, Gateway API
  listeners, and HTTPRoute hostnames.
- An Istio host may be namespace-qualified as `namespace/hostname`; its namespace
  component is considered during matching.
- A Gateway API route attaches only when its hostname intersects with a listener
  hostname, subject to its parent reference and listener route-namespace constraints.
- A more-specific hostname and a wildcard may both match; report the overlap and warn
  when a wildcard may shadow the more-specific rule rather than hiding either result.
- A route may refer to a missing Gateway, a missing listener, a missing backend, or a
  backend in another namespace. Report unresolved references distinctly.
- A referenced cross-namespace backend is authorized only when an applicable
  ReferenceGrant allows the reference; report the grant and its result.
- An Istio VirtualService may not bind to a Gateway, or may target a Gateway that is
  not found; report it as unbound or unresolved.
- A DestinationRule may be absent for a referenced subset; report the subset and
  missing rule.
- Report Istio destination weights that do not sum to 100 and Gateway API backend
  weights that are all zero as warnings; retain the configured values.
- A resource family may be unavailable because its CRDs are not installed; continue
  with available families and clearly identify the skipped family.
- No matching route is different from a route whose binding or references are invalid;
  report the former as hostname not found and the latter with diagnostics.
- Offline input may be incomplete or omit Pods and live status. Do not present
  unavailable information as healthy, failed, or present.
- A Pod that matches a Service selector but is not an actual Service endpoint is not
  reported as receiving traffic through that Service.
- A ServiceEntry destination represents an external service, not a Kubernetes Service
  or Pod workload; show its configured host, ports, location, and resolution instead.
- Do not print Secret contents. For TLS, show the configured certificate Secret
  reference and report existence or status only when that information is available.
- Duplicate resources or multiple routes may describe overlapping hostnames; retain
  resource identity and show potential conflicts instead of silently choosing one.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The CLI MUST accept one hostname as the subject of an analysis and
  identify resources whose hostnames match it.
- **FR-002**: The CLI MUST support cluster mode with kubeconfig, context, and optional
  namespace selection; when namespace is not specified, it MUST inspect all
  namespaces.
- **FR-003**: The CLI MUST support offline mode with one or more YAML manifest files
  and MUST perform no cluster connection in that mode.
- **FR-004**: The CLI MUST inspect Istio Gateway, VirtualService, DestinationRule,
  and ServiceEntry resources. When a ServiceEntry matches the queried hostname or is
  referenced as a route destination, the CLI MUST show it as an external destination
  with its configured host, ports, location, and resolution, and MUST NOT imply that
  Kubernetes Service or Pod details apply.
- **FR-005**: The CLI MUST inspect Gateway API GatewayClass, Gateway, HTTPRoute, and
  ReferenceGrant resources.
- **FR-006**: For each matching entry point, the CLI MUST report the originating API,
  Gateway identity, listener or server name, port, protocol, TLS mode and certificate
  Secret reference, and applicable namespace or route constraints.
- **FR-007**: For Gateway API entry points, the CLI MUST report the GatewayClass and
  its controller.
- **FR-008**: The CLI MUST match exact and wildcard hostnames for the supported
  resource fields and MUST account for namespace-qualified Istio hosts.
- **FR-009**: The CLI MUST report VirtualService URI, header, and method matches;
  rewrites, redirects, timeouts, retries; and destinations, weights, subsets, and
  applicable DestinationRules.
- **FR-010**: The CLI MUST report HTTPRoute path, header, query-parameter, and method
  matches; request-header-modifier, redirect, URL-rewrite, and mirror filters;
  timeouts; and backend references and weights.
- **FR-011**: Rules and matches MUST be presented in their API-defined or declared
  evaluation order when one exists. The CLI MUST preserve resource identity and MUST
  NOT imply a single global evaluation order across independent resources when none is
  defined.
- **FR-012**: The CLI MUST show Services and their actual endpoints behind reported
  destinations, including the endpoint Pods and available readiness or status. It
  MUST distinguish ready from not-ready endpoints. A Pod that only matches a Service
  selector MUST NOT be presented as an actual endpoint. If the data source does not
  provide endpoint or status information, the report MUST label it unavailable rather
  than infer a status.
- **FR-013**: For each HTTPRoute parent reference, the CLI MUST report the referenced
  parent and available Accepted and ResolvedRefs conditions. It MUST report the
  Gateway's available Programmed condition.
- **FR-014**: The CLI MUST explain binding and reference failures in plain language,
  including hostname intersection failures, missing parent Gateways or listeners, and
  unresolved backend references.
- **FR-015**: The CLI MUST report warnings for no matching route, unbound Istio
  VirtualServices, missing HTTPRoute parent Gateways or listeners, subsets without
  DestinationRules, configured weight issues, wildcard overlap or shadowing, missing
  ReferenceGrants, and hostnames served by both Istio and Gateway API.
- **FR-016**: For cross-namespace Gateway API backends, the CLI MUST identify the
  relevant ReferenceGrant, if any, and report whether it allows the reference.
- **FR-017**: When a resource family is not installed, the CLI MUST skip that family,
  emit an informational message, and continue analysis with the remaining available
  families.
- **FR-018**: The CLI MUST provide colored tree-style text by default and support
  JSON, YAML, text, and Mermaid outputs. Every node in text output MUST identify its
  origin as `[Istio]` or `[Gateway API]`. Default text output MUST number rules in
  evaluation order, show weights per backend, mark status conditions with `✔` or `✘`,
  and list warnings at the end with the `⚠` prefix. Origin labels MUST appear on every
  resource, rule, filter, and destination node; they MUST NOT be inherited from a
  parent branch. The queried-hostname root and standalone warning lines are not
  resource nodes.
- **FR-019**: The CLI MUST honor `NO_COLOR` and disable color when standard output is
  not a terminal. Results MUST be written to standard output; informational messages,
  warnings, and errors MUST be written to standard error.
- **FR-020**: The CLI MUST be read-only and MUST NOT modify cluster resources or
  request permissions beyond Kubernetes `get` and `list`.
- **FR-021**: When no supported route matches the hostname, the CLI MUST provide an
  actionable explanation and use the documented hostname-not-found exit code.
- **FR-022**: The CLI MUST NOT expose TLS Secret contents in any output format.

### Key Entities *(include if data involved)*

- **Hostname query**: The requested DNS name and its exact or wildcard match
  relationships to routing resources.
- **Entry point**: An Istio server or Gateway API listener that can accept traffic
  for the hostname, with protocol, port, TLS, and attachment constraints.
- **Route**: An Istio VirtualService or Gateway API HTTPRoute, including its parent
  bindings, ordered match rules, filters or actions, and status conditions.
- **Destination**: An Istio destination and optional subset, a Gateway API backend
  reference, or an external ServiceEntry destination, with applicable namespace,
  weight, authorization status, and external service details.
- **Workload**: A Kubernetes Service and the Pods associated with it, together with
  available readiness or status information.
- **ReferenceGrant**: A Gateway API authorization record that may allow a
  cross-namespace route reference to a backend.
- **Finding**: An informational message, warning, or error tied to a resource or
  missing relationship and expressed in user-understandable language.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In a test set containing each supported resource family and all
  requested route fields, every configured entry point, rule, and destination is
  represented in the report, including available Service endpoints and their
  readiness status.
- **SC-002**: In a validation set containing exact names, wildcard names, and
  namespace-qualified Istio hosts, every expected match and non-match is reported
  correctly.
- **SC-003**: In fixtures covering each warning in FR-015 and each binding failure in
  FR-014, the CLI reports the expected finding and identifies the related resource in
  every case.
- **SC-004**: All four output formats represent the same configured destinations and
  findings for the same input, with every text-tree node carrying its API origin.
- **SC-005**: In offline mode, 100% of findings are derived from supplied manifests,
  no cluster connection is attempted, and unavailable workload status is explicitly
  identified.
- **SC-006**: In a cluster missing either resource family, the available family still
  produces a report and the absent family produces an informational message rather
  than terminating the analysis.
- **SC-007**: In read-only verification, the CLI performs zero cluster write
  operations and requests no permissions beyond `get` and `list`.
- **SC-008**: An operator can use the report to identify the matching entry point,
  route, and final destination for a representative hostname without manually
  correlating the underlying resources.
- **SC-009**: For every representative default-text report, nodes carry the required
  origin label, rules are numbered in evaluation order, backend weights are visible,
  status conditions use `✔` or `✘`, and warnings appear at the end prefixed by `⚠`.
- **SC-010**: In a dedicated three-worker Kubernetes test cluster, each worker
  provisioned with 4 vCPU and 8 GiB memory, at least 95 of 100 measured hostname
  analyses complete within 10 seconds. Before measurement, load 10,000 Services
  across 100 namespaces, 100 Istio VirtualServices, and 100 Gateway API HTTPRoutes.
  Configure exactly one VirtualService and one HTTPRoute to match the measured
  hostname, both referencing the same two Services, each with three ready Pod
  endpoints; the remaining routes use non-matching hostnames. Run 10 unmeasured
  warm-up analyses before 100 sequential measured analyses, and record cluster
  version and worker specifications with the results. The performance
  test MUST use a live test cluster, not a fake client; any fixture provisioning is
  performed by the test harness, while the CLI under test remains read-only.

## Assumptions

- The intended users are Kubernetes operators and developers familiar with cluster
  routing concepts; resource names are retained alongside plain-language explanations.
- Cluster mode reports live resource and status data available to the caller. It does
  not retrieve or display TLS Secret contents.
- Offline mode derives findings only from supplied manifests. It reports Pod and status
  data only when those objects and fields are present in the input; it does not infer
  cluster state.
- Within each resource, the CLI follows API-defined or declared order. Independent
  resources from different APIs do not necessarily have one globally defined
  precedence; the CLI reports them separately and warns about potential conflicts.
- A hostname with no matching route is treated as not found (exit code 3); a matched
  route with unhealthy bindings or unresolved references remains a diagnostic report.
- Gateway API, Istio, and Kubernetes resource schemas are interpreted according to
  their installed or supplied API versions. Unsupported or malformed input is reported
  as an actionable error rather than silently ignored.
- The CLI does not apply, edit, or recommend automatic changes to cluster resources.
- Performance validation uses the fixed workload and live-cluster procedure in
  SC-010; record the Kubernetes version and worker specifications with each result.
