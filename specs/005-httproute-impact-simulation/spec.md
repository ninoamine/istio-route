# Feature Specification: HTTPRoute Removal Impact Simulation

**Feature Branch**: `005-httproute-impact-simulation`

**Created**: 2026-11-10

**Status**: Draft

**Input**: User description: "Add a new `simulate` CLI subcommand to simulate the removal of an HTTPRoute and report its routing impact. Strictly read-only: never delete, patch, create, or modify Kubernetes resources, never modify or generate modified local manifest files; the removal is simulated only in memory. Support local manifest mode and live cluster mode, consistent with existing CLI conventions. Only operate in Istio Ambient Mesh mode: the user selects a specific ambient waypoint (a cluster may contain several waypoints and regular ingress Gateways, and not every Gateway API Gateway is a waypoint), the tool identifies the HTTPRoutes attached to that waypoint, and the user selects one by namespace and name. Routing is re-evaluated as if that HTTPRoute were absent, and the network-level impact is reported by comparing routing before and after: affected hostnames, HTTP paths and methods, routing rules, referenced backend Services and namespaces, whether matching traffic loses its route, and whether other HTTPRoutes still provide matching routes. Pods, workloads, and application health are out of scope. Reuse existing routing and attachment analysis; do not redesign existing commands."

## Clarifications

### Session 2026-11-10

- Q: How should the simulation handle HTTPRoutes whose attachment cannot be confirmed because status is missing or stale, as is common in local manifests? → A: Evaluate the available configuration provisionally; mark the attachment and any affected impact as unconfirmed rather than asserting a definitive outcome.
- Q: How should the simulation report routing impact when the supplied manifests may not include every route attached to the waypoint? → A: Report results relative to the supplied snapshot, warn that its completeness is unverified, and mark affected outcomes undetermined when a detected information gap prevents a conclusion.
- Q: Which HTTPRoutes should be included when a Service uses the selected waypoint but its HTTPRoute names the Service, rather than the waypoint, as its parent? → A: Analyze only HTTPRoutes directly parented to the selected waypoint; disclose that Service-parented routing is excluded.
- Q: How should the report classify a rule when different parts of its traffic have different outcomes after removal? → A: Classify each distinguishable traffic portion separately and summarize the combined outcomes for each rule.
- Q: What should happen when a selected Gateway names the `istio-waypoint` class, but the GatewayClass definition is missing from the available data? → A: Continue provisionally, mark waypoint validation unconfirmed, and mark dependent impact outcomes undetermined.
- Q: How should the simulation handle HTTPRoutes attached to a waypoint listener using HBONE, the protocol commonly used by ambient waypoints? → A: Reject waypoints with HBONE listeners as unsupported. (Superseded after analysis; see the next bullet.)
- Q: Should HBONE listeners remain unsupported, given that typical ambient waypoints expose only an HBONE listener? → A: No. Treat HBONE listeners as applicable listeners for direct parent references to the waypoint; when a listener's restrictions cannot be established, report the attachment as unconfirmed.
- Q: How should the simulation handle an HTTPRoute that is accepted on one listener of the selected waypoint but rejected or unconfirmed on another? → A: Evaluate each listener independently, excluding rejected attachments and marking only affected unconfirmed portions undetermined.
- Q: When should the simulation consider an HTTPRoute attachment status current enough to confirm acceptance or rejection? → A: Require the status condition's observed generation to match the route's nonzero current generation; missing, zero, or mismatched generation information means unconfirmed.
- Q: What should happen if the selected HTTPRoute names the waypoint as a parent but is currently rejected on every applicable listener? → A: Reject the selection with a usage error, explain why, and list eligible routes.
- Q: How should local simulation handle a Gateway or HTTPRoute manifest that omits its resource namespace? → A: Reject relevant Gateway and HTTPRoute manifests with omitted namespaces and explain the input error.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - See what breaks if I remove an HTTPRoute (Priority: P1)

