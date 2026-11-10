---

description: "Task list for HTTPRoute removal impact simulation"
---

# Tasks: HTTPRoute Removal Impact Simulation

**Input**: Design documents from `specs/005-httproute-impact-simulation/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/cli.md](./contracts/cli.md), [quickstart.md](./quickstart.md)

**Tests**: Requested. Every task states its expected **Outcome** and the **Tests** that prove it. Test tasks are table-driven; rendering uses golden files under `testdata/golden/`; run tests with `-race`.

**Organization**: Tasks are grouped by user story. The ten requested work groups are cross-referenced in the table below and tagged `(Group N)` in task descriptions.

## Format: `- [ ] T### [P?] [US#?] Description with file path`

- **[P]**: Parallelizable (different files, no dependency on an incomplete task)
- **[US#]**: User story from spec.md (story phases only)
- Paths are relative to the repository root `/home/abakkali/istio-route`

## Hard constraints (apply to every task)

- Read-only: no Kubernetes `Create`/`Update`/`Patch`/`Delete`/`Watch` calls, no file writing of manifests or generated manifests, no pod, workload, Service, EndpointSlice, or health reads.
- Reuse existing code; do not rewrite `analyze`, hostname trace, or `test`. Changes to `internal/audit` are limited to adding new exported thin wrappers that delegate to existing unexported functions (no behavior change).
- Constitution: `context.Context` first param for I/O functions, errors wrapped with `%w`, no panics outside `main`, no `utils` package, logs via `log/slog` on stderr, results on stdout.
- Run `gofumpt`, `go vet ./...`, `golangci-lint run` on touched code before considering a task done.

## Work-group map

| Group | Topic | Tasks |
|-------|-------|-------|
| 1 | CLI subcommand and argument parsing | T007–T011 |
| 2 | Ambient waypoint validation and selection | T014–T015, T045–T047 |
| 3 | HTTPRoute discovery and selection | T016–T019, T035–T042 |
| 4 | In-memory removal simulation | T020–T022 |
| 5 | Before/after comparison and impact reporting | T023–T029, T043–T046 |
| 6 | Local manifest integration | T012–T013, T033–T034 |
| 7 | Live Kubernetes integration | T052–T056 |
| 8 | Output formatting | T030–T031, T040–T041, T048–T051 |
| 9 | Unit, integration, regression tests | included in every task; consolidated in T059–T062 |
| 10 | Documentation | T057–T058, T063 |

---

## Phase 1: Setup

**Purpose**: Establish a green baseline and the new package.

- [X] T001 Run the repository baseline and record results in the PR description (no file change). **Outcome**: confirmed starting state is green before edits. **Tests**: `go test -race ./...`, `go vet ./...`, `golangci-lint run`, `gofumpt -l cmd internal` (must print nothing).
- [X] T002 [P] Create `internal/simulate/doc.go` with a package comment stating the package is a pure, in-memory, read-only simulation domain that performs no I/O. **Outcome**: package exists and builds. **Tests**: `go build ./...`.

---

## Phase 2: Foundational (blocking prerequisites for all user stories)

**Purpose**: Domain types, reuse seams, CLI skeleton, local input safety, waypoint validation, attachment evidence, and route discovery used by every story.

### Domain types

- [X] T003 Create `internal/simulate/types.go` with the data-model entities: `RouteRef{Namespace, Name}` (identity is group `gateway.networking.k8s.io`, kind `HTTPRoute`, namespace and name; removal matches the full identity, not just the name), `Classification` string enum with exactly `unrouted`, `still-covered`, `no-change`, `undetermined` (serialized in the `outcome` JSON/YAML field), `AttachmentState` enum `accepted`, `rejected`, `unconfirmed`, `ValidationState` enum `confirmed`, `unconfirmed`, `ResultKind` enum `listing`, `impact`, `Waypoint`, `ListenerAttachment` (route identity, listener name, parent section/port, state, reason; "A current Accepted condition has matching, nonzero route generation and observed generation; stale/absent or insufficiently scoped evidence is unconfirmed"), `Predicate` (recursive `all`/`any`/`not` or an atom with dimension `hostname|path|method|header|query`, match type, value), `Provider{Route RouteRef, RuleIndex int, Backends []BackendRef}` and `BackendRef{Namespace, Name string; Port *int32; Weight *int32}` (no health fields), `Entry` (listener, target rule, region, `Classification` as `outcome`, before, after, `BackendChanged bool` tagged `json:"backendChanged,omitempty"`, reasons, optional provisional), `RuleSummary` (outcome set, per-outcome counts, partial-coverage flag), `Totals`, `Result` (`kind`, `source`, `waypoint`, `scope`, `routes`, `selectedRoute`, `entries`, `ruleSummaries`, `totals`, `warnings`) with `json`/`yaml` tags matching the field names in [contracts/cli.md](./contracts/cli.md), and `Validate()` methods enforcing: rule index ≥ 1, outcome and state enums valid, "A rule can have more than one outcome; do not force an exclusive rule-wide classification", every entry has exactly one outcome. **Outcome**: typed model compiles and validates. **Tests**: see T004.
- [X] T004 Create `internal/simulate/types_test.go`: table-driven `Validate()` tests (valid/invalid outcomes, invalid index, missing listener) and JSON/YAML round-trip test proving field names match the contract. **Outcome**: model invariants locked. **Tests**: `go test -race ./internal/simulate/`.

### Reuse seams in internal/audit (Group 2/3/5 reuse)

- [X] T005 Create `internal/audit/shared.go` exporting thin wrappers only: `IsWaypointGateway(*gatewayv1.Gateway) bool` (delegates to `isWaypoint`, audit.go:523), `ParentTargets(ref gatewayv1.ParentReference, routeNamespace string, gateway model.ResourceIdentity) bool` (delegates to `parentTargets`), `HTTPMatchFrom(gatewayv1.HTTPRouteMatch) Match` (delegates to `httpMatch`), `SelectGateway(snapshot source.Snapshot, name, namespace string) (*source.Resource, error)` (delegates to `selectGateway`, audit/source.go:30-52), and `CompareRules(leftRoute Route, leftRule Rule, rightRoute Route, rightRule Rule) (winner int, criterion string)` returning `1`, `-1`, or `0` (undecided) by building two `ruleRef` values and calling `compareRulePrecedence` (precedence.go:284). No existing function is modified. **Outcome**: simulation can reuse waypoint identification, parent matching, match conversion, and the 004 precedence comparator. **Tests**: see T006.
- [X] T006 Create `internal/audit/shared_test.go`: parity tests asserting each wrapper returns the same result as the unexported function for: class `istio-waypoint` vs another class; parent refs with default group/kind/namespace; Exact vs PathPrefix, longer prefix, method, header count, creation-time and identity tie-breakers, and undecided (all equal). **Outcome**: wrappers provably behave like the audit logic. **Tests**: `go test -race ./internal/audit/` and the existing audit suite still passes unmodified.

### CLI skeleton (Group 1)

- [X] T007 Create `internal/render/simulate.go` with `RenderSimulation(result simulate.Result, format string, options Options) (string, error)` dispatching on `FormatText`, `FormatJSON`, `FormatYAML`, `FormatMermaid` to per-format functions that return `fmt.Errorf("simulate %s rendering is not implemented", format)` until their tasks land; unknown format returns `fmt.Errorf("unsupported output format %q", format)` (same as `RenderAudit`, internal/render/audit.go:12-29). **Outcome**: CLI can compile against a stable render entry point. **Tests**: see T008 (dispatch test added in `internal/render/simulate_test.go`: unknown format errors).
- [X] T008 Create `internal/cli/simulate.go` modeled on `newAuditCommand` (internal/cli/audit.go:84-142): define `ErrRouteNotFound = errors.New("HTTPRoute not found")` and `ErrRouteIneligible` (wrapping `ErrUsage`); `SimulateRequest{WaypointName string; Route *simulate.RouteRef; Flags Flags}`; `SimulateService` interface `Simulate(context.Context, SimulateRequest) (simulate.Result, simulate.Status, error)`; `newSimulateCommand(service, stdout, stderr)` with `Use: "simulate [flags] <waypoint-name>"`, exactly one positional argument (else `ErrUsage`), flags `--kubeconfig`, `--context`, `-n/--namespace`, `-f/--file` (string array), `--route <namespace>/<name>`, `--output` (default `text`), `--no-color`; `--route` is split on a single `/` and both parts validated with `validation.IsDNS1123Label`/`IsDNS1123Subdomain` from `k8s.io/apimachinery/pkg/util/validation` else `ErrUsage`; `PreRunE` validates output is `text|json|yaml|mermaid` else `ErrUsage`; `SetFlagErrorFunc` wraps in `ErrUsage`; `RunE` sets `flags.Stdin = command.InOrStdin()`, calls the service, maps `simulate.StatusRouteNotFound` → `ErrRouteNotFound`, `StatusRouteIneligible` → `ErrRouteIneligible`, `StatusWaypointNotFound` → `ErrGatewayNotFound` (existing sentinel), renders with `render.RenderSimulation` and `colorEnabled(...)`, writes to stdout. For not-found/ineligible outcomes, render and print the eligible-route listing to stdout **before** returning the error (stderr gets the error via `main.go`). Add the `simulate.Status` string type and constants (`StatusFound`, `StatusWaypointNotFound`, `StatusRouteNotFound`, `StatusRouteIneligible`) in `internal/simulate/types.go` (distinct from the `Classification` type). **Outcome**: `istio-route simulate --help` works; arguments parsed and delegated; no business logic in Cobra. **Tests**: see T010.
- [X] T009 Edit `internal/cli/root.go` to add `cmd.AddCommand(newSimulateCommand(NewDefaultSimulateService(), stdout, stderr))` next to the existing `newAuditCommand`/`newTestCommand` registrations (root.go:101-102), and create `internal/cli/simulate_service.go` with `NewDefaultSimulateService()` returning a placeholder service that returns `fmt.Errorf("simulate service is not wired")` until T033/T054. Edit `cmd/istio-route/main.go` `runWithService` to map `cli.ErrRouteNotFound` → exit 3 (next to `ErrGatewayNotFound`); `ErrRouteIneligible` already maps to 2 through `ErrUsage`. **Outcome**: `simulate` is a reserved subcommand; `analyze`/`test`/trace unchanged; exit mapping 2/3/1 in place. **Tests**: see T011.
- [X] T010 Create `internal/cli/simulate_test.go` (table-driven, fake `SimulateService` like `fakeGatewayAuditService` in audit_test.go:16-28): valid listing and impact invocations pass `WaypointName`, `Route`, and all source flags; `--route` malformed (`a`, `a/b/c`, `A/b`, empty) → `ErrUsage`; zero/two positional args → `ErrUsage`; bad `--output` → `ErrUsage`; service error propagates; `StatusRouteNotFound` → `errors.Is(err, ErrRouteNotFound)` and listing printed to stdout; `StatusRouteIneligible` → `ErrUsage`; stderr stays empty; stdin passed through for `-f -`. **Outcome**: Group 1 contract verified without a cluster. **Tests**: `go test -race ./internal/cli/ -run Simulate`.
- [X] T011 Extend `cmd/istio-route/main_test.go` with table cases: `simulate` with ErrRouteNotFound → 3, ErrRouteIneligible → 2, `ErrGatewayNotFound` → 3, generic error → 1, and `istio-route simulate` with no args → 2. **Outcome**: exit-code contract (FR-018) enforced. **Tests**: `go test -race ./cmd/istio-route/`.

### Local manifest integration (Group 6)

- [X] T012 Create `internal/simulate/input.go` with `ValidateLocalNamespaces(snapshot source.Snapshot, waypointName string, route *RouteRef) error`: for local mode only, return an input error naming the resource and "missing namespace" for (a) any `Gateway` whose name equals `waypointName` and (b) any `HTTPRoute` whose name equals `route.Name` or any `HTTPRoute` having a Gateway `parentRef` named `waypointName` (parent group/kind defaulting like `audit.ParentTargets`), when `metadata.namespace` is empty. The `--namespace` flag must **not** fill missing namespaces. Exit status for this error is 1 (plain error, not `ErrUsage`). Also add `RequireGatewayAPI(snapshot source.Snapshot) error` returning a plain error "no Gateway API resources were supplied" (exit 1) when no resource has group `gateway.networking.k8s.io`, so Istio-only manifests do not surface as "waypoint not found". **Outcome**: FR-024 and FR-020 (local) enforced; no wrong-namespace guesses. **Tests**: see T013.
- [X] T013 Create `internal/simulate/input_test.go` and fixture `testdata/manifests/simulate-missing-namespace.yaml` (Gateway and HTTPRoute without `metadata.namespace`): table cases — namespace-less waypoint Gateway → error; namespace-less selected route → error; namespace-less unrelated route → no error; all namespaced → no error; `RequireGatewayAPI` with `testdata/manifests/istio.yaml` (Istio-only) → error, with `simulate-impact.yaml` → no error. **Outcome**: Group 6 safety verified. **Tests**: `go test -race ./internal/simulate/ -run ValidateLocalNamespaces`.

### Ambient waypoint validation and selection (Group 2)

- [X] T014 Create `internal/simulate/waypoint.go` with `ResolveWaypoint(snapshot source.Snapshot, name, namespace string) (Waypoint, error)` using `audit.SelectGateway` (the T005 wrapper over `selectGateway`, audit/source.go:30-52, which already returns `audit.ErrAmbiguousGateway` for duplicates) and rules: not found → sentinel `ErrWaypointNotFound`; `audit.IsWaypointGateway` false → error wrapping `ErrNotWaypoint` (CLI maps to usage status 2); missing GatewayClass `istio-waypoint` in snapshot → `Waypoint.Validation = unconfirmed` with reason (does **not** fail). Collect every listener whose protocol is `HBONE`, `HTTP`, or `HTTPS` as an applicable listener with port, hostname, allowed route kinds/namespaces (HBONE is **not** rejected; it is the only listener on typical ambient waypoints). A waypoint with no applicable listener still resolves; routes are then reported ineligible by T018. **Outcome**: FR-004, FR-005, FR-023 behavior. **Tests**: see T015.
- [X] T015 Create `internal/simulate/waypoint_test.go` with fixtures `testdata/manifests/simulate-ingress.yaml` (Gateway with class `istio`) and reuse `testdata/manifests/gateway-audit-waypoint.yaml` (HBONE-only listener, class present): table cases — valid waypoint confirmed; class definition absent → `unconfirmed`; ingress Gateway → `ErrNotWaypoint`; HBONE-only waypoint resolves with one applicable listener; HBONE+HTTP waypoint resolves with both; a waypoint with only a TCP listener resolves with zero applicable listeners; unknown name → `ErrWaypointNotFound`; same name in two namespaces without namespace → ambiguous; with namespace → selected. **Outcome**: Group 2 verified. **Tests**: `go test -race ./internal/simulate/ -run Waypoint`.

### Attachment evidence and route discovery (Group 3)

- [X] T016 Create `internal/simulate/attachment.go` with `EvaluateAttachments(snapshot source.Snapshot, waypoint Waypoint) []RouteAttachments`: iterate HTTPRoutes only (ignore GRPCRoute); a route's **direct** parentRef matches via `audit.ParentTargets` (Service-parented refs do not match and are counted into a `ServiceParentedExcluded` list and an integer count for disclosure); parent refs without namespace refer to the route's namespace; `sectionName`/`port` restrict applicable listeners; listener is applicable only if protocol is HBONE, HTTP, or HTTPS (restrictions that cannot be established, for example HBONE listeners without `allowedRoutes`, make the state `unconfirmed` rather than rejecting), route kind HTTPRoute is allowed, route namespace passes `allowedRoutes.namespaces` (`Same`, `All`; `Selector` without Namespace metadata in snapshot → unconfirmed), and hostnames intersect via `resolve.HostnamesIntersect`. Per listener derive state from `route.Status.Parents` entries matching the waypoint parent (and `controllerName` when present): an Accepted condition is **current only when** `condition.ObservedGeneration` and `route.Generation` are both present, nonzero, and equal; current True → `accepted`, current False → `rejected`, anything else (missing status, zero/mismatched generation, status without listener scope) → `unconfirmed`. A rejection applies only to the listener it was reported for. **Outcome**: FR-006, FR-014, FR-025 attachment evidence per listener. **Tests**: see T017.
- [X] T017 Create `internal/simulate/attachment_test.go` with fixture `testdata/manifests/simulate-attachment.yaml` covering acceptance scenarios US2 #4–#11 and US3: accepted on A / rejected on B; accepted on A / unconfirmed on B; observedGeneration mismatched, zero, and missing; rejected current; Service-parented route excluded and disclosed; section name selects only listener A; same-named waypoint in a different namespace with parentRef omitting namespace → not attached; route allowed-namespaces `Same` from another namespace → not applicable; HBONE-only waypoint accepts a route with no `sectionName`; waypoint with no applicable listener (TCP only) → route has no applicable listeners (reported ineligible by T018). **Outcome**: attachment semantics proven. **Tests**: `go test -race ./internal/simulate/ -run Attachment`.
- [X] T018 Create `internal/simulate/discovery.go` with `EligibleRoutes(attachments []RouteAttachments) []RouteListing` (eligible = at least one listener `accepted` or `unconfirmed`; routes rejected on **every** applicable listener, or when the waypoint has no applicable listener (reason "no applicable listeners"), are ineligible; sorted by namespace/name; each listing carries hostnames, per-listener state+reason) and `SelectRoute(snapshot, attachments, ref RouteRef) (Selection, error)` returning `ErrRouteNotFound` when no HTTPRoute has that exact namespace/name, `ErrRouteIneligible` when it exists but is rejected on all applicable listeners or not attached. **Outcome**: FR-007/FR-008 selection logic. **Tests**: see T019.
- [X] T019 Create `internal/simulate/discovery_test.go`: table cases for US2 #1 (3 attached, 2 on another waypoint → exactly 3 listed), #2 (none attached → empty list, no error), #3 (exists but not attached → `ErrRouteIneligible`), missing route → `ErrRouteNotFound`, rejected on all listeners → ineligible, a waypoint with no applicable listener → all routes ineligible with reason "no applicable listeners", unconfirmed → eligible and labeled, deterministic ordering across input permutations. **Outcome**: Group 3 verified. **Tests**: `go test -race ./internal/simulate/ -run Discovery`.

**Checkpoint**: Foundation ready — a waypoint can be validated, routes discovered and selected, and `simulate` exists as a subcommand.

---

## Phase 3: User Story 1 — See what breaks if I remove an HTTPRoute (Priority: P1) 🎯 MVP

**Goal**: Given a local manifest set, a waypoint, and a route, report which traffic of the route becomes unrouted, stays covered, is unchanged, or is undetermined.

**Independent Test**: `go run ./cmd/istio-route simulate -f testdata/manifests/simulate-impact.yaml -n mesh --route shop/primary waypoint` shows `still-covered` (naming `shop/fallback`) and `unrouted` portions and the file hash is unchanged.

- [X] T020 [P] [US1] Create fixtures under `testdata/manifests/`: `simulate-impact.yaml` (GatewayClass `istio-waypoint`; Gateway `mesh/waypoint` with an HBONE `mesh :15008` listener plus HTTP `:80` and HTTPS `:443` listeners; HTTPRoutes `shop/primary` with rule 1 `GET` PathPrefix `/api` → `shop/api:8080` and rule 2 `POST` PathPrefix `/admin` → `shop/admin:8080`, and `shop/fallback` with a single `GET` PathPrefix `/` rule → `shop/web:80`; explicit namespaces, `metadata.generation: 1`, and `status.parents` Accepted True with `observedGeneration: 1` for the waypoint) so that `/api` GET is **still covered** by `shop/fallback` while `/admin` POST is **unrouted**, matching [quickstart.md](./quickstart.md); plus variants `simulate-sole-route.yaml` (only `shop/primary`), `simulate-partial.yaml` (one `primary` rule matches GET and POST on `/api`, fallback covers only GET), and `simulate-shadowed.yaml` (a higher-precedence Exact `/api` route from another HTTPRoute shadows the target rule). **Outcome**: reusable offline inputs for US1 scenarios 1–4. **Tests**: loaded by `FilesSource` in T028/T030 (`go test ./internal/simulate/`).
- [X] T021 [US1] Create `internal/simulate/removal.go` with `Without(snapshot source.Snapshot, target RouteRef) source.Snapshot` that builds a **new** `source.Snapshot` whose `Resources` slice excludes only the resource with group `gateway.networking.k8s.io`, kind `HTTPRoute`, and the exact namespace/name; no mutation of the original slice or objects (allocate a new slice; do not reuse the backing array). (Group 4) **Outcome**: in-memory removal only. **Tests**: see T022.
- [X] T022 [US1] Create `internal/simulate/removal_test.go`: original snapshot unchanged (length, order, pointer identity of retained objects); same-name route in another namespace is **not** removed; a Service/other-kind resource with the same name is retained; removing a non-existent identity returns an equal copy; a file-hash test loading `simulate-impact.yaml` and asserting bytes unchanged. (Group 4) **Outcome**: FR-002 / SC-002 verified at domain level. **Tests**: `go test -race ./internal/simulate/ -run Removal`.
- [X] T023 [US1] Create `internal/simulate/predicate.go` implementing traffic-region predicates in two ordered steps. **Step 1 (constructors)**: `FromMatch(host string, match audit.Match) Predicate` (missing path ⇒ PathPrefix `/`, missing method ⇒ any), `And`, `Or`, `Not`, and a deterministic canonical string form; verify with the constructor and canonical-form cases of T024 before continuing. **Step 2 (relations)**: `Relate(a, b Predicate) Relation` returning `Equal|Subset|Superset|Disjoint|Overlap|Unknown`. Provable rules only: hostnames via `resolve.HostnamesIntersect` and wildcard matching (`resolve.MatchesHostname`); paths — Exact vs Exact by equality, Exact vs Prefix by segment-aware prefix (`/api` does **not** cover `/apix`), Prefix vs Prefix by segment-aware containment; methods by equality/any. Any header, query, or RegularExpression atom that is not textually identical on both sides ⇒ `Unknown`. (Group 5) **Outcome**: conservative set algebra; unknown stays unknown. **Tests**: see T024.
- [X] T024 [US1] Create `internal/simulate/predicate_test.go`: table cases — `/api` vs `/apix` (disjoint), `/api` vs `/api/v2` (superset), `/` covers all, Exact vs Prefix, wildcard hosts, `*.example.com` vs `a.b.example.com`, method any vs GET, identical header atoms equal, different header atoms Unknown, regex Unknown, disjunction (`Or`) subset, exclusion (`Not`) expressing "prefix `/api` excluding prefix `/api/v2`"; deterministic canonical string form for goldens. (Group 5) **Outcome**: Group 5 algebra locked. **Tests**: `go test -race ./internal/simulate/ -run Predicate`.
- [X] T025 [US1] Create `internal/simulate/compare.go` with `BuildView(snapshot source.Snapshot, waypoint Waypoint, attachments []RouteAttachments) View` producing, per applicable listener, the list of candidate rules (`audit.Route`/`audit.Rule` built from direct-parented HTTPRoutes using `audit.HTTPMatchFrom`; only `accepted` or `unconfirmed` attachments count; `rejected` never counts) and `Winner(view View, listener string, region Predicate) (Provider, certainty)` choosing the best **matching** candidate for a region using `audit.CompareRules` (not a rule-wide best match unrelated to the region); undecided precedence or any candidate whose relation to the region is `Unknown` or only `Overlap` ⇒ certainty `undetermined`. Different listeners never cover one another. (Group 5) **Outcome**: before/after routing views reuse the 004 comparator. **Tests**: see T026.
- [X] T026 [US1] Create `internal/simulate/compare_test.go`: winner selection for Exact beats Prefix, longer prefix, method, creation-time tie-break; target not winning because another route beats it (⇒ later `no-change`); union of two remaining routes covering a region; different listeners isolated; unconfirmed candidate yields provisional (not definitive) winner; shuffle-input permutation test yields identical views. (Group 5) **Outcome**: Group 5 view verified. **Tests**: `go test -race ./internal/simulate/ -run Compare`.
- [X] T027 [US1] Create `internal/simulate/impact.go` with `BuildImpact(before, after View, target RouteRef, waypoint Waypoint, scope Scope) Result` in three ordered steps, each verified by its own T028 cases before moving on. **Step 1 (regions)**: for each rule of the target and each applicable listener derive the rule's **effective-before region** (its match regions × effective hostnames minus regions won by other rules; hostnames default to listener hostnames, or "all hostnames" when none). **Step 2 (classification)**: for each region compute the after-winner and classify: `no-change` (target was not the before-winner), `unrouted` (no in-scope remaining candidate matches), `still-covered` (a remaining candidate provably serves it; record `after` provider, rule, backends, and set `BackendChanged` when the after backends differ from the removed rule's backends), `undetermined` (relation unknown, unconfirmed attachment or unconfirmed waypoint validation involved; set `provisional` with the configuration-based outcome). Split a region into covered and unrouted portions (using `Not`) when a remaining candidate covers only part. **Step 3 (summaries)**: produce `RuleSummary` (outcome set, counts, `partial` when both covered and unrouted portions exist) and `Totals`; sort deterministically by listener, rule index, canonical region. Report backends from the HTTPRoute references only (namespace defaults to route namespace). (Group 5) **Outcome**: FR-009–FR-014 results as typed data. **Tests**: see T028.
- [X] T028 [US1] Create `internal/simulate/impact_test.go` (table-driven, using fixtures from T020 via `source.FilesSource`): US1 #1 (sole route ⇒ `unrouted`, names removed rule and backend `shop/api`), #2 (fallback covers ⇒ `still-covered` naming `shop/fallback` rule 1 and `shop/web:80`, with `backendChanged` true because `shop/api` differs from `shop/web`; a variant where the fallback uses the same backend has `backendChanged` false), #3 (partial: covered and unrouted portions plus `partial` summary), #4 (shadowed ⇒ `no-change`), only-route edge case reports no remaining in-scope routes in `warnings`, backend in another namespace reported as written, GRPCRoute ignored, catch-all rule (no matches) treated as PathPrefix `/`, `/api` target with fallback `/apix` stays `unrouted`; each rule appears in a `RuleSummary`; deterministic under input permutation. (Group 5) **Outcome**: SC-004 verified for the domain. **Tests**: `go test -race ./internal/simulate/ -run Impact`.
- [X] T029 [US1] Create `internal/simulate/simulate.go` with `Run(ctx context.Context, request Request, snapshot source.Snapshot) (Result, Status, error)` orchestrating: `ctx.Err()` check, local namespace validation and `RequireGatewayAPI` (T012) when `snapshot.Metadata.Mode == model.SourceModeFiles`, `ResolveWaypoint` (T014), `EvaluateAttachments` (T016), listing when `request.Route == nil` (full listing behavior in US2), `SelectRoute` (T018), `Without` (T021), `BuildView` before/after (T025), `BuildImpact` (T027); copy `snapshot.Metadata` into `Result.Source` and add the warning "supplied manifests' completeness is unverified" for local mode (FR-022) and the exclusion disclosure including the count of excluded Service-parented routes (Service-parented and non-HTTPRoute routing). Define `Request{WaypointName, Namespace string; Route *RouteRef}`. (Group 4/5) **Outcome**: single domain entry point; snapshot never mutated. **Tests**: see T030.
- [X] T030 [US1] Create `internal/simulate/simulate_test.go` end-to-end domain tests over the T020 fixtures asserting the full `Result` for the independent-test scenario, and a read-only test that hashes (`sha256`) `simulate-impact.yaml` before and after `FilesSource.Snapshot` + `Run`. **Outcome**: US1 domain behavior and SC-002 for local files. **Tests**: `go test -race ./internal/simulate/`.
- [X] T031 [US1] Implement the text renderer for impact reports in `internal/render/simulate.go` (replace the T007 stub for `FormatText`) using the existing `newTextStyles` from `internal/render/style.go`: header with waypoint and removed route, source-scope warnings, then **separate visibly distinct groups** `UNROUTED`, `STILL COVERED`, `UNDETERMINED`, `NO CHANGE` (headings plus color only when `options.Color`), each entry listing listener, hostnames, path/method/header/query conditions, rule position, backend `namespace/name[:port]`, providers for covered entries, an explicit "backend changed: <before> → <after>" line when `BackendChanged` is true, `provisional` labels, then per-rule summaries and totals. (Group 8) **Outcome**: FR-010, FR-012, FR-013 readable output. **Tests**: golden in T032.
- [X] T032 [P] [US1] Create `internal/render/simulate_test.go` golden tests for text output of `simulate-impact.yaml` (`testdata/golden/simulate-impact.txt`) honoring `UPDATE_GOLDEN=1` like `TestAuditRenderFormatsGolden` (internal/render/audit_test.go:15-60), and a no-color test proving no ANSI escapes when `Options{Color:false}`. (Group 8) **Outcome**: stable text output. **Tests**: `go test -race ./internal/render/ -run Simulate`.
- [X] T033 [US1] Implement local mode in `internal/cli/simulate_service.go`: `defaultSimulateService.Simulate` — when `len(flags.Files) > 0` build `&source.FilesSource{Files, Stdin}`, call `Snapshot(ctx)`, run `simulate.Run`, map domain errors: `ErrWaypointNotFound` → `StatusWaypointNotFound`, `ErrNotWaypoint` and ambiguous → wrap `ErrUsage`, route outcomes through `Result`; never construct a Kubernetes client in this branch. (Group 6) **Outcome**: local mode works end to end and contacts no cluster. **Tests**: see T034.
- [X] T034 [US1] Extend `internal/cli/simulate_test.go` with `TestDefaultSimulateServiceUsesOfflineFiles` (pattern from `TestDefaultGatewayAuditServiceUsesOfflineFiles`, audit_test.go:111-130): impact run on `simulate-impact.yaml` with an unreadable `Kubeconfig` path proves no cluster access; file hash unchanged; `-f -` stdin variant; plus an error-path case (a manifest with a namespace-less selected route) that fails with exit-1-class error yet leaves the file hash unchanged. (Group 6) **Outcome**: US1 acceptance #5 for local mode on success and error. **Tests**: `go test -race ./internal/cli/`.

