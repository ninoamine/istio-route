# Data Model: HTTPRoute Removal Impact Simulation

## Input and identities

- **Simulation request**: waypoint name and resolved namespace; optional HTTPRoute namespace/name; source mode and file/stdin or kubeconfig/context settings; output format and color preference. The route identity is required to simulate and absent to list candidates.
- **Snapshot**: source metadata plus immutable Gateway, optional GatewayClass, HTTPRoutes, and namespace data needed for selected-listener restrictions. It is read once. Local snapshots may be incomplete; live snapshots are not atomic.
- **Waypoint**: Gateway group/kind/namespace/name, class name `istio-waypoint`, listener definitions, and validation state (`confirmed` or `unconfirmed` if its class definition is absent). Reject other classes. HTTP, HTTPS and HBONE listeners are applicable listeners; a waypoint with none makes every route ineligible ("no applicable listeners").
- **Route identity**: group `gateway.networking.k8s.io`, kind `HTTPRoute`, namespace and name. Removing a route matches the full identity, not just its name. Direct references to other parents are retained only for an out-of-scope note.
- **Attachment evidence**: route identity, selected listener identity, relevant parent reference including section/port, and `accepted`, `rejected`, or `unconfirmed` state with reason. A current Accepted condition has matching, nonzero route generation and observed generation; stale/absent or insufficiently scoped evidence is unconfirmed. Rejection for one listener does not affect another.

## Routing evaluation

- **Rule**: route identity, 1-based position, hostnames, disjunctive HTTP match expressions, backend references, and precedence inputs. Service references carry namespace, name, optional port and weight where derivable; no service health fields.
- **Traffic region**: a selected listener plus a symbolic predicate over effective hostname, path, method, headers and query parameters. Predicates can represent inclusion, conjunction, alternatives and exclusions, so e.g. prefix `/api` excluding prefix `/api/v2` is expressible. Regions with different listeners remain separate. A match with no path defaults to prefix `/`; unspecified method means any.
- **Routing view**: for each derived region, the winning route/rule and referenced backends where provable, or uncertainty and reasons. Derive a before view from the original immutable input and an after view from an independently owned input without the selected route. Do not infer a winner from a rule's unrelated best match.
- **Impact entry**: target rule identity and position, region predicate and listener, before/after winner and backend references where known, one outcome (`unrouted`, `still-covered`, `no-change`, `undetermined`; implemented as a `Classification` type), a `backendChanged` indicator for still-covered portions, reasons, and an optional clearly labeled provisional outcome and providers. `unrouted` means no in-scope direct waypoint-parented HTTPRoute remains for that portion; it does not claim that Service-parented or other routing cannot handle traffic.
- **Rule summary**: target rule and referenced backends, its set of outcome labels, per-outcome region counts, and partial-coverage indication where both covered and unrouted portions are known. A rule can have more than one outcome; do not force an exclusive rule-wide classification.
- **Result**: source scope/warnings and exclusions (with the count of excluded Service-parented routes), waypoint, optional selected route, eligible route listing or impact entries, per-rule summaries, and outcome totals. Results are immutable after construction and deterministically ordered.

## Invariants and state transitions

1. Target and candidate identities are namespace-qualified. Local relevant Gateway/HTTPRoute resources without metadata.namespace are input errors; the waypoint namespace flag does not fill them.
2. A route is eligible if at least one selected-waypoint listener is confirmed accepted or plausibly attached but unconfirmed. A route known rejected/absent on all applicable listeners is ineligible; a route on another waypoint alone is not a replacement.
3. `unconfirmed` never becomes confirmed merely because status is absent or stale. A current negative status can reject only the listener to which it applies. Unknown evidence that can change a region's outcome yields `undetermined`; any hypothetical result is explicitly provisional.
4. The before input includes the target; the after input excludes that exact route only. Source snapshot, sibling resources, local files, and cluster state are not mutated.
5. For each target rule and listener, report distinguishable regions sufficient to describe all in-scope outcomes. Do not assert disjointness, winners, coverage, or unsupported regex/header/query relations without proof; unknown or unresolved subsets carry a reason. Keep derived regions and summaries stable under input permutation.
6. A completed simulation has exit status 0 even if portions are unrouted or undetermined. Invalid selection, unavailable required input, and unsupported waypoints follow the contract's error paths.
