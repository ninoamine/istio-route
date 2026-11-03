Feature: Gateway audit scenarios
  Gateway API configuration can be checked in offline behavior scenarios.

  @healthy
  Scenario: A healthy gateway has no rejected routes
    When I audit gateway "infra/edge"
    Then the gateway should be found
    And there should be no finding with code "rejected-route"

  @rejected
  Scenario: A rejected route is reported
    When I audit gateway "infra/edge"
    Then there should be a finding with code "rejected-route" severity "error" category "orphaned"
    And finding "rejected-route" should involve "HTTPRoute" "shop/rejected"

  @missing-backend
  Scenario: A missing backend service is reported
    When I audit gateway "infra/edge"
    Then there should be a finding with code "backend-service-not-found" severity "error" category "orphaned"
    And finding "backend-service-not-found" should involve "Service" "shop/missing"

  @unused-grant
  Scenario: An unused ReferenceGrant is reported
    When I audit gateway "infra/edge"
    Then there should be a finding with code "unused-reference-grant" severity "warning" category "orphaned"
    And finding "unused-reference-grant" should involve "ReferenceGrant" "platform/unused"
    And there should be a finding with code "missing-reference-grant" severity "warning" category "orphaned"

  @duplicate
  Scenario: Duplicate route rules are reported
    When I audit gateway "infra/edge"
    Then there should be a finding with code "duplicate-route-rule" severity "warning" category "duplicated"
    And finding "duplicate-route-rule" should involve "HTTPRoute" "shop/route-b"
    And rule 1 of route "shop/route-a" should win
    And rule 1 of route "shop/route-b" should be shadowed

  @overlap
  Scenario: More specific overlapping rules take precedence
    When I audit gateway "infra/edge"
    Then rule 1 of route "shop/specific" should win
    And rule 1 of route "shop/general" should partially overlap rule 1 of route "shop/specific"

  @no-ready-endpoints
  Scenario: A winning route without ready endpoints is deficient
    When I audit gateway "infra/edge"
    Then there should be a finding with code "deficient-destination" severity "error" category "deficient-destination"
    And finding "deficient-destination" should involve "Service" "shop/api"

  @waypoint-selected
  Scenario: A selected waypoint is audited
    When I audit gateway "app/waypoint"
    Then the gateway should be found
    And there should be no finding with code "waypoint-not-found"

  @waypoint-missing
  Scenario: A waypoint label to a missing target is reported
    When I audit gateway "app/missing-waypoint"
    Then the gateway should not be found
    And there should be a finding with code "waypoint-not-found" severity "error" category "orphaned"
    And finding "waypoint-not-found" should involve "Namespace" "app"

  @unknown-gateway
  Scenario: An unknown gateway is an assertable outcome
    When I audit gateway "infra/absent"
    Then the gateway should not be found