**Checkpoint**: US1 is shippable as the MVP for local manifests in text format.

---

## Phase 4: User Story 2 — Discover which HTTPRoutes are attached (Priority: P2)

**Goal**: With no `--route`, list eligible routes; with an unknown or ineligible route, list eligible routes and fail with the right exit code.

**Independent Test**: `simulate -f testdata/manifests/simulate-listing.yaml -n mesh waypoint` lists exactly the routes attached to that waypoint with state labels.

- [X] T035 [P] [US2] Create fixture `testdata/manifests/simulate-listing.yaml`: waypoints `mesh/waypoint` and `mesh/other`; 3 routes attached to `waypoint` (one with missing status ⇒ unconfirmed), 2 attached to `other`, one Service-parented route, one route rejected on all `waypoint` listeners (current generation), one GRPCRoute. **Outcome**: input for US2 scenarios. **Tests**: used by T037/T041.
- [X] T036 [US2] Complete listing in `internal/simulate/simulate.go`: when `Route == nil` return `Result{Kind: listing, Routes: EligibleRoutes(...)}` with hostnames, per-listener states/reasons, source-scope warning, and the Service-parented exclusion disclosure with its count; empty list is explicit (`routes: []`) and status `StatusFound`. (Group 3) **Outcome**: FR-007. **Tests**: see T037.
- [X] T037 [US2] Add listing tests in `internal/simulate/simulate_test.go`: exactly 3 routes listed; none attached ⇒ empty and success; unconfirmed route labeled; Service-parented and GRPCRoute excluded with disclosure; rejected-on-all route not listed. (Group 3) **Outcome**: US2 #1, #2, #4, #5, #9 verified. **Tests**: `go test -race ./internal/simulate/ -run Listing`.
- [X] T038 [US2] In `internal/simulate/simulate.go` return `StatusRouteNotFound` / `StatusRouteIneligible` together with a listing `Result` (never an impact report) when `SelectRoute` fails (T018). (Group 3) **Outcome**: FR-008. **Tests**: see T039.
- [X] T039 [US2] Add domain and CLI tests: missing route ⇒ listing + `ErrRouteNotFound` (exit 3); route attached only to another waypoint or rejected on every listener ⇒ listing + `ErrRouteIneligible` (exit 2), stderr carries the explanation, stdout has only the listing, no impact entries. Place in `internal/simulate/simulate_test.go` and `internal/cli/simulate_test.go`. (Group 3) **Outcome**: US2 #3, #9 and exit codes. **Tests**: `go test -race ./internal/simulate/ ./internal/cli/`.
- [X] T040 [US2] Implement listing text rendering in `internal/render/simulate.go` (route namespace/name, hostnames, listener, `accepted|rejected|unconfirmed` with reason, "no eligible HTTPRoutes" message, exclusion and source warnings). (Group 8) **Outcome**: readable listing. **Tests**: see T041.
- [X] T041 [P] [US2] Add golden `testdata/golden/simulate-listing.txt` and test in `internal/render/simulate_test.go`. (Group 8) **Outcome**: stable listing text. **Tests**: `go test -race ./internal/render/ -run Simulate`.