A platform engineer plans to delete an HTTPRoute that is attached to an ambient waypoint. Before doing so, they run the simulation, naming the waypoint and the HTTPRoute (by namespace and name). The tool re-evaluates the waypoint's routing as if that HTTPRoute did not exist and reports, for every hostname, path, and method the route served, whether the traffic would become unrouted or would still be served by another HTTPRoute, together with the rules and backend Services involved.

**Why this priority**: This is the core value of the feature: a safe, read-only "what if" answer to "what happens to traffic if I delete this route?". Every other story builds on it.

**Independent Test**: Run the simulation against a local manifest set that contains a validated waypoint and two HTTPRoutes with current confirmed attachment status, where one rule's hostname/path is fully covered by the other route and another rule's hostname/path is covered by no other route. Verify the snapshot-scoped report lists the first as "still covered" (naming the covering route) and the second as "unrouted", and that the input files are byte-identical afterwards.

**Acceptance Scenarios**:

1. **Given** a validated waypoint with one confirmed attached HTTPRoute that is the only route in scope serving `shop.example.com` path prefix `/api`, **When** the user simulates removing it, **Then** the report states that traffic for `shop.example.com` `/api` becomes unrouted within the input scope and names the removed rule and its backend Service and namespace.
2. **Given** a validated waypoint with two confirmed attached HTTPRoutes where the first wins for a hostname and `/api` path prefix and the second matches that hostname with a path prefix of `/`, **When** the user simulates removing the first, **Then** the report states that the traffic remains covered, names the second route and its rule as the new provider, and shows its backend Service and namespace.
3. **Given** confirmed attachments and an HTTPRoute rule whose traffic is partly uniquely served and partly covered by another route, **When** the user simulates its removal, **Then** the report distinguishes the unrouted and covered traffic portions and summarizes both outcomes for the rule.
4. **Given** confirmed attachments and a removed rule that was already shadowed by a higher-precedence rule from another HTTPRoute, **When** the user simulates removal, **Then** the report states that no effective routing change occurs for that rule.
5. **Given** any simulation run, **When** it completes (successfully or with an error), **Then** no cluster resource and no local manifest file has been created, modified, or deleted.

---

### User Story 2 - Discover which HTTPRoutes are attached to a waypoint (Priority: P2)

An engineer who does not remember the exact route identity asks the tool which HTTPRoutes are attached to a given waypoint. The tool lists them by namespace and name (with their hostnames) so the engineer can choose one to simulate.

**Why this priority**: Selection by namespace and name is required for Story 1; making candidates discoverable avoids guesswork and typos, but Story 1 is usable without it when the identity is already known.

**Independent Test**: Run the command with a waypoint selected but no HTTPRoute selected, against a manifest set with several routes of which only some attach to that waypoint. Verify only the attached routes are listed and no simulation is performed.

**Acceptance Scenarios**:

