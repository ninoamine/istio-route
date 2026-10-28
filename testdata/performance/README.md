# Live-cluster performance check

This benchmark is an **opt-in, mutating test harness**. Run it only against a
dedicated disposable Kubernetes cluster with exactly three Ready worker nodes
provisioned at 4 vCPU and 8 GiB each. The `istio-route` binary remains read-only;
the harness creates and deletes the test workload.

## Requirements

- Go 1.27 or newer, `kubectl`, and Python 3.
- Istio `networking.istio.io/v1` and Gateway API `gateway.networking.k8s.io/v1`
  CRDs installed.
- A Kubernetes context authorized to create namespaces, Services, Pods,
  EndpointSlices, Istio/Gateway API resources, and a GatewayClass, and to delete
  the resources it creates.
- A test cluster with three Ready worker nodes sized at 4 vCPU and 8 GiB RAM each.

Set `KUBE_CONTEXT` or pass `--context` to select the same context for both
`kubectl` provisioning and CLI analysis:

```sh
./testdata/performance/run.sh --context dedicated-perf
```

The generator provisions 100 labeled namespaces with 100 Services each (10,000
total), 100 Istio VirtualServices, and 100 Gateway API HTTPRoutes. Exactly one
route in each API family matches the generated hostname; both matching routes
reference the same two Services, each backed by three ready Pods and the
EndpointSlices produced for those Pods by Kubernetes. The harness waits until all
six Pods and all six ready endpoints are observed before measuring. It performs
10 warm-up analyses and then 100 sequential JSON analyses, recording every duration,
the Kubernetes server and worker kubelet versions, worker capacity, and p95.

The script fails unless p95 is below 10 seconds. It prints and saves a report under
`testdata/performance/results/` by default; override the destination with
`RESULTS_DIR=/path/to/results`. A unique run ID is used for all created namespaces
and the GatewayClass. An exit trap deletes those labeled namespaces and that
GatewayClass. If the script is forcibly terminated or the machine loses power,
manually remove only the namespaces labeled
`app.kubernetes.io/managed-by=istio-route-performance` and the run's GatewayClass.

The node check requires exactly three Ready workers, 4 CPUs, and 7–9 GiB of
reported memory capacity to allow for provider-specific Kubernetes capacity
accounting. Provision the requested 4 vCPU/8 GiB worker size; the actual capacity
values are recorded in the results.
