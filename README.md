# istio-route

`istio-route` is a read-only command-line tool that traces a hostname through
Istio and Kubernetes Gateway API configuration. It shows matching entry points,
attached routes, ordered rules, destinations, and actionable findings.

## Build

Requires Go 1.27 or newer.

```sh
go build -o istio-route ./cmd/istio-route
```

## Inspect offline manifests

Pass one or more files with `-f`/`--file`. Each file can contain multiple YAML
documents. Offline mode reads only those files and does not initialize a Kubernetes
client:

```sh
./istio-route -f testdata/manifests/istio.yaml shop.example.com
./istio-route -f testdata/manifests/istio.yaml \
  -f testdata/manifests/gateway-api.yaml shop.example.com
./istio-route -f testdata/manifests/mixed.yaml --output json shop.example.com
```

The examples under [`testdata/manifests/`](./testdata/manifests/) are intentionally
small and safe to inspect. `missing-reference-grant.yaml` demonstrates a rejected
cross-namespace reference; `endpoints.yaml` includes a Service, EndpointSlice, and
Pod for endpoint inspection.

## Try against a live cluster

The optional Istio demo creates a `demo.example.test` route and a small nginx
workload in the isolated `istio-route-demo` namespace:

```sh
kubectl apply -f testdata/manifests/live-demo.yaml
kubectl -n istio-route-demo rollout status deployment/demo-web
./istio-route --context docker-desktop --namespace istio-route-demo demo.example.test
./istio-route --context docker-desktop --namespace istio-route-demo \
  --output json demo.example.test
```

Replace `docker-desktop` with your kubeconfig context. The demo requires the
Istio networking API; Gateway API output is skipped if its CRDs are not installed.
It is for inspecting routing configuration, not exposing a reachable ingress.
Remove only the demo namespace when finished:

```sh
kubectl delete namespace istio-route-demo
```

## Inspect a live cluster

Without `-f`, the current kubeconfig context is used. Kubeconfig path, context, and
namespace can be selected explicitly:

```sh
./istio-route shop.example.com
./istio-route --kubeconfig ~/.kube/config --context staging \
  --namespace shop shop.example.com
```

The default namespace scope is all namespaces. If Istio or Gateway API CRDs are
unavailable, that resource family is skipped with an informational message on
stderr; available families are still analyzed.

## Output formats and color

Text is the default. Select `json`, `yaml`, or `mermaid` for machine-readable data
or a relationship diagram:

```sh
./istio-route -f testdata/manifests/mixed.yaml --output text shop.example.com
./istio-route -f testdata/manifests/mixed.yaml --output json shop.example.com
./istio-route -f testdata/manifests/mixed.yaml --output yaml shop.example.com
./istio-route -f testdata/manifests/mixed.yaml --output mermaid shop.example.com
```

Colors are used only for terminal text output. `--no-color`, `NO_COLOR`, and
redirecting stdout disable them.

## Permissions and data handling

Cluster mode is read-only. The sample
[`deploy/rbac.yaml`](./deploy/rbac.yaml) grants only `get` and `list` for the
routing resources, Services, EndpointSlices, and endpoint-target Pods used by the
tool. Bind its `istio-route-reader` ServiceAccount to your chosen namespace or
cluster context according to your organization's access policy.

The tool never reads Secret objects or Secret contents. Istio TLS certificate
references are shown by name only; they are not fetched. Offline mode reads the
manifest files supplied on the command line and makes no cluster requests.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Analysis succeeded and a matching route was found |
| `1` | Runtime, source, parsing, or rendering error |
| `2` | Invalid command usage |
| `3` | No supported route matched the requested hostname |

A matching route with unhealthy conditions or unresolved references is reported
with findings; it is not treated as hostname-not-found.

## Development and validation

```sh
go test -race ./...
go vet ./...
golangci-lint run
```

The full feature plan, offline/cluster examples, and live performance procedure are
in [`specs/001-hostname-routing/`](./specs/001-hostname-routing/).
