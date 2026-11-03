# Data Model: Gateway-Scoped Routing Audit

## Audit Request

One immutable request identifies the Gateway or ambient waypoint and exactly one
input mode.

| Field | Description | Validation |
|---|---|---|
| `gatewayName` | Requested Gateway metadata name | Required Kubernetes DNS name |
| `namespace` | Namespace containing the Gateway | Explicit value wins; otherwise use kubeconfig namespace; if neither exists, invalid usage |
| `source` | Live cluster or offline files/stdin | Exactly one source mode |
| `output` | Text, JSON, YAML, or Mermaid | Defaults to text |
| `noColor` | Disable terminal styling | `NO_COLOR` and non-terminal stdout also disable styling |

## Gateway Audit Result

`internal/audit.Result` is distinct from `model.Analysis`. It contains no hostname
and describes one selected Gateway identity.

| Field | Description |
|---|---|
| `source` | Existing source mode/context/namespace/file metadata and API-family availability |
| `gateway` | Gateway identity, GatewayClass identity/name, class controller, and programmed condition |
| `listeners` | Ordered listener names, hostnames, ports, protocols, allowed route constraints, and listener status |
| `routes` | HTTPRoute and GRPCRoute resources whose parentRefs target the selected Gateway, including rejected or status-unknown routes |
| `waypointSelections` | Namespace and Service identities and their waypoint label values when auditing an `istio-waypoint` |
| `findings` | Stable, ordered `model.Finding` values, including audit category and suggested fix |
| `endpointCheck` | Whether destination readiness was checked, skipped for missing offline slices, or unavailable due to source failure |

An absent Gateway returns a gateway-not-found outcome, not a successful empty audit.
Source, parsing, and API access failures remain errors rather than success-shaped
results.

## Route and Rule

A route retains its original Kubernetes kind and namespace/name, declared hostnames,
parent references, and the per-parent/per-listener acceptance state:

- **accepted** when the matching parent status has `Accepted=True`;
- **rejected** when the matching parent status has `Accepted=False`, with reason and
  message when provided;
- **unknown** when the route has no corresponding acceptance condition.

Each route rule retains its one-based API order, listener scope, match alternatives,
backend references, and precedence relationship. HTTP matches preserve path kind and
value, headers, method, and query parameters. GRPC matches preserve service, method,
and headers. A rule has one of the following precedence states per overlapping
traffic region:

- **winner**: highest precedence for that region;
- **partial-overlap**: loses only in the intersection and remains eligible elsewhere;
- **shadowed**: its complete match space is covered by higher-precedence rules.

Exact duplicate identity is determined by listener scope, normalized hostname, and
equivalent match fields. Creation timestamp and namespace/name resolve precedence
ties; original rule ordering remains visible.

## Backend Destination

Each backend reference records its originating route/rule, referenced group/kind,
namespace/name/port, ReferenceGrant decision when cross-namespace, and any resolved
Service facts.

| Field | Description |
|---|---|
| `service` | Referenced Service identity and declared ports, or not-found/unavailable state |
| `portStatus` | Whether the numeric backend port exists on the Service |
| `endpointSlicesSupplied` | Distinguishes no offline endpoint data from an observed empty/zero-ready result |
| `readyAddressCount` | Count of addresses whose EndpointSlice endpoint condition is ready |
| `winningState` | Whether the rule wins, partially overlaps, or is fully shadowed for the analyzed traffic |

A missing Service or port is orphaned. A present Service with zero ready addresses
across its supplied/queried EndpointSlices is deficient. Missing offline EndpointSlices
skip readiness evaluation rather than implying zero readiness or success. No Pod
object is required to determine endpoint readiness.

## Waypoint Selection

A waypoint is a Gateway whose GatewayClass name is `istio-waypoint`. Namespace and
Service selector facts retain:

- selecting resource identity and kind;
- raw `istio.io/use-waypoint` label value;
- resolved waypoint namespace/name, whether implicit or namespace-qualified;
- target existence and Programmed condition.

A label pointing to a missing waypoint or one not programmed is an orphaned finding.
Selectors for unrelated waypoint identities are outside the selected audit result.

## ReferenceGrant

A grant is relevant when it is in a namespace targeted by a cross-namespace backend
reference from a selected route and its `from` rules can refer to that route's
namespace/kind. The result records the grant identity, applicable `from`/`to`
constraints, and whether an in-use backend reference is authorized. A relevant grant
that authorizes no selected route reference produces an unused-grant finding.

## Finding

Findings reuse `model.Finding` and add optional audit-only values:

| Field | Description |
|---|---|
| `code` | Stable kebab-case machine identifier |
| `severity` | `error`, `warning`, or `info` |
| `category` | `orphaned`, `duplicated`, `precedence`, or `deficient-destination` |
| `message` | Human explanation and reason |
| `resources` | Involved resource identities (kind, namespace, name) |
| `suggestedFix` | Actionable remediation |

Audit-only fields are omitted for existing hostname findings so existing JSON/YAML
goldens remain byte-for-byte stable. Audit findings sort by severity (error, warning,
info), then canonical category order, then stable resource/code keys.

## Finding Categories and Severity

| Category | Examples | Severity |
|---|---|---|
| `orphaned` | Rejected route; missing Service/port; missing ReferenceGrant; unused ReferenceGrant; missing or unprogrammed waypoint | Errors for rejected route, missing Service/port, or missing/unprogrammed waypoint; warnings for missing/unused ReferenceGrant |
| `duplicated` | Identical hostname, listener, and match rule | Warning |
| `precedence` | Winner; partial overlap; fully shadowed rule | Informational winner/partial-overlap; warning for shadowed |
| `deficient-destination` | No ready endpoint addresses | Error when winning; warning when fully shadowed |

## Source Acquisition and State Flow

```text
AnalyzeRequest
  -> targeted ClusterSource.GatewaySnapshot OR supplied FilesSource.Snapshot
  -> exact Gateway/GatewayClass and parentRef-filtered HTTPRoute/GRPCRoute set
  -> referenced Services, EndpointSlices, relevant ReferenceGrants, waypoint selectors
  -> orphaned / duplicated / precedence / deficient-destination checkers
  -> AuditResult + model.Finding values
  -> audit text, JSON, YAML, or Mermaid renderer
```

The live path must never list unrelated resource families or read Secrets. Route
families are the deliberate exception to per-object lookup: they are listed because
parent references have no server-side selector, then filtered before dependent
resources are fetched.
