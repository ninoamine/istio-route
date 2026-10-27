# Data Model: Hostname Traffic Routing

## Analysis

One read-only explanation for a requested DNS hostname, produced from either a live
cluster source or supplied manifest files.

| Field | Description | Validation |
|-------|-------------|------------|
| `hostname` | User-requested DNS hostname | Required; normalize case and trailing dot consistently before matching |
| `source` | Cluster or files input and its scope | Exactly one source mode per invocation |
| `entries` | Matching Istio servers and Gateway API listeners | Include only resources whose host rules match or intersect |
| `routes` | Matching VirtualServices and HTTPRoutes | Preserve resource identity, parent bindings, and API-defined rule order |
| `findings` | Informational messages, warnings, and errors | Each finding carries a stable code, severity, message, and related resource when available |

## Resource Identity

Resources are identified by API group, kind, namespace, and name. Cluster-scoped
resources (including GatewayClass) have no namespace. Keep the original API version
and namespace/name for display and diagnostics. Duplicate manifest objects with the
same identity are diagnosed rather than silently treated as distinct resources.

## Hostname Pattern

A normalized hostname is compared against:

- Istio host patterns, including namespace-qualified `namespace/host` entries.
- Istio Gateway server hosts.
- Gateway API listener hostnames.
- HTTPRoute hostnames.

Exact names and supported single-label wildcard patterns are matched according to
the API's hostname semantics. Gateway API route attachment additionally requires
the route hostname to intersect with a listener hostname and the parent/listener
namespace and section constraints to permit attachment. Keep the match reason to
explain exclusions and overlaps.

## Entry Point

An entry point is either an Istio Gateway server or a Gateway API Gateway listener.

| Field | Description |
|-------|-------------|
| `origin` | Istio or Gateway API |
| `resource` | Namespaced Gateway identity |
| `name` | Server/listener name |
| `hostname` | Configured hostname pattern and match result |
| `port` / `protocol` | Configured entry port and protocol |
| `tls` | TLS mode and certificate Secret reference; never Secret contents |
| `route_constraints` | Allowed route kinds/namespaces or equivalent Istio binding data |
| `gateway_class` | GatewayClass name and controller for Gateway API |
| `conditions` | Available Gateway conditions, including Programmed |

## Route and Rule

A route is an Istio VirtualService or Gateway API HTTPRoute. It preserves resource
identity, origin, matched host, attachment/binding information, status conditions,
and ordered rules.

Each rule contains:

- Its original API order/index and match criteria.
- Path/URI, headers, query parameters, and method constraints where applicable.
- Rewrite, redirect, retry, timeout, header modifier, mirror, and other supported
  actions/filters without dropping unrecognized fields silently.
- One or more weighted destinations.

Rule order is preserved within an API resource. The model MUST NOT invent a global
precedence across independent resources or API families.

## Destination

A destination is one of:

1. An Istio host and optional subset with its configured weight and applicable
   DestinationRule/subset definition.
2. A Gateway API backendRef with group, kind, namespace, name, port, weight, and
   ReferenceGrant authorization result.
3. An external ServiceEntry with configured hosts, ports, location, and resolution.

Weights are retained as configured. The diagnostics layer flags Istio weights that
do not sum to 100 and Gateway API weights that are all zero.

## Service Endpoint and Pod

A resolved in-cluster backend may link to a Kubernetes Service and the Service's
actual EndpointSlice endpoints.

| Field | Description |
|-------|-------------|
| `service` | Namespace/name, ports, and available Service status |
| `endpoint` | Address, port, readiness/serving/terminating state, and target reference |
| `pod` | Pod identity and available readiness/status when the endpoint targets a Pod |
| `availability` | Present, not found, or unavailable from the source |

Selector-matching Pods that are not actual endpoints are not represented as receiving
traffic. Missing endpoint or status data is unavailable, not an inferred success or
failure. ServiceEntry destinations do not have Kubernetes Service or Pod records.

## Conditions and Findings

Conditions preserve type, status, reason, message, and observed generation when
provided by the API. Display true/false status with the required check/cross markers;
retain reason/message for user-facing explanation. Missing status is represented as
unavailable and is not inferred.

Findings use a stable code, severity (`info`, `warning`, or `error`), explanatory
message, and zero or more related resource identities. Required warning classes
include unmatched hostname, unbound or rejected route, unresolved parent/listener/
backend, missing subset rule or ReferenceGrant, invalid weights, wildcard overlap,
and cross-family hostname conflict.

## State and Data Flow

```text
AnalysisRequest
  -> SourceSnapshot (cluster or files)
  -> HostnameMatches
  -> RoutingTree (entries, routes, rules, destinations, endpoint details)
  -> Findings
  -> RenderedReport (text, JSON, YAML, or Mermaid)
```

Cluster snapshots distinguish unavailable API families from empty families. Offline
snapshots never claim live-only status unless that status is present in a manifest.
