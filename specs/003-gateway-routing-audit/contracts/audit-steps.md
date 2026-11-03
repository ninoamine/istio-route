# Gherkin Contract: Gateway Audit Steps

## Execution Context

Each audit step runs against the immutable offline manifest set selected for that
scenario. Audit steps use a narrow audit callback supplied by the CLI; the behavior
package does not depend on Cobra or `internal/cli`. A missing Gateway is a valid
audit outcome that may be asserted by a scenario. Parse/source failures remain
invalid test inputs.

## Fixed Vocabulary Additions

| Step | Meaning |
|---|---|
| `When I audit gateway "infra/edge"` | Analyze the Gateway identity and retain the audit result; namespace/name is explicit for offline scenarios |
| `Then the gateway should be found` | Assert the audit target exists |
| `Then the gateway should not be found` | Assert the audit target is absent |
| `Then there should be a finding with code "missing-backend" severity "error" category "orphaned"` | Assert a finding's stable code, severity, and category |
| `Then finding "missing-backend" should involve "Service" "shop/api"` | Assert that a finding code includes the given kind and namespace/name |
| `Then there should be no finding with code "duplicate-route"` | Assert absence by code |
| `Then rule 1 of route "shop/catch-all" should win` | Assert the route rule is a precedence winner |
| `Then rule 1 of route "shop/old-route" should be shadowed` | Assert the route rule is fully shadowed |
| `Then rule 1 of route "shop/general" should partially overlap rule 1 of route "shop/specific"` | Assert partial overlap without claiming the general rule is unreachable |

Quoted audit-target, route, and Service identities use `namespace/name`. Finding
steps can be combined to assert the involved route, rule, backend, and severity.
Indexes in generic field assertions remain one-based for entities and zero-based
inside serialized lists.

## Assertion Semantics

- The audit result retains rejected and acceptance-unknown routes that name the
  selected Gateway as a parent.
- Missing Gateway is assertable and does not become an invalid step or source error.
- A finding absence assertion succeeds only when no finding with that code exists.
- `should be shadowed` means every request represented by the lower-ranked rule's
  match set is covered by higher-precedence rules. Partial intersection is asserted
  separately and does not count as complete shadowing.
- Assertions compare canonical kind and namespace/name identities, stable codes,
  severity, and category exactly.
- Adding audit steps must not alter existing step meanings, suggestions for
  unsupported existing steps, or existing JUnit XML fields.

The normative user-facing list of existing and new step patterns remains
`specs/002-routing-behavior-testing/step-reference.md`.
