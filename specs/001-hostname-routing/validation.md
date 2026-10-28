# Phase 7 validation

## Completed checks

- Exercised the offline quickstart analysis using the Istio and Gateway API
  fixtures together. Confirmed text, JSON, YAML, and Mermaid output, parsed the
  JSON and YAML, and verified `NO_COLOR=1` and `--no-color` suppress styling.
- Checked the endpoint fixture's ready Pod details, the missing
  ReferenceGrant warning, and the unmatched-hostname exit code (`3`).
- Verified the performance generator's workload counts: 100 namespaces,
  10,000 Services, 100 Istio VirtualServices, 100 HTTPRoutes, six backend Pods,
  one GatewayClass, and two Gateways.
- Confirmed `deploy/rbac.yaml` contains only `get`/`list` permissions and that
  `internal/kube/` has no Secret access or Kubernetes write methods.
- Passed `go test -race ./...`, `go test ./internal/... -cover`,
  `go test -race -coverpkg=./internal/... -coverprofile=... ./...`
  (81.9% aggregate internal coverage), `go vet ./...`, `gofumpt`, and
  `golangci-lint run`.
- Passed `go mod tidy -diff`, `git diff --check`, and `bash -n
  testdata/performance/run.sh`.
- Validated the GoReleaser configuration and built Linux, macOS, and Windows
  binaries for amd64 and arm64.

## Live checks and remaining gaps

Rechecked on 2026-10-28 after Istio was installed. The active context is
`docker-desktop`; it has two Ready workers, each reporting 4 CPUs and
8135248Ki memory. The Istio `networking.istio.io/v1` API is available, but
there are no Istio routing resources installed. The Gateway API
`gateway.networking.k8s.io/v1` API is unavailable, and no `staging` context is
configured.

Ran both quickstart cluster-analysis commands against `docker-desktop` as
read-only substitutes for the unavailable `staging` context: one scoped to
namespace `shop`, one across all namespaces. Both completed with the documented
no-match exit code `3` and logged that the Gateway API families were skipped.
This confirms live source access and graceful handling of the absent API, but
does not validate a matching live Istio route.

The quickstart's requested live performance run remains blocked: the harness
requires exactly three Ready workers and both Istio and Gateway API. This
context has only two workers and lacks Gateway API. The mutating harness was
not run, and no live performance result is claimed. T052 remains unchecked
until the remaining staging-context and dedicated performance-cluster
scenarios can be exercised.