**Checkpoint**: US1 and US2 work independently for local manifests.

---

## Phase 5: User Story 3 — Select the right waypoint and reject non-waypoints (Priority: P2)

**Goal**: Results are scoped to one waypoint; non-waypoints and unsupported waypoints are rejected with the right status; uncertainty from missing class definitions is propagated.

**Independent Test**: With `simulate-multi-waypoint.yaml`, the same route simulated against waypoint A and B yields different, scoped results; the ingress Gateway exits 2; the HBONE-only waypoint resolves normally (empty listing, exit 0).

- [X] T042 [P] [US3] Create fixture `testdata/manifests/simulate-multi-waypoint.yaml`: waypoints `mesh/wp-a` and `mesh/wp-b`, ingress Gateway `edge/ingress`, a route `shop/primary` attached to both waypoints, replacement routes `shop/only-a` (parent wp-a) and `shop/only-b` (parent wp-b) serving the same hostname/path, and a variant `testdata/manifests/simulate-no-class.yaml` where the `istio-waypoint` GatewayClass object is absent. **Outcome**: multi-waypoint and provisional inputs. **Tests**: used by T043–T047.
- [X] T043 [US3] Ensure `BuildView`/`BuildImpact` (T025/T027) count only routes attached to the selected waypoint and add the other-parent note: when the removed route also has direct parentRefs to other Gateways, append a `scope` note and `warnings` entry naming them ("also attached to mesh/wp-b; impact not included"). Edit `internal/simulate/impact.go`. (Group 5) **Outcome**: US3 #1, #4; SC-003. **Tests**: T044.
- [X] T044 [US3] Add table tests in `internal/simulate/impact_test.go`: simulate `shop/primary` on `wp-a` — only `only-a` may cover; on `wp-b` — only `only-b`; 0 providers originate from the other waypoint (SC-003); other-attachment note present. (Group 2/5) **Outcome**: independent multi-waypoint selection. **Tests**: `go test -race ./internal/simulate/ -run Impact`.
- [X] T045 [US3] Propagate waypoint-validation uncertainty: when `Waypoint.Validation == unconfirmed`, every entry that would be `unrouted` or `still-covered` becomes `undetermined` with `provisional` set to the computed outcome and a reason "GatewayClass istio-waypoint definition unavailable". Edit `internal/simulate/impact.go`. (Group 2/5) **Outcome**: FR-004 provisional rule. **Tests**: T046.
- [X] T046 [US3] Add tests: `simulate-no-class.yaml` ⇒ all entries `undetermined` with provisional outcomes, `no-change` entries stay `no-change`; confirmed class ⇒ definitive outcomes; unconfirmed attachment (missing status) ⇒ `undetermined` with provisional. (Group 2/5) **Outcome**: US3 #5, US2 #4. **Tests**: `go test -race ./internal/simulate/`.
- [X] T047 [US3] Add CLI/service tests in `internal/cli/simulate_test.go` using the fixtures: ingress Gateway ⇒ `ErrUsage` mentioning "not an Istio ambient waypoint"; unknown waypoint ⇒ `ErrGatewayNotFound`; `edge/ingress` ⇒ `ErrUsage` (exit 2) and **empty stdout**; a manifest with no Gateway API resources (`testdata/manifests/istio.yaml`) ⇒ plain error (exit 1) and **empty stdout**; the HBONE-only waypoint (`testdata/manifests/gateway-audit-waypoint.yaml`, `-n app`) ⇒ success with an explicit empty listing (not rejected); duplicate waypoint names across namespaces without `-n` ⇒ `ErrUsage`; same with `-n` succeeds. (Group 2) **Outcome**: SC-005, FR-018, FR-023 end to end. **Tests**: `go test -race ./internal/cli/ ./cmd/istio-route/`.

