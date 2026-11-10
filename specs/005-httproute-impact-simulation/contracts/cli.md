# CLI Contract: HTTPRoute Removal Impact Simulation

## Invocation

```text
istio-route simulate [flags] <waypoint-name>
  -n, --namespace <namespace>   waypoint namespace; same resolution as analyze
  -f, --file <path>             repeatable local manifests; - reads stdin once
      --route <namespace>/<name> optional HTTPRoute identity; absent lists candidates
      --kubeconfig <path>       live-mode kubeconfig
      --context <name>          live-mode context
      --output <format>         text (default), json, yaml, mermaid
      --no-color                disable terminal styling
```

Follow `analyze` namespace resolution: an explicit namespace selects the waypoint; live mode uses the kubeconfig namespace if omitted and fails with usage status 2 when no default is available; offline mode permits unambiguous waypoint-name selection without the flag. The route's namespace is always explicit in `--route`. Repeated `--file` values combine one snapshot, and stdin may occur once. With files, do not initialize or contact a cluster. Without files, use read-only live access.

`simulate` is a reserved root command name. The selected Gateway must name class `istio-waypoint`. A Gateway of another class is a usage error. HBONE, HTTP and HTTPS listeners are applicable; a waypoint with no applicable listener reports every route as ineligible with the reason "no applicable listeners". A missing class definition is provisional, not proof of a valid class. Only HTTPRoutes directly parented to the waypoint are selectable or counted as replacements; Service-parented routes and other kinds are disclosed as excluded, with a count.

## Result and error streams

- Without `--route`, write an eligible-route listing to stdout and return 0. Include namespace/name, hostnames, listener attachment states and reasons, source scope, and exclusions. An empty list is explicit.
- With a valid eligible `--route`, write an impact report to stdout and return 0, including for unrouted or undetermined traffic.
- For a missing route or a route ineligible for the selected waypoint, write the eligible-route listing to stdout, explain the error on stderr, and return 3 or 2 respectively. A current rejection on every applicable listener makes the route ineligible; an unconfirmed candidate is eligible for a provisional simulation.
- Invalid flags/identity or non-waypoint target return 2. A missing waypoint returns 3. Source, decoding, missing Gateway API resources in local manifests, and output errors return 1. A local relevant Gateway/HTTPRoute lacking `metadata.namespace` returns 1. No impact report is emitted for an error.
- Output errors override an otherwise successful result. Diagnostics and logs go to stderr, never mixed into structured stdout. `--no-color`, `NO_COLOR`, and non-TTY stdout disable styling.

## Logical output contract

All four formats represent the same logical fields. JSON and YAML serialize equivalent structured values; text and Mermaid must visibly include the same entries and uncertainty. Use deterministic ordering by waypoint/listener, route namespace/name, rule position and canonical region.

The top-level result includes:

| Field | Meaning |
|-------|---------|
| `kind` | `listing` or `impact` |
| `source` | mode and source scope; local reports explicitly warn that input completeness is unverified |
| `waypoint` | namespace, name, class, validation state and reason if unconfirmed |
| `scope` | selected waypoint, direct HTTPRoute-only coverage, exclusions (including the count of excluded Service-parented routes) and other-parent notes |
| `routes` | eligible identities, hostnames and per-listener attachment state for listings |
| `selectedRoute` | namespace/name for impact reports |
| `entries` | per-listener, per-target-rule traffic regions and outcomes |
| `ruleSummaries` | each target rule's outcome set, counts and partial-coverage indication |
| `totals` | counts of entries by outcome |
| `warnings` | missing or incomplete evidence and other limitations |

Each impact entry contains `listener`, target `rule` (route namespace/name and 1-based index), a `region` predicate, `outcome`, `before`, `after`, optional `backendChanged` (true when a still-covered portion is served by backends that differ from the removed rule's), and optional `reasons` and `provisional` data. A winner/provider names route namespace/name, rule index and backend Service namespace/name (and port/weight when available); `before` or `after` may be unknown or empty, but never silently converted into a confirmed provider.

Region predicates must faithfully encode hostname, path, method and any applicable header/query constraints, including exclusions needed to describe partial coverage. A structured predicate is a recursive expression with `all`, `any`, or `not`, or an atom identifying the dimension, match type and value. Omitted constraints mean any; unknown match semantics produce an undetermined region with an explanatory reason. Renderers must not flatten a disjunction or exclusion into a broader definite claim.

Outcomes are:

| Value | Meaning within the selected listener and disclosed scope |
|-------|----------------------------------------------------------|
| `unrouted` | Before traffic was served by the removed route; no in-scope HTTPRoute serves it after removal. |
| `still-covered` | Before traffic was served by the removed route; another in-scope HTTPRoute serves it after removal. |
| `no-change` | The removed rule was not the effective winner for that traffic before removal. |
| `undetermined` | Evidence or match comparison cannot establish the outcome; optional provisional results are explicitly labeled. |

An unconfirmed attachment or waypoint validation cannot be promoted to definitive coverage when the result depends on it. Snapshot-scoped conclusions are not statements about omitted routes or the entire cluster. Each rule summary may contain multiple outcomes.
