Feature: Shop traffic routing
  Verify rendered routing configuration without a cluster.

  Background:
    When I trace the hostname "shop.example.com"

  Scenario: Production hostname is attached to the gateway
    Then the hostname should be found
    And it should be exposed through gateway "shop-gateway"
    And route 1 should be "VirtualService" "shop/shop"
    And rule 1 of route 1 should match path prefix "/api"
    And rule 1 of route 1 should send traffic to service "api" on port 8080
    And the traffic split for rule 1 of route 1 should be 90% "v1" and 10% "v2"
    And there should be a finding with severity "warning" and message containing "subset v2"
    And endpoint 1 of destination 1 of rule 1 of route 1 field "addresses[0]" should equal "10.0.0.10"

  Scenario Outline: Destination <destination> preserves subset <subset>
    Then destination <destination> of rule 1 of route 1 field "subset" should equal "<subset>"

    Examples:
      | destination | subset |
      | 1           | v1     |
      | 2           | v2     |

  Scenario: An unrelated hostname has no route
    When I trace the hostname "absent.example.com"
    Then the hostname should not be found
    And no route should match
