Feature: Gateway API routing

  Scenario: HTTPRoute is attached to the gateway listener
    When I trace the hostname "shop.example.com"
    Then the hostname should be found
    And it should be exposed through gateway "shop"
    And route 1 should be "HTTPRoute" "shop/shop"
    And rule 1 of route 1 should match path prefix "/api"
    And rule 1 of route 1 should send traffic to service "api" on port 8080
