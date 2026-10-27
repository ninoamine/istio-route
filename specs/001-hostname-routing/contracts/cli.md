# CLI Contract: Hostname Traffic Routing

## Invocation

```text
istio-route [global flags] <hostname>
```

Exactly one hostname is required. The default source is the current Kubernetes
context. One or more `-f, --file` arguments select offline mode; when files are
provided, no cluster connection is initialized.

## Flags

| Flag | Meaning | Default |
|------|---------|---------|
| `--kubeconfig` | Path to kubeconfig in cluster mode | Standard client-go loading rules |
| `--context` | Kubernetes context in cluster mode | Current context |
| `-n, --namespace` | Namespace scope in cluster mode | All namespaces |
| `-f, --file` | YAML file; repeatable for multiple files and multi-document input | None |
| `--output` | `text`, `json`, `yaml`, or `mermaid` | `text` |
| `--no-color` | Disable terminal colors | False; `NO_COLOR` and non-TTY also disable |
| `-h, --help` | Print command help | — |
| `--version` | Print CLI version | — |

## Output Streams

- Successful report data is written to stdout only.
- Logs, skipped-family information, warnings not embedded in the report, and errors
  are written to stderr.
- Text output is a tree with explicit `[Istio]` or `[Gateway API]` labels on every
  resource, rule, filter, and destination node; rules are ordered and numbered,
  weights are visible per backend, conditions use `✔`/`✘`, and warnings appear last
  with `⚠`.
- JSON and YAML encode the same routing tree and findings. Mermaid represents the
  relationships without implying global evaluation order across independent routes.
- The CLI never emits Secret data.

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Analysis succeeded and at least one matching route was found |
| `1` | Runtime, source, parsing, or rendering error |
| `2` | Invalid command usage, including missing or extra hostname arguments |
| `3` | No supported route matched the hostname |

A matched route with rejected conditions or unresolved references returns a diagnostic
report and is not treated as hostname-not-found.

## Read-Only Behavior

Cluster mode uses only Kubernetes get/list requests and requires no write permissions.
Offline mode reads files only and does not initialize a cluster connection.
