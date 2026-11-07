# Quickstart: Gateway API Rule Precedence

**Date**: 2026-11-07 | **Feature**: [spec.md](./spec.md)

Runnable validation scenarios proving the lexicographic precedence comparator end-to-end.
See [data-model.md](./data-model.md) for the comparison model and
[contracts/cli.md](./contracts/cli.md) for the CLI contract.

## Prerequisites

- Go 1.27+, `gofumpt`, `golangci-lint` installed.
- Repository checked out on branch `004-gateway-api-precedence`.

## 1. Unit tests (comparator, tie-breakers, undecided)

```sh
go test -race ./internal/audit/...
```

Expected: table-driven tests pass for each criterion (Exact vs prefix+method+header,
short vs long prefix, multi-match vs Exact, method vs two headers), each tie-breaker
(creation timestamp, namespace/name, rule index), regex ranking, catch-all ranking, and
the undecided outcome. Overlap/coverage tests pass unmodified.

## 2. Permutation property tests (input-order independence)

```sh
go test -race ./internal/audit/ -run 'Permutation' -v
```

Expected: for every offline fixture, all tested permutations of input route/rule order
produce byte-identical serialized results (SC-002).

## 3. Offline fixtures across all four formats

```sh
go run ./cmd/istio-route audit gateway <gateway> --namespace <ns> \
  --from-files testdata/manifests/gateway-audit-precedence-*.yaml --output text
```

Repeat with `--output json|yaml|mermaid`. Expected: the spec-compliant winner, shadowed,
and partial-overlap rules per fixture; findings name the competing rule and deciding
criterion; all four formats agree on rules, states, and findings (SC-004); exit code 0.

## 4. Golden files

```sh
go test -race ./internal/render/... ./internal/cli/...
```

Expected: updated goldens under `testdata/golden/` match; the only diffs versus the
previous baseline are the documented precedence corrections and finding wording
(SC-005). `testdata/features/gateway-audit-overlap.feature` scenarios pass via the
existing Godog suite.

## 5. Quality gates

```sh
gofumpt -l . && go vet ./... && golangci-lint run && go test -race ./...
```

Expected: no formatting findings, no vet/lint errors, all tests pass with `-race`;
`internal/` coverage ≥80%.
