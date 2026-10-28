#!/usr/bin/env bash
set -euo pipefail

if [[ $# -gt 2 ]]; then
  echo "usage: $0 [--context <name>]" >&2
  exit 2
fi

context="${KUBE_CONTEXT:-}"
if [[ $# -gt 0 ]]; then
  if [[ $# -ne 2 || "$1" != "--context" ]]; then
    echo "usage: $0 [--context <name>]" >&2
    exit 2
  fi
  context="$2"
fi

for tool in kubectl python3; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "required command not found: $tool" >&2
    exit 1
  fi
done
go_bin="$(command -v go || true)"
if [[ -z "$go_bin" && -x /usr/local/go/bin/go ]]; then
  go_bin=/usr/local/go/bin/go
fi
if [[ -z "$go_bin" ]]; then
  echo "required command not found: go" >&2
  exit 1
fi

kubectl_cmd=(kubectl)
if [[ -n "$context" ]]; then
  kubectl_cmd+=(--context "$context")
fi

run_id="$(date -u +%Y%m%d%H%M%S)-$$"
hostname="perf-${run_id}.example.test"
gateway_class="perf-${run_id}"
selector="istio-route.dev/run-id=${run_id}"
results_dir="${RESULTS_DIR:-testdata/performance/results}"
mkdir -p "$results_dir"
report="${results_dir}/performance-${run_id}.txt"
work_dir="$(mktemp -d "${TMPDIR:-/tmp}/istio-route-perf.XXXXXXXX")"
manifest="${work_dir}/workload.yaml"
binary="${work_dir}/istio-route"
nodes_json="${work_dir}/nodes.json"
cluster_json="${work_dir}/cluster.json"

cleanup() {
  local exit_code=$?
  trap - EXIT
  set +e
  "${kubectl_cmd[@]}" delete gatewayclass "$gateway_class" --ignore-not-found --wait=false >/dev/null 2>&1
  "${kubectl_cmd[@]}" delete namespaces -l "$selector" --ignore-not-found --wait=false >/dev/null 2>&1
  rm -f "$manifest" "$binary" "$nodes_json" "$cluster_json"
  rmdir "$work_dir" 2>/dev/null
  exit "$exit_code"
}
trap cleanup EXIT

"${kubectl_cmd[@]}" get nodes -o json >"$nodes_json"
"${kubectl_cmd[@]}" version -o json >"$cluster_json"
node_report="$(python3 - "$nodes_json" <<'PY'
import json
import re
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
workers = []
for node in data.get("items", []):
    labels = node.get("metadata", {}).get("labels", {})
    if "node-role.kubernetes.io/control-plane" in labels or "node-role.kubernetes.io/master" in labels:
        continue
    workers.append(node)
if len(workers) != 3:
    raise SystemExit(f"expected exactly 3 worker nodes, found {len(workers)}")

def memory_bytes(quantity):
    match = re.fullmatch(r"([0-9.]+)([KMGTE]i?|)", quantity)
    if not match:
        raise ValueError(f"unrecognized memory quantity: {quantity}")
    number, suffix = match.groups()
    factors = {
        "": 1, "K": 1000, "M": 1000**2, "G": 1000**3, "T": 1000**4, "E": 1000**5,
        "Ki": 1024, "Mi": 1024**2, "Gi": 1024**3, "Ti": 1024**4, "Ei": 1024**5,
    }
    return float(number) * factors[suffix]

for node in workers:
    name = node["metadata"]["name"]
    info = node.get("status", {}).get("nodeInfo", {})
    capacity = node.get("status", {}).get("capacity", {})
    cpu = int(capacity.get("cpu", "0"))
    memory = memory_bytes(capacity.get("memory", "0"))
    ready = any(
        condition.get("type") == "Ready" and condition.get("status") == "True"
        for condition in node.get("status", {}).get("conditions", [])
    )
    if not ready or cpu != 4 or not 7 * 1024**3 <= memory <= 9 * 1024**3:
        raise SystemExit(
            f"worker {name} must be Ready and provisioned with 4 CPU and 8 GiB "
            f"(capacity: {cpu} CPU, {memory / 1024**3:.2f} GiB)"
        )
    print(
        f"{name}: kubelet={info.get('kubeletVersion', 'unknown')} "
        f"capacity={capacity.get('cpu')} CPU/{capacity.get('memory')} memory"
    )
PY
)"
cluster_version="$(
  python3 - "$cluster_json" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1], encoding="utf-8"))
print(data.get("serverVersion", {}).get("gitVersion", "unknown"))
PY
)"

if ! "${kubectl_cmd[@]}" get --raw /apis/networking.istio.io/v1 >/dev/null; then
  echo "Istio networking.istio.io/v1 API is required for this benchmark" >&2
  exit 1
fi
if ! "${kubectl_cmd[@]}" get --raw /apis/gateway.networking.k8s.io/v1 >/dev/null; then
  echo "Gateway API gateway.networking.k8s.io/v1 API is required for this benchmark" >&2
  exit 1
fi

"$go_bin" build -o "$binary" ./cmd/istio-route
"$go_bin" run ./testdata/performance/generate.go "$run_id" >"$manifest"
"${kubectl_cmd[@]}" apply --request-timeout=5m -f "$manifest"

backend_namespace="istio-route-perf-${run_id}-00"
"${kubectl_cmd[@]}" wait --for=condition=Ready pod \
  -n "$backend_namespace" -l 'app in (backend-a,backend-b)' --timeout=5m
for service in backend-a backend-b; do
  endpoints_ready=0
  for attempt in $(seq 1 30); do
    ready_count="$(
      "${kubectl_cmd[@]}" get endpointslices -n "$backend_namespace" \
        -l "kubernetes.io/service-name=${service}" -o json |
        python3 -c 'import json,sys; data=json.load(sys.stdin); print(sum(1 for s in data.get("items",[]) for e in s.get("endpoints",[]) if e.get("conditions",{}).get("ready") is True))'
    )"
    if [[ "$ready_count" -eq 3 ]]; then
      endpoints_ready=1
      break
    fi
    sleep 2
  done
  if [[ "$endpoints_ready" -ne 1 ]]; then
    echo "expected three ready EndpointSlice endpoints for ${backend_namespace}/${service}, found ${ready_count}" >&2
    exit 1
  fi