1. **Given** a waypoint with three attached HTTPRoutes and two HTTPRoutes attached to a different waypoint, **When** the user selects the waypoint without selecting a route, **Then** exactly the three attached routes are listed by namespace and name.
2. **Given** a waypoint with no attached HTTPRoutes, **When** the user selects it without a route, **Then** the tool reports that no HTTPRoutes are attached and exits successfully.
3. **Given** a selected HTTPRoute that exists but is not attached to the selected waypoint, **When** the user requests its simulation, **Then** the tool reports that the route is not attached to that waypoint and lists the routes that are, without simulating anything.
4. **Given** a route whose configuration permits attachment but whose status is missing or stale, **When** the user lists routes or simulates an affected route, **Then** its attachment and any impact depending on it are identified as unconfirmed, with a provisional configuration-based evaluation rather than a definitive claim.
5. **Given** an HTTPRoute parented to a Service that selects the waypoint but not directly parented to the waypoint, **When** the user lists routes or runs a simulation, **Then** that route is excluded from selection and coverage, and the tool discloses the exclusion.
6. **Given** a route accepted on listener A but rejected on listener B of the selected waypoint, **When** the user simulates removal, **Then** the route participates only on listener A; the rejection on B does not exclude it from A.
7. **Given** a route accepted on listener A and unconfirmed on listener B, **When** the user simulates removal, **Then** outcomes on A can be confirmed while outcomes depending on B are undetermined and accompanied by any provisional evaluation.
8. **Given** an attachment's Accepted condition has an observed generation different from the HTTPRoute's generation, or either generation is missing or zero, **When** the user lists routes or simulates an affected route, **Then** the attachment is unconfirmed regardless of whether that condition says accepted or rejected.
9. **Given** a route is currently rejected on every applicable listener of the selected waypoint, **When** the user selects it for simulation, **Then** the tool exits with a usage error, explains the rejection, lists eligible routes, and produces no impact report.
10. **Given** two listeners on a selected waypoint and a route whose parent reference selects only listener A, **When** the user simulates removal, **Then** the route is not treated as attached to listener B or as providing coverage there.
11. **Given** a route in another namespace and a parent reference that omits its namespace, **When** the user selects a same-named waypoint in a different namespace, **Then** that parent reference does not attach the route to the selected waypoint.

---

### User Story 3 - Select the right waypoint and reject non-waypoints (Priority: P2)

An engineer works in an environment that has several ambient waypoints and several regular ingress Gateways. They select one waypoint by namespace and name and get results scoped only to that waypoint. If they select a Gateway that is not an Istio ambient waypoint, the tool refuses to simulate and says why.

**Why this priority**: The feature is explicitly limited to Istio Ambient Mesh and must not treat every Gateway as a waypoint. Correct scoping is essential for trustworthy results but depends on the Story 1 engine.

**Independent Test**: Use a manifest set with two waypoints and one ingress Gateway, all with HTTPRoutes. Simulate the same route against each waypoint and the ingress Gateway, and verify results are scoped per waypoint and the ingress Gateway is rejected.

**Acceptance Scenarios**:

1. **Given** two waypoints that each have routes serving the same hostname, **When** the user simulates removing a route against waypoint A, **Then** the report considers only HTTPRoutes attached to waypoint A as potential replacements and never counts routes attached only to waypoint B.
2. **Given** a Gateway that is a regular ingress Gateway (not an ambient waypoint), **When** the user selects it, **Then** the tool stops with a clear message that the target is not an Istio ambient waypoint and performs no simulation.
3. **Given** a waypoint name that does not exist in the selected namespace or manifests, **When** the user selects it, **Then** the tool reports that the waypoint was not found using the same not-found exit status as the existing Gateway audit.
4. **Given** the same HTTPRoute is attached to two waypoints, **When** the user simulates its removal for waypoint A, **Then** the report covers waypoint A and notes that the route is also attached to waypoint B, whose impact is not included.
5. **Given** the selected Gateway names the `istio-waypoint` class but its GatewayClass definition is unavailable, **When** the user runs a simulation, **Then** waypoint validation is marked unconfirmed and dependent outcomes are undetermined, with any provisional evaluation clearly labeled.
6. **Given** a selected waypoint whose only listener uses the HBONE protocol, **When** the user requests a route listing or simulation, **Then** HTTPRoutes directly parented to the waypoint are evaluated against that listener, and the tool does not reject the waypoint.
7. **Given** a selected waypoint with no applicable listener for HTTPRoutes, **When** the user requests a route listing or simulation, **Then** every route is reported as ineligible with the reason "no applicable listeners".

---

### User Story 4 - Consume results in the formats the tool already supports (Priority: P3)

An engineer or CI job wants the simulation result as plain text for reading, as JSON or YAML for automation, or as a Mermaid diagram for documentation, using the same options and conventions as the existing commands (manifest files or live cluster, kubeconfig/context/namespace selection, colors, exit statuses).

