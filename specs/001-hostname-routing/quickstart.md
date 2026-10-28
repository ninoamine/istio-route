# Quickstart: Hostname Traffic Routing

This guide defines validation runs for the planned CLI. Commands assume the binary is
available as `./istio-route` after build; exact build commands are finalized with the
implementation tasks.

## Prerequisites

- Go 1.27 toolchain.
- A checkout of this repository and dependencies downloaded by Go modules.
- For cluster checks, a kubeconfig/context with read-only `get`/`list` access to
  routing resources and referenced Services, EndpointSlices, and Pods.
- For offline checks, fixtures under `testdata/manifests/`.

## Offline Analysis

```sh
./istio-route -f testdata/manifests/istio.yaml \
  -f testdata/manifests/gateway-api.yaml shop.example.com
```

Expected outcome: no cluster access is attempted. The report includes matching
Istio and Gateway API paths, explicit origin labels, ordered rules, weighted
destinations, and findings represented by the supplied manifests. Missing Pod,
endpoint, or status data is labeled unavailable.

## Output Format Checks

```sh
./istio-route -f testdata/manifests/mixed.yaml --output text shop.example.com
./istio-route -f testdata/manifests/mixed.yaml --output json shop.example.com
./istio-route -f testdata/manifests/mixed.yaml --output yaml shop.example.com
./istio-route -f testdata/manifests/mixed.yaml --output mermaid shop.example.com
NO_COLOR=1 ./istio-route -f testdata/manifests/mixed.yaml --output text shop.example.com
./istio-route -f testdata/manifests/mixed.yaml --no-color shop.example.com
```

Expected outcome: all formats describe the same routes, destinations, conditions,
and findings. Text output matches the tree contract in [contracts/cli.md](./contracts/cli.md).
With `NO_COLOR=1` or redirected stdout, text contains no color control sequences.

## Cluster Analysis

```sh
./istio-route --context staging --namespace shop shop.example.com
./istio-route --context staging shop.example.com
```

Expected outcome: the first command scopes namespaced data to `shop`; the second
inspects all namespaces. Both use read-only operations. When a supported CRD family
is unavailable, the command continues with an informational message for that family.

## Not-Found and Diagnostic Checks

```sh
./istio-route -f testdata/manifests/mixed.yaml absent.example.com
./istio-route -f testdata/manifests/missing-reference-grant.yaml shop.example.com
```

Expected outcome: an unmatched hostname gives an actionable message and exit code 3.
A matching but rejected or unresolved route produces a report with conditions and
warnings rather than being mislabeled as not found. See [data-model.md](./data-model.md)
for identity, endpoint, and finding semantics.

## CI Validation

The implementation's CI commands MUST cover:

- `gofumpt` formatting check, `go vet ./...`, and `golangci-lint run`.
- `go test -race ./...` and enforcement of at least 80% coverage for `internal/`.
- Build on the supported platforms/architectures.
- GoReleaser validation in CI and release publication on version tags.
- Golden comparison for text, JSON, YAML, and Mermaid reports.

## Live-Cluster Performance Validation

Provision a dedicated three-worker Kubernetes test cluster, with each worker set to
4 vCPU and 8 GiB memory. Use the harness described in
[`testdata/performance/README.md`](../../testdata/performance/README.md) to load
10,000 Services across 100 namespaces, 100 Istio VirtualServices, and
100 Gateway API HTTPRoutes. Exactly one VirtualService and one HTTPRoute match the
queried hostname; both reference the same two Services, each with three ready Pod
endpoints. The other routes use non-matching hostnames.

```sh
testdata/performance/run.sh
```

Expected outcome: the harness provisions only test resources, runs 10 warm-ups and
100 sequential CLI analyses, records Kubernetes/worker specifications and each
duration, computes p95, and exits unsuccessfully unless at least 95 of the 100
measurements finish under 10 seconds. Do not substitute fake-client timings for
this live-cluster validation. The `istio-route` binary itself remains read-only.
