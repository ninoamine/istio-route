package behavior

import (
	"strings"
)

var supportedExamples = []string{
	`the manifests for "prod"`,
	`When I trace the hostname "shop.example.com"`,
	`the hostname should be found`,
	`the hostname should not be found`,
	`a route should match`,
	`no route should match`,
	`it should be exposed through gateway "public-gateway"`,
	`route 1 should be "VirtualService" "shop/shop-vs"`,
	`rule 1 of route 1 should match path prefix "/api"`,
	`rule 1 of route 1 should send traffic to service "api" on port 8080`,
	`the traffic split for rule 1 of route 1 should be 90% "v1" and 10% "v2"`,
	`there should be no findings with severity "error"`,
	`there should be a finding with severity "warning" and message containing "not programmed"`,
	`trace field "hostname" should equal "shop.example.com"`,
	`entry point 1 field "protocol" should equal "HTTPS"`,
	`route 1 field "resource.kind" should equal "VirtualService"`,
	`rule 1 of route 1 field "matches[0].path" should equal "/api"`,
	`destination 1 of rule 1 of route 1 field "subset" should equal "v1"`,
	`endpoint 1 of destination 1 of rule 1 of route 1 field "ready" should equal "true"`,
	`finding 1 field "severity" should equal "warning"`,
}

func suggestStep(step string) string {
	step = strings.TrimSpace(step)
	if step == "" {
		return ""
	}
	best := supportedExamples[0]
	bestDistance := levenshtein(strings.ToLower(step), strings.ToLower(best))
	for _, candidate := range supportedExamples[1:] {
		distance := levenshtein(strings.ToLower(step), strings.ToLower(candidate))
		if distance < bestDistance {
			best = candidate
			bestDistance = distance
		}
	}
	return best
}

func levenshtein(first, second string) int {
	previous := make([]int, len(second)+1)
	for i := range previous {
		previous[i] = i
	}
	for i := 1; i <= len(first); i++ {
		current := make([]int, len(second)+1)
		current[0] = i
		for j := 1; j <= len(second); j++ {
			cost := 1
			if first[i-1] == second[j-1] {
				cost = 0
			}
			current[j] = min(
				current[j-1]+1,
				previous[j]+1,
				previous[j-1]+cost,
			)
		}
		previous = current
	}
	return previous[len(second)]
}