**Why this priority**: Consistent output and input conventions make the feature usable in pipelines and familiar to current users, but the report content (Story 1) is the primary value.

**Independent Test**: Run the same simulation in each output format and in both local and live-cluster modes, verifying all formats convey the same classification of every affected rule and that text output honors color-disabling conventions.

**Acceptance Scenarios**:

1. **Given** a simulation, **When** the user chooses text, JSON, YAML, or Mermaid output, **Then** each format contains the same before/after impact information, and results go to standard output while diagnostics go to standard error.
2. **Given** manifest files supplied with the file option (including standard input), **When** the user runs the simulation, **Then** no cluster connection is made.
3. **Given** no manifest files, **When** the user runs the simulation, **Then** the current kubeconfig context (or the explicitly chosen kubeconfig, context, and namespace) is used and only read access is exercised.
4. **Given** color is disabled by flag, environment convention, or non-terminal output, **When** text output is produced, **Then** it contains no styling.
5. **Given** local manifests that omit a route present elsewhere, **When** the user simulates removal, **Then** the report scopes its conclusions to the supplied manifests and warns that their completeness is unverified.
6. **Given** a local Gateway manifest named like the selected waypoint or an HTTPRoute manifest that is selected or could refer to that waypoint, **When** its resource namespace is omitted, **Then** the tool reports the missing namespace as an input error and does not simulate.

---

### Edge Cases