**Checkpoint**: US1–US3 behave correctly in local mode.

---

## Phase 6: User Story 4 — Formats, live cluster, and conventions (Priority: P3)

**Goal**: JSON/YAML/Mermaid output with identical content; live read-only cluster mode following `analyze` conventions.

**Independent Test**: The same simulation renders in all four formats with identical entries; live mode via fake clients issues only `get`/`list` on allowed resources.

### Output formatting (Group 8)

- [X] T048 [US4] Implement JSON and YAML in `internal/render/simulate.go` mirroring `RenderAudit` (`json.MarshalIndent(result, "", "  ")` + trailing newline; `sigs.k8s.io/yaml`), so all logical fields in [contracts/cli.md](./contracts/cli.md) are serialized, including recursive predicates and provisional data. **Outcome**: machine-readable output. **Tests**: T050.
- [X] T049 [US4] Implement Mermaid in `internal/render/simulate.go` using the existing helpers `mermaidID`/`escapeMermaid` (internal/render/mermaid.go): nodes for waypoint, removed route, each entry (label includes outcome, listener, region summary), providers and backend Services; edges distinguish `unrouted`, `still-covered`, `undetermined`, `no-change`, and a still-covered edge with `BackendChanged` is labeled "backend changed"; listings render routes attached to the waypoint. **Outcome**: diagram carries the same entries. **Tests**: T050.
- [X] T050 [US4] Extend `internal/render/simulate_test.go` with goldens `simulate-impact.{json,yaml,mmd}` and a cross-format agreement test modeled on `TestAuditRenderFormatsAgreeOnPrecedence` (render_test.go: `summarizeAudit`/`summarizeText`): decode JSON/YAML into `simulate.Result` and compare per-entry (listener, rule, outcome) and totals with the text rendering and with Mermaid node labels. **Outcome**: SC-006. **Tests**: `go test -race ./internal/render/`.
- [X] T051 [P] [US4] Add tests in `internal/cli/simulate_test.go` that `NO_COLOR=1` and `--no-color` suppress styling and that a non-`*os.File` stdout disables color via `colorEnabled`; results only on stdout, nothing on stderr for successful runs. **Outcome**: FR-016/FR-017. **Tests**: `go test -race ./internal/cli/ -run Simulate`.

