# Implementation Plan: Gateway API Rule Precedence

**Branch**: `004-gateway-api-precedence` | **Date**: 2026-11-07 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/004-gateway-api-precedence/spec.md`

## Summary

Replace the additive specificity score in the Gateway audit (`ruleSpecificity`/`matchSpecificity`
in `internal/audit/precedence.go`) with a deterministic, lexicographic comparator over each rule's
**best match**: (path type rank, prefix length in segments, method presence, header count, query
param count), followed by the existing tie-breaker chain (oldest creation timestamp, namespace/name
alphabetical, rule index). The comparator is centralized in `internal/audit` and returns both the
outcome and the deciding criterion so findings can name them (FR-008); all four output formats keep
consuming the single `audit.Result`, so consistency across text/JSON/YAML/Mermaid is preserved by
construction. Overlap (`rulesOverlap`) and coverage (`rulesCover`) logic and the CLI contract
(commands, formats, exit codes) remain unchanged. Equal-on-everything overlapping rules surface an
explicit "undecided" informational finding instead of an invented winner (FR-005).

## Technical Context

**Language/Version**: Go 1.27 (module `github.com/ninoamine/istio-route`)

**Primary Dependencies**: cobra (CLI), k8s.io/apimachinery (metav1.Time), sigs.k8s.io/gateway-api
types via `internal/source`; no new dependencies required.

**Storage**: N/A (offline manifests + read-only cluster reads; unchanged)

**Testing**: `go test -race ./...`; table-driven unit tests in `internal/audit`, permutation
property tests, golden files under `testdata/golden`, offline fixtures under `testdata/manifests`,
Cucumber/Godog feature files under `testdata/features`.

**Target Platform**: Linux/macOS/Windows CLI (amd64/arm64)

**Project Type**: CLI (cobra), business logic in `internal/`

**Performance Goals**: N/A (audit is O(rules²) pairwise; rule counts are small)

**Constraints**: Read-only cluster access; deterministic, input-order-independent results;
`gofumpt`, `go vet`, `golangci-lint run` must pass; ≥80% coverage in `internal/`.

**Scale/Scope**: One package touched (`internal/audit`), plus test fixtures/goldens and docs.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Quality Gates | PASS | Table-driven comparator tests, golden files for rendering, `-race` in CI; coverage stays ≥80%. |
| II. Domain Architecture | PASS | Change confined to `internal/audit` (domain logic). No cobra command changes; no `utils` package. |
| III. Predictable CLI UX | PASS | Commands, `--output json|yaml|text|mermaid`, exit codes 0/1/2/3 unchanged (FR-012). Finding message wording changes are documented in contracts/. |
| IV. Read-Only Cluster Access | PASS | No Kubernetes access changes; comparator is pure in-memory logic. |
| V. Reproducible Releases | PASS | No release/GoReleaser changes. |

No violations; Complexity Tracking not required.

## Project Structure

### Documentation (this feature)

```text
specs/004-gateway-api-precedence/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   └── cli.md           # CLI contract (unchanged surface, documented finding wording)
└── tasks.md             # Phase 2 output (/speckit-tasks - NOT created here)
```

### Source Code (repository root)

```text
internal/audit/
├── precedence.go        # comparator replaces ruleSpecificity/matchSpecificity; overlap/coverage unchanged
├── precedence_test.go   # table-driven comparator + tie-breaker + undecided tests
├── types.go             # PrecedenceState unchanged; no new exported states
└── audit.go             # unchanged call sites (applyPrecedence, precedenceFindings)

testdata/
├── manifests/           # new offline fixtures per precedence scenario
├── golden/              # updated/new goldens for text/json/yaml/mermaid audit output
└── features/            # gateway-audit-overlap.feature updated for new finding wording
```

**Structure Decision**: Single Go module, domain logic in `internal/audit` per Constitution II.
The comparator lives in `internal/audit/precedence.go` next to the existing overlap/coverage code;
renderers in `internal/render` keep consuming `audit.Result` untouched.

## Complexity Tracking

> No constitution violations; section intentionally empty.
