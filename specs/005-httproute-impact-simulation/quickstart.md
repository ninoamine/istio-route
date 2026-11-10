# Quickstart: Validate HTTPRoute Removal Impact Simulation

This guide is for the implementation phase. The named `simulate-*` fixtures and command will be added by that phase; they do not yet exist. Use the [CLI contract](./contracts/cli.md) and [data model](./data-model.md) for exact semantics.

## Prerequisites

- Go 1.27 and the repository's existing dependencies.
- A future `testdata/manifests/simulate-impact.yaml` fixture containing an `istio-waypoint` Gateway `mesh/waypoint` with HTTP/HTTPS listeners, its GatewayClass, and two explicitly namespaced HTTPRoutes with matching current generation/Accepted status: `shop/primary` wins `/api` and `shop/fallback` covers `/api` via `/`, while a separate portion of `primary` is uncovered.
- An optional read-only live test cluster with the required Gateway API access and an `istio-waypoint` Gateway. **Do not create or alter cluster resources as part of this guide.**

## Local listing and impact

After implementation, from the repository root:

```sh
go run ./cmd/istio-route simulate -f testdata/manifests/simulate-impact.yaml \
  -n mesh waypoint
go run ./cmd/istio-route simulate -f testdata/manifests/simulate-impact.yaml \
  -n mesh --route shop/primary waypoint
go run ./cmd/istio-route simulate -f testdata/manifests/simulate-impact.yaml \
  -n mesh --route shop/primary --output json waypoint
go run ./cmd/istio-route simulate -f testdata/manifests/simulate-impact.yaml \
  -n mesh --route shop/primary --output yaml waypoint
go run ./cmd/istio-route simulate -f testdata/manifests/simulate-impact.yaml \
  -n mesh --route shop/primary --output mermaid waypoint
```

Expect the listing to identify both routes, and the report to separate the covered and unrouted portions, identify listener/rule/backend and fallback, and warn that offline input completeness is unverified. All formats must convey the same results. Omitting class or current status from a separate fixture produces explicitly undetermined outcomes with provisional details, not a definitive result.

Verify stdin and unchanged files:

```sh
sha256sum testdata/manifests/simulate-impact.yaml
go run ./cmd/istio-route simulate -f - -n mesh --route shop/primary waypoint \
  < testdata/manifests/simulate-impact.yaml
sha256sum testdata/manifests/simulate-impact.yaml
```

The two hashes must match. The existing `testdata/manifests/gateway-audit-waypoint.yaml` contains a waypoint whose only listener is HBONE (`app/waypoint`); it has no HTTPRoutes, so the listing is empty and the command succeeds:

```sh
go run ./cmd/istio-route simulate -f testdata/manifests/gateway-audit-waypoint.yaml \
  -n app waypoint
```

Expected: an explicit "no eligible HTTPRoutes" listing and exit status 0 (the waypoint is not rejected). A manifest lacking any Gateway API resources, for example `testdata/manifests/istio.yaml`, must fail with a missing-input error and status 1. `go run` wraps a program's nonzero exit; use a built binary or command tests to check exact statuses.

## Live and automated verification

Against an existing suitable waypoint, use its actual names:

```sh
go run ./cmd/istio-route simulate --context staging -n mesh waypoint
go run ./cmd/istio-route simulate --context staging -n mesh \
  --route shop/primary --output json waypoint
```

Expected: only `get`/`list` requests, no pod/workload/EndpointSlice/health access, and results scoped to the selected waypoint and available snapshot. Missing permissions are explicit errors. Routes on other waypoints alone must not count as coverage.

Run the planned focused tests and project gates after implementation:

```sh
go test -race ./internal/simulate/... ./internal/source/... ./internal/cli/... ./internal/render/... ./internal/audit/... ./cmd/istio-route/...
go test -race ./...
go vet ./...
golangci-lint run
gofumpt -l cmd internal
```

The focused and full suites must pass; `gofumpt -l` must print no paths. The tests must verify fake-client request verbs and resource kinds, immutable snapshots and files, all four renderer goldens, uncertainty and namespace/listener edge cases, and existing-command regressions.