### Live Kubernetes integration (Group 7)

- [X] T052 [US4] Create `internal/source/waypoint.go` with `(*ClusterSource).WaypointSnapshot(ctx context.Context, name, namespace, routeNamespace, routeName string) (Snapshot, error)` (empty route strings mean no selected route; plain strings keep `internal/source` free of any import of `internal/simulate`) using the existing `kube.Reader` interface and the same Gateway API discovery helper used by `GatewaySnapshot` (internal/source/gateway.go:24-108): `GetGateway`, `GetGatewayClass` (NotFound tolerated ⇒ class omitted from snapshot), `ListHTTPRoutes(NamespaceAll)` keeping routes that directly target the Gateway **plus** the selected route if present (so an existing unattached route is distinguishable from a missing one), and `ListNamespaces` **only** when a selected-waypoint listener uses `allowedRoutes.namespaces.from: Selector`. Missing Gateway ⇒ `ErrGatewayNotFound` (existing sentinel). Do **not** read Services, EndpointSlices, Pods, GRPCRoutes, ReferenceGrants, Istio resources, or waypoint selectors. Record unavailable families via the existing source metadata so a missing Gateway API ⇒ error (FR-020) while a missing Istio API does not matter. **Outcome**: minimal read-only live snapshot. **Tests**: T053.
- [X] T053 [US4] Create `internal/source/waypoint_test.go` with fake clientsets (patterns in `internal/source/gateway_test.go` and `cluster_test.go`): returns selected waypoint, class, direct-parent routes across namespaces; keeps selected unattached route; class NotFound tolerated; Gateway NotFound ⇒ `ErrGatewayNotFound`; Gateway API absent ⇒ error; Forbidden/unauthorized on list ⇒ error (not "missing data"); canceled context returns promptly; **action assertion**: collect `Actions()` from all fake clients and require every verb ∈ {`get`, `list`} and every resource ∈ {`gateways`, `gatewayclasses`, `httproutes`, `namespaces`}; no pods/services/endpointslices/grpcroutes. **Outcome**: SC-002 for the cluster, FR-015, FR-021. **Tests**: `go test -race ./internal/source/`.
- [X] T054 [US4] Complete live mode in `internal/cli/simulate_service.go`: when no files, call `source.NewClusterSource(ctx, kubeconfig, context, namespace)` (as `AnalyzeGateway` does, audit.go:50-61), require a namespace else `fmt.Errorf("%w: --namespace is required when the kubeconfig has no default namespace", ErrUsage)`, call `WaypointSnapshot` passing `request.Route.Namespace`/`request.Route.Name` as plain strings (empty when listing), map `ErrGatewayNotFound` to `StatusWaypointNotFound`, log skipped unavailable families with `slog.Info` (stderr) like root.go:62-66, then `simulate.Run`. (Group 7) **Outcome**: live mode reuses CLI conventions. **Tests**: T055.
- [X] T055 [US4] Add CLI tests in `internal/cli/simulate_test.go`: kubeconfig without default namespace and no `-n` ⇒ `ErrUsage` mentioning `--namespace` (pattern: `TestDefaultGatewayAuditServiceRequiresNamespaceWithoutKubeconfigDefault`, audit_test.go:132-160); service-level test with an injected cluster-source factory (use existing `ClusterClientFactory` hook, internal/source/cluster.go:24-61) proving parity between file-based and fake-cluster snapshots of the same fixture (same `Result`). (Group 7) **Outcome**: local/live parity. **Tests**: `go test -race ./internal/cli/ ./internal/source/`.
- [X] T056 [US4] Review `deploy/rbac.yaml`: confirm existing `get`/`list` rules already cover `gateways`, `gatewayclasses`, `httproutes`, and `namespaces`; if any are missing add **only** `get`/`list` (never create/update/patch/delete); add a test in `internal/source/waypoint_test.go` (or a small `deploy` YAML check) asserting the rules mention no write verbs. (Group 7/10) **Outcome**: FR-021. **Tests**: `go test -race ./internal/source/`.

