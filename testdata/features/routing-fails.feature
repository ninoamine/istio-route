Feature: Incorrect shop routing expectation

  Scenario: Wrong route identity is reported
    When I trace the hostname "shop.example.com"
    Then route 1 should be "HTTPRoute" "shop/shop"
