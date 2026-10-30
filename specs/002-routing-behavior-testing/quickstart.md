# Quickstart: Validate Routing Behavior

## Prerequisites

- Build or install `istio-route`.
- Render manifests before invoking the test command; the command does not render
  Helm charts or access a Kubernetes cluster.
- Write one or more Gherkin `.feature` files. See the [step vocabulary](./step-reference.md).

## Test one rendered manifest set

```sh
helm template shop ./chart -f values-prod.yaml > rendered-prod.yaml
istio-route test -f rendered-prod.yaml features/
```

Expected: each feature and scenario is listed as passed or failed. Assertion
failures include the expected value and actual trace information.

To stream rendered output directly into the test command:

```sh
helm template shop ./chart -f values-prod.yaml |
  istio-route test -f - features/
```

## Test named environments and publish JUnit

```sh
helm template shop ./chart -f values-prod.yaml > rendered-prod.yaml
helm template shop ./chart -f values-staging.yaml > rendered-staging.yaml
istio-route test \
  --manifests prod=rendered-prod.yaml \
  --manifests staging=rendered-staging.yaml \
  --junit report.xml \
  features/
```

Tag an environment-specific scenario with `@prod` or `@staging`. It runs only
against that named manifest set. Untagged scenarios run once against each set.
`Given the manifests for "prod"` confirms the set selected for that scenario; it
does not switch environments.

## Minimal feature example

```gherkin
Feature: Shop traffic

  @prod
  Scenario: Production API routes to the stable service
    Given the manifests for "prod"
    When I trace the hostname "shop.example.com"
    Then the hostname should be found
    And it should be exposed through gateway "public-gateway"
    And rule 1 of route 1 should send traffic to service "api" on port 8080
    And there should be no findings with severity "error"
```

## CI usage

The command returns 0 when all scenarios pass, 1 for assertion failures, and 2 for
invalid features/steps/inputs or report-write failures. The report is one JUnit XML
document covering all selected scenarios and environment executions. Existing
hostname trace invocations and their exit statuses are unchanged. GitHub Actions
and GitLab CI can publish the report as a test artifact:

### GitHub Actions

```yaml
name: Routing behavior
on: [push, pull_request]
jobs:
  routing:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: azure/setup-helm@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - run: go build -o istio-route ./cmd/istio-route
      - name: Test rendered routes
        run: helm template shop ./chart -f values-prod.yaml | ./istio-route test -f - --junit report.xml features/
      - if: always()
        uses: actions/upload-artifact@v4
        with:
          name: routing-junit
          path: report.xml
```

### GitLab CI

```yaml
stages: [build, test]

build-istio-route:
  image: golang:1.27
  stage: build
  script:
    - go build -o istio-route ./cmd/istio-route
  artifacts:
    paths: [istio-route]

routing-behavior:
  image: alpine/helm:3.18.6
  stage: test
  dependencies: [build-istio-route]
  script:
    - set -o pipefail
    - helm template shop ./chart -f values-prod.yaml | ./istio-route test -f - --junit report.xml features/
  artifacts:
    when: always
    reports:
      junit: report.xml
```

These are usage examples; production pipelines should pin third-party actions,
containers, and installer sources according to their normal dependency policy.

The complete supported step forms are listed in the [step vocabulary reference](./step-reference.md).

## Validation

Run the project checks after implementation:

```sh
go test -race ./...
go vet ./...
golangci-lint run
```
