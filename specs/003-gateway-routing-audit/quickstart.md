# Quickstart: Gateway-Scoped Routing Audit

This guide describes end-to-end checks for the planned audit command. Build the
binary from the repository root before running the examples.

## Prerequisites

- Go 1.27 or newer.
- Offline YAML fixtures under `testdata/manifests/`, or a kubeconfig with the
  documented read-only get/list permissions.
- For live mode with no `--namespace`, a kubeconfig context with a default namespace.

## Offline Audit

```sh
go build -o istio-route ./cmd/istio-route
./istio-route analyze -f testdata/manifests/gateway-audit-healthy.yaml \
  -n infra edge-gateway
./istio-route analyze -f testdata/manifests/gateway-audit-overlap.yaml \
  -n infra --output json edge-gateway
./istio-route analyze -f testdata/manifests/gateway-audit-waypoint.yaml \
  -n app waypoint
```

Expected outcomes: the healthy case returns 0; cases with findings return 4 while
printing the complete audit; an absent target returns 3. If an offline input has no
EndpointSlices, the report marks endpoint-health evaluation as skipped.

## Output Formats

```sh
./istio-route analyze -f testdata/manifests/gateway-audit-overlap.yaml \
  -n infra --output text edge-gateway
./istio-route analyze -f testdata/manifests/gateway-audit-overlap.yaml \
  -n infra --output json edge-gateway
./istio-route analyze -f testdata/manifests/gateway-audit-overlap.yaml \
  -n infra --output yaml edge-gateway
./istio-route analyze -f testdata/manifests/gateway-audit-overlap.yaml \
  -n infra --output mermaid edge-gateway
```

All formats must carry equivalent gateway, route, backend, precedence, and finding
facts. Mermaid shows Gateway -> routes -> Services and distinguishes winning,
partially overlapping, shadowed, and deficient state. `--no-color`, `NO_COLOR`, and
redirected stdout disable text styling.

## Scenario Assertions

```sh
./istio-route test -f testdata/manifests/gateway-audit-overlap.yaml \
  testdata/features/gateway-audit-overlap.feature
```

The feature asserts finding code/severity/category/resource and winner/shadowed
status. Full syntax and semantics are documented in
[contracts/audit-steps.md](./contracts/audit-steps.md) and added to the existing
[behavior step reference](../002-routing-behavior-testing/step-reference.md).

## Live Audit

```sh
./istio-route analyze --context staging --namespace infra edge-gateway
./istio-route analyze --context staging --namespace infra \
  --output yaml waypoint
```

Live mode gets the selected Gateway/class, lists route families to find matching
parentRefs, then fetches only referenced resources and matching waypoint selectors.
The command never reads Secrets or performs writes. See
[contracts/analyze-command.md](./contracts/analyze-command.md) and `deploy/rbac.yaml`
for the command contract and permissions.

## Required Regression Gates

Run these checks at the end of every code-changing implementation phase; any failure
blocks the next phase.

```sh
go test -race ./...
go vet ./...
golangci-lint run
```

Build and run all existing Gherkin inputs. The first three invocations must return
0, and the failing-assertion fixture must return 1:

```sh
go build -o istio-route ./cmd/istio-route
./istio-route test -f testdata/manifests/behavior.yaml testdata/features/routing-passes.feature
./istio-route test -f testdata/manifests/gateway-api.yaml testdata/features/gateway-api.feature
./istio-route test --manifests prod=testdata/manifests/behavior-prod.yaml --manifests staging=testdata/manifests/behavior-staging.yaml testdata/features/environments.feature
./istio-route test -f testdata/manifests/behavior.yaml testdata/features/routing-fails.feature
```

Run the hostname command with each format and diff stdout against the existing
goldens. Every diff must be empty; no existing expected file may be edited:

```sh
diff -u testdata/golden/mixed-routing.txt <(./istio-route -f testdata/manifests/mixed.yaml --output text shop.example.com)
diff -u testdata/golden/mixed-routing.json <(./istio-route -f testdata/manifests/mixed.yaml --output json shop.example.com)
diff -u testdata/golden/mixed-routing.yaml <(./istio-route -f testdata/manifests/mixed.yaml --output yaml shop.example.com)
diff -u testdata/golden/mixed-routing.mmd <(./istio-route -f testdata/manifests/mixed.yaml --output mermaid shop.example.com)
```

Finally verify `internal/` coverage remains at least 80%, audit goldens cover all
four formats, and 100 behavior scenarios run against two manifest sets in five
seconds or less. The existing hostname and behavior suites must pass unchanged.