- The selected HTTPRoute is the only confirmed route in scope for the waypoint: its previously routed traffic becomes unrouted in that scope; the report states that no in-scope routes remain.
- The HTTPRoute declares no hostnames and inherits hostnames from the waypoint listener: the report uses the inherited hostnames, or states that all hostnames are affected when the listener has none.
- A rule has no match conditions (catch-all): it is treated as matching every path and method, consistent with existing precedence behavior.
- A rule matches on headers, query parameters, or regular-expression paths in a way that cannot be compared conclusively with another route's matches: the report marks the affected traffic portion as "undetermined" instead of claiming it is covered or unrouted.
- Remaining routes cover only part of the removed rule's traffic (for example only some methods or a narrower path): the report describes distinguishable covered and unrouted portions and summarizes the rule as partially covered.
- Remaining coverage comes from a broader catch-all or lower-precedence rule: the report names the fallback route and highlights that traffic would be served by a different backend than before.
- A removed route's rules send traffic to a backend in another namespace: the backend Service and its namespace are reported as written in the route.
- The selected HTTPRoute does not exist in the manifests or cluster: the tool reports route-not-found using the existing not-found exit status.
- Required routing data cannot be read in the cluster, or manifests contain no Gateway API resources: the tool reports the unavailable input clearly instead of producing an empty or misleading result.
- The referenced backend Service is not defined in the manifests: the backend is still reported by name and namespace; existence of the Service is not required for the impact analysis.
- Manifests supplied via standard input are read once and never written back.
- A known missing input prevents a routing conclusion: the affected outcome is undetermined and the report identifies the missing information.
- Local Gateway and HTTPRoute manifests relevant to identifying the target or determining potential direct attachment must explicitly declare their resource namespaces; the waypoint namespace option does not fill them in.
- Another HTTPRoute is currently rejected or demonstrably unattached to the waypoint: it is not counted as providing coverage. Missing or stale attachment status is unconfirmed, not rejection.
- A route can have different attachment states for different listeners or parents; a rejection for one does not invalidate a confirmed attachment to another.
- If available parent or listener status does not establish which listener an acceptance applies to, affected listener attachments remain unconfirmed unless other available evidence resolves the ambiguity.
- A waypoint's HBONE listener is an applicable listener for direct parent references; if its route-kind, namespace, or hostname restrictions cannot be established, the affected attachment is unconfirmed.
- The manifests contain no Gateway API resources: the tool reports the missing input as an error rather than "waypoint not found".

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The tool MUST provide a `simulate` subcommand that simulates removal of one HTTPRoute from one Istio ambient waypoint and reports the resulting routing impact.
- **FR-002**: The simulation MUST be strictly read-only: it MUST NOT create, update, patch, or delete any cluster resource and MUST NOT modify, rewrite, or generate modified copies of any local manifest file. The removal MUST exist only in memory for the duration of the run.
- **FR-003**: The subcommand MUST support local manifest mode (one or more files, or standard input) and live-cluster mode, using the same input-selection options and defaults as the existing `analyze` subcommand (manifest files, kubeconfig, context, namespace). Local manifest mode MUST NOT contact a cluster.
- **FR-004**: The subcommand MUST operate only for Istio Ambient Mesh: the selected Gateway MUST name the `istio-waypoint` GatewayClass, consistent with the existing Gateway audit. A Gateway naming another class MUST be rejected with a clear message and no simulation. If the `istio-waypoint` GatewayClass definition is unavailable, the tool MUST identify waypoint validation as unconfirmed, continue provisionally, and mark dependent outcomes undetermined, with any provisional evaluation clearly labeled.
- **FR-005**: The user MUST be able to select a specific waypoint by namespace and name. When several waypoints exist, each MUST be selectable independently, and results for one waypoint MUST NOT include routing from routes attached only to another.
- **FR-006**: The tool MUST identify HTTPRoutes directly parented to the selected waypoint using the existing attachment analysis, and MUST let the user select one by namespace and name. Routes parented only to a Service that selects the waypoint are excluded from selection and coverage; the tool MUST disclose this exclusion, including the number of excluded routes, in listings and reports.
- **FR-007**: When a waypoint is selected without an HTTPRoute, the tool MUST list eligible HTTPRoutes for that waypoint (namespace, name, hostnames) and MUST NOT run a simulation. An eligible route has at least one confirmed or unconfirmed direct attachment to an applicable listener; unconfirmed attachments MUST be labeled.
- **FR-008**: When the selected HTTPRoute does not exist, or exists but is not eligible for the selected waypoint, the tool MUST explain the reason and list eligible routes. A missing route MUST use the existing not-found exit status. A route currently rejected on every applicable listener, or demonstrably unattached, MUST be rejected with usage status `2` and no impact report.
- **FR-009**: The tool MUST compute routing for the selected waypoint before the simulation and again with the selected HTTPRoute excluded, reusing the existing route-matching and precedence logic, and MUST report the difference.
- **FR-010**: For every rule of the removed HTTPRoute, the report MUST include the affected hostnames, HTTP path matches, HTTP methods (or "any"), the rule's position within the route, and the referenced backend Services with their namespaces.
- **FR-011**: Within the direct waypoint-parented HTTPRoute scope, the report MUST classify each distinguishable portion of a removed rule's traffic as **unrouted** (no route remains in scope), **still covered** (another attached HTTPRoute serves it), **no change** (the removed rule was not its effective winner), or **undetermined** (a definitive outcome cannot be derived from the available data). It MUST identify the listener and describe the hostname and request conditions of each portion and summarize all outcomes for each rule, including partial coverage when applicable. When portions cannot be distinguished conclusively, the affected portion MUST be undetermined rather than guessed. An outcome depending on unconfirmed attachment or waypoint validation MUST be undetermined, with any configuration-based provisional outcome clearly labeled.
- **FR-012**: For each still-covered traffic portion, the report MUST name the HTTPRoute(s) and rule(s) that now serve the traffic and their backend Services and namespaces, and MUST explicitly indicate when those backends differ from the removed rule's backends.
- **FR-013**: The report MUST present unrouted and still-covered traffic portions in clearly separate, visibly distinct groups, and MUST include summary counts for each portion classification and per-rule outcome summary.
- **FR-014**: The tool MUST evaluate attachment and coverage independently for each applicable selected-waypoint listener and MUST NOT count an attachment currently rejected or demonstrably absent for that listener as coverage there. Rejection on one listener MUST NOT exclude a confirmed attachment on another. An attachment's Accepted condition is current only when its observed generation and the HTTPRoute's generation are both present, nonzero, and equal. Missing, zero, or mismatched generation information makes that condition unconfirmed, including when it reports rejection. When attachment status is missing or unconfirmed, the tool MUST evaluate the available configuration provisionally for the affected listener, label the attachment unconfirmed, and mark any impact depending on it undetermined while identifying its provisional outcome. Routes on different listeners MUST NOT be treated as covering one another.
- **FR-015**: The tool MUST NOT analyze or report on pods, workloads, endpoint readiness, or application health.
- **FR-016**: The subcommand MUST support the same output formats as the existing audit (text, JSON, YAML, Mermaid), with the same information in each, sending results to standard output and diagnostics to standard error.
- **FR-017**: Text output MUST honor the existing color conventions (disabled by `--no-color`, `NO_COLOR`, or non-terminal output).
- **FR-018**: The subcommand MUST use the project's exit-status conventions: `0` when the simulation (or route listing) completes, regardless of whether traffic would become unrouted; `1` for runtime, source, or parsing errors; `2` for invalid usage, including a target that is not an ambient waypoint; `3` when the waypoint or HTTPRoute is not found.
- **FR-019**: The existing hostname trace, `analyze`, and `test` subcommands MUST continue to behave exactly as before; the literal word `simulate` becomes a reserved root-level subcommand name in the same manner as `analyze`.
- **FR-020**: When required Gateway API routing data cannot be read, the tool MUST report the unavailable input clearly rather than producing an empty or misleading impact result. A missing GatewayClass definition is handled provisionally as specified in FR-004; an absent Istio networking API alone MUST NOT prevent analysis of available Gateway API routing data.
- **FR-021**: Live-cluster mode MUST use only read (get/list) permissions; any additional read permission beyond those already documented MUST be documented alongside the existing permissions guidance.
- **FR-022**: Local results MUST be explicitly scoped to the supplied manifest snapshot and warn that its completeness is unverified. When a detected information gap prevents a conclusion, the affected outcome MUST be undetermined and identify the gap.
- **FR-023**: The tool MUST treat the selected waypoint's HBONE listeners as applicable listeners for direct parent references, alongside HTTP and HTTPS listeners. Where a listener's route-kind, namespace, or hostname restrictions cannot be established, the affected attachment MUST be reported unconfirmed. If a waypoint has no applicable listener, every route MUST be reported ineligible with the reason "no applicable listeners".
- **FR-024**: In local mode, a Gateway manifest whose name matches the selected waypoint and an HTTPRoute manifest whose name matches the selected route or whose Gateway parent reference could target the selected waypoint MUST explicitly declare its resource namespace. If one does not, the tool MUST report the resource and missing namespace as an input error, exit with status `1`, and produce no impact report. The waypoint namespace option MUST NOT supply missing resource namespaces.
- **FR-025**: Direct attachment MUST respect the parent reference's Gateway identity and its listener selection, including section name and port when specified, and the listener's route-kind, namespace, and hostname restrictions. A parent reference with no namespace refers to the HTTPRoute's namespace. The tool MUST NOT infer attachment to an unselected or incompatible listener; missing information needed to establish an attachment MUST be reported as unconfirmed rather than assumed.

