# CLI Contract: Gateway API Rule Precedence

**Date**: 2026-11-07 | **Feature**: [spec.md](./spec.md)

The CLI surface is **unchanged** (FR-012). This contract documents the stable surface and
the single documented change: precedence finding message wording.

## Commands (unchanged)

- `istio-route audit gateway <name> [--namespace <ns>]` with the existing source flags
  (live cluster or offline manifest files). No new, renamed, or removed flags.

## Output formats (unchanged)

- `--output text|json|yaml|mermaid` — all four render the same `audit.Result`; rules,
  precedence states (`winner`, `partial-overlap`, `shadowed`), and findings are identical
  across formats (FR-011).
- Results on stdout; logs/errors on stderr; `NO_COLOR` honored; color disabled when stdout
  is not a TTY.

## Exit codes (unchanged)

| Code | Meaning |
|------|---------|
| 0 | success |
| 1 | error |
| 2 | usage error |
| 3 | hostname not found |

## Finding contract (documented change)

- Existing codes kept: `shadowed-route-rule` (warning), `partial-overlap-route-rule`
  (info), `winning-route-rule` (info).
- **New code**: `undecided-route-rule-overlap` (info) — emitted on both rules when two
  overlapping rules are equal on all normative criteria and all tie-breakers (FR-005).
- **Wording change (FR-008)**: precedence finding messages now name the competing rule
  (`<kind> <namespace>/<name> rule <index>`) and the criterion that decided the outcome
  (one of: `path type`, `prefix length`, `method`, `headers`, `query params`,
  `creation timestamp`, `namespace/name`, `rule index`). Category (`precedence`),
  severities, and suggested fixes are unchanged.
- JSON/YAML schema unchanged: same fields, same `omitempty` behavior; only message strings
  and the additional finding code differ.

## Documented limitations (FR-009)

- RegularExpression path matches rank below Exact/PathPrefix and above no-path;
  implementation-specific per the Gateway API specification.
- GRPCRoute rules are compared on path/method/header/query-param criteria only; GRPC
  service/method fields do not influence precedence.
- Route kinds other than HTTPRoute/GRPCRoute are excluded from precedence comparison.
