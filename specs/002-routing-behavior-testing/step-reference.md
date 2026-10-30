# Routing Behavior Step Reference

This page defines the fixed built-in vocabulary for behavior feature files. Replace
quoted values and one-based indexes with scenario-outline placeholders as needed.
Only these forms are supported; user-defined step implementations are not loaded.
`And` and `But` may continue the appropriate preceding `Given`, `When`, or `Then`.

## Select manifests and trace a hostname

| Step | Meaning |
|---|---|
| `Given the manifests for "prod"` | Assert that the current execution uses the named set `prod`; does not switch sets |
| `When I trace the hostname "shop.example.com"` | Run the existing offline trace and make its structured result available to assertions |
| `Then the hostname should be found` | Assert that the trace contains at least one matching route |
| `Then the hostname should not be found` | Assert that the trace contains no matching routes |
| `Then a route should match` | Assert at least one route is present |
| `Then no route should match` | Assert no routes are present |

The manifest-selection step is optional for an unnamed single-set run. With named
sets it can document the set expected by a scenario, but environment tags select the
execution. An untagged scenario is executed for every set, so an explicit set step
in such a scenario is invalid when multiple sets exist.

## Assert the full structured trace

The generic field steps expose every serialized field of the structured trace
without adding a separate bespoke step for each model property. Field names follow
the JSON field names in the existing trace model. Supported locations are entry
points, routes, rules, destinations, findings, and endpoints nested beneath a
destination's service. A trace-root selector can address any value in the entire
analysis, including source metadata and resource-family availability.

| Step | Meaning |
|---|---|
| `Then trace field "hostname" should equal "shop.example.com"` | Compare any field in the complete structured trace, using a path from the analysis root |
| `Then entry point 1 field "protocol" should equal "HTTPS"` | Compare a scalar or string field on an ordered entry point |
| `Then route 1 field "resource.kind" should equal "VirtualService"` | Compare route identity, origin, hostnames, parent references, or binding conditions |
| `Then rule 1 of route 1 field "matches[0].path" should equal "/api"` | Compare ordered rule match conditions, filters, actions, or destinations |
| `Then destination 1 of rule 1 of route 1 field "subset" should equal "v1"` | Compare backend identity, kind, port, subset, weight, external details, or service data |
| `Then endpoint 1 of destination 1 of rule 1 of route 1 field "ready" should equal "true"` | Compare endpoint address, port, workload identity, and readiness fields |
| `Then finding 1 field "severity" should equal "warning"` | Compare finding code, severity, message, or related resource |

For these steps, `field` uses dot-separated object fields and zero-based indexes
inside serialized lists. The entity ordinal before `field` remains one-based.
For `trace field`, the selector starts at the `model.Analysis` root and uses the
same dot-and-index notation, for example
`source.families[0].available` or `routes[0].rules[0].matches[0].path`.
Boolean, numeric, and string scalar values are written as quoted text and compared
to the field's canonical scalar representation. Missing fields or invalid paths fail
with expected and actual values. Object or list values may be compared using their
canonical JSON representation. A missing field is represented as `<absent>`; use
that quoted value to assert that the trace did not report a field.

## Common routing assertions

The generic structured assertions above are the normative vocabulary for every
trace fact. The following declarative forms cover common chart expectations:

| Step | Meaning |
|---|---|
| `Then it should be exposed through gateway "public-gateway"` | Assert a matching gateway/listener entry point with that name |
| `Then route 1 should be "VirtualService" "shop/shop-vs"` | Assert route kind, namespace/name, and position |
| `Then rule 1 of route 1 should match path prefix "/api"` | Assert a path match condition |
| `Then rule 1 of route 1 should send traffic to service "api" on port 8080` | Assert a service backend and port |
| `Then the traffic split for rule 1 of route 1 should be 90% "v1" and 10% "v2"` | Assert weighted traffic by destination subset |
| `Then there should be no findings with severity "error"` | Assert no finding has that severity |
| `Then there should be a finding with severity "warning" and message containing "not programmed"` | Assert a finding severity and message substring |

The concise forms are equivalent to structured field assertions. They must report
`expected X, got Y` using the trace result when the assertion fails.

## Trace fields that can be asserted

- **Entry points**: origin, resource identity, name, hostname, port, protocol, TLS
  mode and certificate references, route constraints, GatewayClass identity and
  controller, and conditions.
- **Routes**: origin, resource identity, hostnames, parent references, binding
  conditions, and their stable order.
- **Rules**: one-based rule order, path type/value, headers, query parameters,
  method, filters and details, actions, and destinations.
- **Destinations**: kind, group/kind/namespace/name, port, subset, destination rule,
  subset labels, weight, reference-grant result, external host/port/location/
  resolution, and linked Service details.
- **Services and endpoints**: service identity, ports, availability, endpoint
  addresses and ports, readiness/serving/terminating state, and workload identity
  and status where the trace contains them.
- **Findings**: code, severity, message, related resource, and stable finding order.
- **Analysis/source metadata**: hostname, source mode, selected context or namespace
  when present, input files, and resource-family availability and reasons.

An omitted field means the trace did not report that information; scenarios may
assert absence through the supported absence and finding steps or by comparing a
field explicitly to the documented empty representation.
