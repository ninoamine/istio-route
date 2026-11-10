# Research: HTTPRoute Removal Impact Simulation

## Snapshot selection and read-only access

**Decision**: Reuse `FilesSource.Snapshot` for offline inputs and add a focused live source using existing client interfaces and common discovery/parent-matching helpers. It reads the selected Gateway, its GatewayClass when present, all HTTPRoutes needed for selection and direct-parent coverage, and namespace metadata needed by listener selectors. A missing GatewayClass is provisional; denied or failed reads are errors, not missing data. Retain the identity of an existing but unattached selected route.

**Rationale**: `internal/source/files.go` already supports multiple YAML/JSON documents, one stdin stream, duplicate detection, and no cluster calls. `internal/source/gateway.go` lists HTTPRoutes across namespaces but retains only parent-matched routes and also reads services, EndpointSlices, grants, GRPCRoutes and waypoint selectors. Its filtered results cannot distinguish a nonexistent selected route from an existing unattached route. `internal/kube` already provides injectable readers/client factories and fake-client patterns. HTTPRoute backend references themselves provide Service names and namespaces; no health read is needed. Namespace objects are needed only to evaluate listener namespace selectors; a missing offline namespace object yields unconfirmed where it affects applicability.

**Alternatives considered**: Use `GatewaySnapshot` unchanged: rejected because it collects out-of-scope endpoint data and discards unattached route identity. Use the full cluster snapshot: rejected because it reads unrelated resource families. Fetch only routes in the waypoint namespace: rejected because a route in another namespace may be allowed to attach. Fetch individual selected routes without listing potential replacements: rejected because it misses coverage.

## Waypoint identification and attachment

**Decision**: Extract the audit's `istio-waypoint` class-name predicate and direct Gateway parent identity matcher into reusable domain helpers without changing existing command behavior. Treat HBONE, HTTP and HTTPS listeners of the selected waypoint as applicable, and derive applicability using parent group/kind/name/namespace, optional section name and port, allowed kinds and namespaces, and hostname intersection; restrictions that cannot be established make the attachment unconfirmed. Evaluate current Accepted evidence against the route generation and the applicable parent/listener; distinguish accepted, rejected, and unconfirmed.

**Rationale**: `internal/audit/audit.go` currently recognizes the waypoint by class name, while `internal/audit/source.go` implements parent-reference defaults. The audit's listener and acceptance representations omit port and generation freshness, and its listener filtering is HTTP/HTTPS only. Current `audit.Analyze` requires an existing GatewayClass and includes health analysis, so its public result is insufficient for this feature. Missing class, namespace selector inputs, listener-specific evidence, or stale status must not silently become confirmed acceptance or rejection.

**Alternatives considered**: Treat every Gateway as a waypoint: rejected by scope. Use a Service's waypoint label to infer a Service-parented HTTPRoute attachment: explicitly excluded. Trust Accepted without its observed generation, or merge statuses across listeners: rejected because either can invent coverage. Accept HBONE using the current HTTP/HTTPS-only listener matcher: rejected, because the audit's matcher would ignore the only listener on typical waypoints; the simulation treats HBONE as applicable. Reject HBONE waypoints outright: rejected after analysis because it would make the feature unusable on ordinary ambient waypoints.

## Matching, precedence, and impact regions

**Decision**: Factor existing pure match-ranking and hostname functions into shared APIs; keep the existing audit call path and its outputs stable. Construct before and after routing views using the best *matching* rule per region, and compare them over listener/hostname/request regions. Support provable exact and segment-aware prefix path, hostname, and method relations; use symbolic inclusion/exclusion to express partial coverage and unions. Analyze header/query and regular-expression matches only where a relation is provable; otherwise mark the affected region undetermined. Propagate uncertainty when unknown attachment, class validity, missing selector data, or ambiguous precedence could change the result.

**Rationale**: `internal/audit/precedence.go` compares best match keys and records pairwise winner/partial/shadowed labels, not a complete routing table or traffic-set subtraction. It uses rule-wide best matches, skips some equivalent pairs, treats an empty listener set as overlapping, and uses character-prefix overlap/coverage checks. Those summaries cannot establish exact impact, including `/api` versus `/apix`, same-rule disjunctions, unions of competing routes, or per-listener acceptance. Existing `internal/resolve/hostname.go` provides hostname intersection but must be used only where its result is sufficient for the required set relation.

**Alternatives considered**: Compare only `audit.Result` precedence labels: rejected because it loses the winning traffic portions and may create false coverage. Copy and fork the precedence algorithm: rejected because it would duplicate the routing engine and drift. Rewrite the existing `analyze` command: rejected by the specification. Guess equivalence of regex/header/query matches: rejected because uncertainty must be visible.

## In-memory exclusion and consistency

**Decision**: Parse or fetch the snapshot once, keep its objects immutable, and build separate before/after inputs, excluding the selected group/kind/namespace/name only from the latter. Use deep copies wherever a shared helper could mutate nested objects or slices. Evaluate only the selected waypoint and retain other parent identities for an informational note.

**Rationale**: This preserves all resources and file bytes, avoids a second live read changing the baseline, and avoids slice-alias mutation. Kubernetes reads are not an atomic cluster transaction; the report must not imply a globally consistent or complete cluster snapshot.

**Alternatives considered**: Delete and restore the resource or modify manifests: prohibited. Fetch twice: rejected because unrelated live changes would contaminate the comparison. Mutate the snapshot slice in place: rejected because it can change the baseline through aliasing.

## CLI and presentation

**Decision**: Use `simulate [flags] <waypoint-name>` with `--namespace` for the waypoint and optional `--route <namespace>/<name>` for an HTTPRoute. Without `--route`, list eligible routes. Reuse source, format and color conventions; return one typed logical result in all formats. Emit a route listing before a selection error, and use existing 0/1/2/3 semantics as specified in the contract.

**Rationale**: This follows `analyze` while making cross-namespace route identity explicit and preserving scriptability. Route-level summaries and per-region outcomes avoid hiding mixed results. Listings and impact reports explicitly disclose excluded Service-parented and non-HTTPRoute traffic; offline output warns that the supplied manifest set may be incomplete.

**Alternatives considered**: Interactive selection: rejected because scripts and non-TTY use must work. Assume a route shares the waypoint namespace: rejected because cross-namespace attachments exist. Use `analyze`'s findings exit status 4 for lost routes: rejected by FR-018.

## Test strategy

**Decision**: Add confirmed and unconfirmed offline fixtures, source/CLI/domain table tests, fake-client action checks, and four deterministic render goldens; run existing tests plus race, formatting, vet, lint and coverage gates. Verify before/after immutability and prove no forbidden I/O or local file changes on both success and error paths.

**Rationale**: Existing `internal/cli/audit_test.go`, `internal/source/*_test.go`, and `internal/render/audit_test.go` already demonstrate injectable fakes, cluster reactors, and goldens. New tests must also check semantic agreement, not only renderer snapshots.

**Alternatives considered**: Live-cluster-only validation: rejected because it is not reproducible and cannot reliably verify absence of write requests. Golden output alone: rejected because it does not prove routing outcomes or preservation.