### Key Entities

- **Ambient Waypoint**: A Gateway identified as an Istio ambient waypoint, selected by namespace and name. It is the scope boundary for all routing evaluation in a simulation.
- **Attached HTTPRoute**: An HTTPRoute whose attachment to the selected waypoint is established by the existing attachment analysis. Identified by namespace and name; has hostnames and ordered rules.
- **Routing Rule**: One rule of an HTTPRoute, defined by its position, match conditions (path, method, headers, query parameters), and backend Service references.
- **Backend Reference**: A Service (name and namespace) a rule sends traffic to.
- **Simulation**: A single in-memory evaluation of a waypoint's routing with one HTTPRoute excluded. Produces a before view, an after view, and the difference. Never persisted.
- **Impact Entry**: The outcome for a distinguishable portion of a removed rule's traffic on one listener: affected hostnames, request conditions, backends, classification (unrouted, still covered, no change, or undetermined), and replacement route(s) and rule(s) where applicable.
- **Impact Report**: The complete result: the selected waypoint, the removed route, impact entries grouped by classification, combined outcomes per rule, summary counts, and notes such as additional attachments to other waypoints.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For a waypoint with a handful of routes, a user can determine in a single command invocation which traffic of a chosen HTTPRoute would become unrouted and which would remain covered, without reading any raw resource definitions.
- **SC-002**: Across all test scenarios, 100% of simulation runs leave local manifest files byte-identical and issue zero write operations against a cluster.
- **SC-003**: In every scenario with multiple waypoints, 0% of reported replacement coverage comes from routes attached only to a different waypoint.
- **SC-004**: In every verified scenario, traffic portions that lose their route and portions that remain covered by other HTTPRoutes appear in separate groups, including when both outcomes arise from one rule; every rule has a combined outcome summary.
- **SC-005**: Every non-waypoint Gateway selected as a target is rejected with an explanatory message in 100% of tested cases, and no impact report is produced.
- **SC-006**: Text, JSON, YAML, and Mermaid results of the same simulation convey the same set of affected rules and classifications in 100% of verified cases.
- **SC-007**: Existing hostname trace, `analyze`, and `test` behavior and outputs are unchanged, verified by the existing test suites passing without modification.
- **SC-008**: A user who knows a waypoint's namespace and name can find a candidate HTTPRoute and run a first simulation in under 2 minutes.

