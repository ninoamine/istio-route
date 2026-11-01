Feature: Routing across environments

  @prod
  Scenario: Production uses the production destination
    Given the manifests for "prod"
    When I trace the hostname "shop.example.com"
    Then the hostname should be found
    And destination 1 of rule 1 of route 1 field "name" should equal "api-prod"

  @staging
  Scenario: Staging uses the staging destination
    Given the manifests for "staging"
    When I trace the hostname "shop.example.com"
    Then the hostname should be found
    And destination 1 of rule 1 of route 1 field "name" should equal "api-staging"

  Scenario: Shared hostname remains routed
    When I trace the hostname "shop.example.com"
    Then the hostname should be found

  @smoke
  Scenario: Ordinary tags do not select an environment
    When I trace the hostname "shop.example.com"
    Then the hostname should be found
