# CLI Contract: Gateway-Scoped Routing Audit

## Invocation

```text
istio-route analyze [flags] <gateway-name>
```

Exactly one Gateway or waypoint name is required. In live mode, `--namespace` is
used when supplied; otherwise the kubeconfig's namespace is used. If neither is
available, the command returns invalid usage and asks for `--namespace`. Offline
mode analyzes only resources supplied through `-f`/`--file`.

## Flags

| Flag | Meaning | Default |
|---|---|---|
| `--namespace`, `-n` | Namespace of the Gateway | Kubeconfig namespace; required if unset in live mode |
| `--file`, `-f` | Offline YAML input; repeatable; `-` means stdin | None (live mode) |
| `--kubeconfig` | Kubeconfig path in live mode | Standard client-go loading rules |
| `--context` | Kubernetes context in live mode | Current context |
| `--output` | `text`, `json`, `yaml`, or `mermaid` | `text` |
| `--no-color` | Disable styled text output | False; `NO_COLOR` and non-TTY also disable |
| `-h`, `--help` | Show subcommand usage | — |

The audit command defines its own flags. It does not change the root hostname
command's existing argument or flag contract.

## Read and Output Behavior

- Offline mode reads no cluster resources and does not initialize a Kubernetes
  client.
- Live mode gets only the selected Gateway and class, lists HTTPRoute and GRPCRoute
  families to filter parentRefs, then reads only linked Services, EndpointSlices,
  relevant ReferenceGrants, and waypoint selectors.
- If the Gateway/GatewayClass API needed to identify the target is unavailable, the
  command reports a source error. An unavailable route or ReferenceGrant family is
  reported clearly and does not prevent analysis of available families.
- No Secrets are read. Only get/list Kubernetes operations are permitted.
- The report is written to stdout; diagnostics/errors go to stderr.
- Findings carry stable code, severity, category, involved resource identities,
  explanation, and suggested fix. Ordering is severity (`error`, `warning`, `info`),
  category (`orphaned`, `duplicated`, `precedence`, `deficient-destination`), then
  stable identity.
- Endpoint health is marked skipped/unavailable in offline input when no
  EndpointSlices are supplied.
- Mermaid draws Gateway -> route -> backend Service and indicates winning,
  partial-overlap, shadowed, and deficient state.

## Exit Codes

| Code | Meaning |
|---|---|
| `0` | Audit completed with no findings |
| `1` | Runtime, source, or parse error |
| `2` | Invalid usage, including missing namespace when kubeconfig has none |
| `3` | Gateway or waypoint not found |
| `4` | Audit completed and one or more findings were reported |

An ambiguous offline Gateway name returns invalid usage and lists candidate
namespaces. A rejected route is an audit finding, not a gateway lookup failure.
Missing or unused ReferenceGrants are warnings; their cross-namespace reference
results are included in the orphaned category.

## Compatibility

The literal root hostname `analyze` is reserved for Cobra subcommand dispatch.
Other hostnames retain current parsing. Existing hostname trace and `test` arguments,
output, step suggestions, JUnit representation, and exit codes do not change.