**Checkpoint**: All four stories complete in local and live modes.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [X] T057 [P] Update `README.md`: add a "Simulate removing an HTTPRoute" section after "Audit a Gateway" with local and live examples (`-f`, `-n`, `--route namespace/name`, listing mode, four output formats), explicit limitations (only direct waypoint-parented HTTPRoutes counted; Service-parented routes (with the reported exclusion count) and GRPCRoutes excluded; HBONE listeners treated as applicable with unconfirmed attachment when restrictions cannot be established; local results are snapshot-scoped; live reads are not atomic; "undetermined"/provisional meaning), the read-only/RBAC statement (no Services, Pods, or EndpointSlices read), and that `simulate` is a reserved root command. (Group 10) **Outcome**: user-facing docs (constitution: CLI options/formats/exit codes documented). **Tests**: render check by reading; links resolve.
- [X] T058 [P] Update the README "Exit codes" table with `istio-route simulate`: `0` completed (including unrouted/undetermined traffic and listings), `1` runtime/source/parsing/missing-Gateway-API-resources/missing-namespace/output error, `2` invalid usage (including non-waypoint target and ineligible route), `3` waypoint or HTTPRoute not found. (Group 10) **Outcome**: FR-018 documented. **Tests**: manual diff review against `cmd/istio-route/main.go`.
- [X] T059 Add `internal/simulate/readonly_test.go`: parse non-test Go files in `internal/simulate/` and `internal/source/waypoint.go` and fail if they contain calls named `Create`, `Update`, `Patch`, `Delete`, `DeleteCollection`, `Watch`, `os.WriteFile`, `os.Create`, `os.Rename`, `os.Remove`, or identifiers `GetPod`, `ListPods`, `ListEndpointSlices`, `GetService`, `ListServices`. **Outcome**: static guard for read-only/no pod analysis. **Tests**: `go test -race ./internal/simulate/ -run ReadOnly`.
- [X] T060 Add regression tests: `cmd/istio-route/main_test.go` asserting `analyze` and hostname trace outputs/exit codes unchanged and that `simulate` runs through the subcommand (not as a hostname); `internal/cli/root_test.go` case that `istio-route simulate` is dispatched to the subcommand. Run the **unmodified** existing suites `./internal/audit/... ./internal/render/... ./internal/cli/... ./internal/source/...`. **Outcome**: SC-007. **Tests**: `go test -race ./...`.
- [X] T061 Execute [quickstart.md](./quickstart.md) end to end (local listing/impact in all four formats, stdin hash check, the HBONE-only waypoint listing and the Istio-only manifest failing with status 1 via a built binary `go build -o /tmp/istio-route ./cmd/istio-route`); fix any quickstart/fixture mismatch. **Outcome**: documented flows work. **Tests**: documented commands succeed with expected exit statuses; delete `/tmp/istio-route` afterwards.
- [X] T062 Run the full quality gates: `gofumpt -l cmd internal` (no output), `go vet ./...`, `golangci-lint run`, `go test -race ./...`, and `go test -race -cover ./internal/...` confirming `internal/` coverage ≥ 80% (Constitution I). Fix any failure in the touched code only. **Outcome**: merge-ready. **Tests**: all commands pass.
- [X] T063 Update the Development section of `README.md` to reference `specs/005-httproute-impact-simulation/` alongside the existing feature specs (README "Development and validation"). (Group 10) **Outcome**: contributor docs current. **Tests**: link check.