## Assumptions

- The waypoint is selected the same way the existing `analyze` subcommand selects a Gateway: by name, with its namespace given through the namespace option. The HTTPRoute is selected by its namespace and name through a dedicated option; omitting it lists the attached routes (User Story 2). The exact option spelling is decided during planning.
- "Istio ambient waypoint" is determined with the same class-name rule the existing Gateway audit uses (`istio-waypoint`); no Gateway naming another class is treated as a waypoint. A missing class definition makes validation provisional.
- The feature reuses existing attachment and precedence analysis where applicable, while honoring FR-014 and FR-025 where additional listener-specific analysis is needed; it adds no new precedence rules.
- Typical ambient waypoints expose only an HBONE listener, so HBONE listeners are supported. Istio's exact attachment semantics for HBONE listeners are an assumption to verify during implementation; unverifiable restrictions yield unconfirmed attachments rather than assertions.
- Configuration-based attachment evaluations without current confirming status are provisional and are not evidence of live traffic behavior.
- A local manifest set is not assumed to describe the entire cluster; snapshot-scoped conclusions are not assertions about omitted resources.
- Only HTTPRoute is in scope as the removable resource; GRPCRoutes and other route kinds are neither removable nor reported as replacement coverage in this feature, but their presence does not cause an error.
- An HTTPRoute parented to both the selected waypoint and a Service is included only through its direct waypoint attachment; its Service-parented behavior is outside the impact report.
- Removing a route that is also attached to other waypoints is simulated in memory as a removal of the entire HTTPRoute, but the impact is reported only for the selected waypoint, with a note about its other attachments.
- Backend Services are reported as referenced by the route; whether the Service or its endpoints exist or are healthy is out of scope.
- A completed simulation exits with status `0` even when traffic would become unrouted; unrouted traffic is the report's result, not a tool failure. Introducing a distinct "impact found" exit status is out of scope.
- Where match conditions are too complex to compare (regular-expression paths, header or query matches that do not clearly relate), the tool reports "undetermined" rather than guessing.
- The simulation reports routing impact only; it does not propose fixes, apply changes, or generate replacement manifests.