done

context_args=()
if [[ -n "$context" ]]; then
  context_args+=(--context "$context")
fi

python3 - "$binary" "$hostname" "$report" "$cluster_version" "$node_report" "${context_args[@]}" <<'PY'
import math
import subprocess
import sys
import time

binary, hostname, report, cluster_version, node_report, *context_args = sys.argv[1:]
command = [binary, *context_args, "--output", "json", hostname]
warmups = 10
measurements = 100
durations = []

for index in range(warmups + measurements):
    started = time.perf_counter()
    result = subprocess.run(command, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, text=True)
    elapsed = time.perf_counter() - started
    if result.returncode:
        sys.stderr.write(result.stderr)
        raise SystemExit(f"analysis failed on run {index + 1} with exit code {result.returncode}")
    if index >= warmups:
        durations.append(elapsed)
        print(f"measurement {index - warmups + 1:03d}: {elapsed:.3f}s")

ordered = sorted(durations)
p95 = ordered[math.ceil(0.95 * len(ordered)) - 1]
lines = [
    f"Kubernetes server: {cluster_version}",
    f"Hostname: {hostname}",
    "Workers:",
    *[f"  {line}" for line in node_report.splitlines()],
    f"Warm-ups: {warmups}",
    f"Measurements: {measurements}",
    "Sequential durations (seconds):",
    *[f"  {index + 1:03d}: {duration:.6f}" for index, duration in enumerate(durations)],
    f"p95: {p95:.6f}s",
    f"Target: p95 < 10s ({'PASS' if p95 < 10 else 'FAIL'})",
]
with open(report, "w", encoding="utf-8") as output:
    output.write("\n".join(lines) + "\n")
print(f"p95: {p95:.3f}s; report: {report}")
if p95 >= 10:
    raise SystemExit("performance target failed: p95 must be below 10 seconds")
PY