---

## Dependencies & Execution Order

### Phase dependencies

- **Phase 1** (T001–T002): no dependencies.
- **Phase 2** (T003–T019): after Phase 1; **blocks all stories**. Internal order: T003→T004; T005→T006; T007→T008→T009→(T010, T011); T012→T013; T014→T015; T016→T017→T018→T019 (T014 precedes T016; T003 precedes T008, T014, T016).
- **US1** (T020–T034) after Phase 2. **US2** (T035–T041) after US1's T029. **US3** (T042–T047) after T027/T029. **US4** (T048–T056) after US1 text renderer scaffolding (T031 for shared render file) and T029; live tasks (T052–T056) depend only on Phase 2 + T029/T033.
- **Polish** (T057–T063) after the stories it documents; T062 last (T063 before T062).

### Story dependencies

- US1 is the MVP and has no story dependencies beyond Foundational.
- US2 and US3 extend `simulate.Run` and fixtures; they are testable independently once US1's `Run` exists.
- US4 renderers and live mode are independent of US2/US3 content but reuse `Run`.

### Within each story

Fixtures → domain implementation → tests → renderer → CLI wiring.

### Parallel opportunities

- T002 alongside T001; T003 ‖ T005 (different packages); T012 ‖ T014 after T003.
- T020 ‖ T021 ‖ T023 (different files); T032 ‖ T033 after T031.
- T035 ‖ T042 (fixtures); T041 ‖ T039.
- T048 ‖ T049 ‖ T052 (different files); T057 ‖ T058.

## Parallel Example: User Story 1

```text
T020 fixtures (testdata/manifests/simulate-*.yaml)
T021 removal.go        T023 predicate.go        # distinct files, then tests T022 / T024
T025 compare.go → T027 impact.go → T029 simulate.go
T032 golden test ‖ T033 CLI local service
```

## Implementation Strategy

### MVP first (User Story 1)

1. Phases 1–2, then US1 (T020–T034): local manifests, text output, impact classification.
2. **Stop and validate**: independent test for US1, quickstart local command, byte-identical input.

### Incremental delivery

1. Add US2 (listing and error paths) → validate exit codes 2/3.
2. Add US3 (multi-waypoint, provisional class, rejection) → validate SC-003/SC-005.
3. Add US4 (JSON/YAML/Mermaid, live mode, RBAC) → validate SC-006 and read-only action assertions.
4. Polish: docs, regression, gates.

## Notes

- Do not modify existing audit behavior; T005 adds wrappers only and T006 proves parity.
- If a precedence or coverage relation cannot be proven (header/query/regex differences, undecided ties, unconfirmed inputs) the result is `undetermined` — never guess.
- Commit after each task or logical group; keep tasks independently verifiable by the named tests.
