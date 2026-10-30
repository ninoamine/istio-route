package behavior

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
	"github.com/ninoamine/istio-route/internal/model"
)

var ErrRouteNotFound = errors.New("hostname has no matching route")

type TraceFunc func(context.Context, string) (model.Analysis, error)

type scenarioState struct {
	trace        TraceFunc
	manifestName string
	analysis     model.Analysis
	traced       bool
}

func (s *scenarioState) reset() {
	s.analysis = model.Analysis{}
	s.traced = false
}

func (s *scenarioState) traceHostname(ctx context.Context, hostname string) error {
	if s.trace == nil {
		return fmt.Errorf("trace hostname %q: trace function is not configured", hostname)
	}
	analysis, err := s.trace(ctx, hostname)
	if errors.Is(err, ErrRouteNotFound) {
		s.analysis = analysis
		s.traced = true
		return nil
	}
	if err != nil {
		return fmt.Errorf("trace hostname %q: %w", hostname, err)
	}
	s.analysis = analysis
	s.traced = true
	return nil
}

func (s *scenarioState) assertManifests(name string) error {
	if name != s.manifestName {
		actual := "unnamed manifest set"
		if s.manifestName != "" {
			actual = fmt.Sprintf("manifest set %q", s.manifestName)
		}
		return assertionFailure(fmt.Sprintf("manifests for %q", name), actual)
	}
	return nil
}

func (s *scenarioState) requireTrace() error {
	if !s.traced {
		return fmt.Errorf("expected a hostname trace before this assertion, got no trace")
	}
	return nil
}

func (s *scenarioState) assertHostnameFound(found bool) error {
	if err := s.requireTrace(); err != nil {
		return err
	}
	actual := "hostname not found"
	if len(s.analysis.Routes) > 0 {
		actual = "hostname found"
	}
	expected := "hostname not found"
	if found {
		expected = "hostname found"
	}
	if (len(s.analysis.Routes) > 0) != found {
		return assertionFailure(expected, actual)
	}
	return nil
}

func (s *scenarioState) assertRouteMatch(matches bool) error {
	if err := s.requireTrace(); err != nil {
		return err
	}
	actual := summarizeRoutes(s.analysis.Routes)
	if (len(s.analysis.Routes) > 0) != matches {
		expected := "at least one route"
		if !matches {
			expected = "no matching route"
		}
		return assertionFailure(expected, actual)
	}
	return nil
}

func (s *scenarioState) assertGateway(name string) error {
	if err := s.requireTrace(); err != nil {
		return err
	}
	for _, entry := range s.analysis.EntryPoints {
		if entry.Resource.Name == name || entry.Name == name {
			return nil
		}
	}
	actual := "no matching entry point"
	if len(s.analysis.EntryPoints) > 0 {
		names := make([]string, 0, len(s.analysis.EntryPoints))
		for _, entry := range s.analysis.EntryPoints {
			names = append(names, entry.Resource.Name+"/"+entry.Name)
		}
		actual = strings.Join(names, ", ")
	}
	return assertionFailure(fmt.Sprintf("gateway %q", name), actual)
}

func (s *scenarioState) assertRoute(index int, kind, identity string) error {
	if err := s.requireTrace(); err != nil {
		return err
	}
	if index < 1 || index > len(s.analysis.Routes) {
		return assertionFailure(
			fmt.Sprintf("route %d is %q %q", index, kind, identity),
			summarizeRoutes(s.analysis.Routes),
		)
	}
	route := s.analysis.Routes[index-1]
	actualID := route.Resource.Name
	if route.Resource.Namespace != "" {
		actualID = route.Resource.Namespace + "/" + actualID
	}
	if route.Resource.Kind != kind || actualID != identity {
		return assertionFailure(
			fmt.Sprintf("route %d is %q %q", index, kind, identity),
			fmt.Sprintf("%s %s", route.Resource.Kind, actualID),
		)
	}
	return nil
}

func (s *scenarioState) assertPathPrefix(routeIndex, ruleIndex int, expected string) error {
	if err := s.requireTrace(); err != nil {
		return err
	}
	rule, err := s.rule(routeIndex, ruleIndex)
	if err != nil {
		return assertionFailure(
			fmt.Sprintf("rule %d of route %d matches path prefix %q", ruleIndex, routeIndex, expected),
			err.Error(),
		)
	}
	for _, match := range rule.Matches {
		if strings.EqualFold(strings.TrimPrefix(match.PathType, "Path"), "Prefix") && match.Path == expected {
			return nil
		}
	}
	return assertionFailure(
		fmt.Sprintf("rule %d of route %d matches path prefix %q", ruleIndex, routeIndex, expected),
		summarizeMatches(rule.Matches),
	)
}

func (s *scenarioState) assertDestination(routeIndex, ruleIndex int, service string, port int) error {
	if err := s.requireTrace(); err != nil {
		return err
	}
	rule, err := s.rule(routeIndex, ruleIndex)
	if err != nil {
		return assertionFailure(
			fmt.Sprintf("rule %d of route %d sends traffic to service %q on port %d", ruleIndex, routeIndex, service, port),
			err.Error(),
		)
	}
	for _, destination := range rule.Destinations {
		if destination.Kind == model.DestinationKindService &&
			destination.Name == service && destination.Port != nil && int(*destination.Port) == port {
			return nil
		}
	}
	return assertionFailure(
		fmt.Sprintf("rule %d of route %d sends traffic to service %q on port %d", ruleIndex, routeIndex, service, port),
		summarizeDestinations(rule.Destinations),
	)
}

type weightedDestination struct {
	subset  string
	percent int
}

func (s *scenarioState) assertTrafficSplit(routeIndex, ruleIndex int, expected []weightedDestination) error {
	if err := s.requireTrace(); err != nil {
		return err
	}
	rule, err := s.rule(routeIndex, ruleIndex)
	expectedText := formatSplit(expected)
	if err != nil {
		return assertionFailure(expectedText, err.Error())
	}
	var total int64
	for _, destination := range rule.Destinations {
		if destination.Weight != nil {
			total += int64(*destination.Weight)
		}
	}
	actual := make([]weightedDestination, 0, len(rule.Destinations))
	for _, destination := range rule.Destinations {
		percent := 0
		if destination.Weight != nil && total > 0 {
			percent = int((int64(*destination.Weight)*100 + total/2) / total)
		}
		actual = append(actual, weightedDestination{subset: destination.Subset, percent: percent})
	}
	if len(actual) != len(expected) {
		return assertionFailure(expectedText, formatSplit(actual))
	}
	for i := range expected {
		if actual[i] != expected[i] {
			return assertionFailure(expectedText, formatSplit(actual))
		}
	}
	return nil
}

func (s *scenarioState) assertNoFindings(severity model.FindingSeverity) error {
	if err := s.requireTrace(); err != nil {
		return err
	}
	var findings []string
	for _, finding := range s.analysis.Findings {
		if finding.Severity == severity {
			findings = append(findings, fmt.Sprintf("%s: %s", finding.Severity, finding.Message))
		}
	}
	if len(findings) > 0 {
		return assertionFailure(fmt.Sprintf("no findings with severity %q", severity), strings.Join(findings, "; "))
	}
	return nil
}

func (s *scenarioState) assertFinding(severity model.FindingSeverity, message string) error {
	if err := s.requireTrace(); err != nil {
		return err
	}
	for _, finding := range s.analysis.Findings {
		if finding.Severity == severity && strings.Contains(finding.Message, message) {
			return nil
		}
	}
	var actual []string
	for _, finding := range s.analysis.Findings {
		actual = append(actual, fmt.Sprintf("%s: %s", finding.Severity, finding.Message))
	}
	if len(actual) == 0 {
		actual = append(actual, "no findings")
	}
	return assertionFailure(
		fmt.Sprintf("finding with severity %q and message containing %q", severity, message),
		strings.Join(actual, "; "),
	)
}

func (s *scenarioState) assertField(scope, path, expected string) error {
	if err := s.requireTrace(); err != nil {
		return err
	}
	actual, exists, err := lookupAnalysisField(s.analysis, path)
	if err != nil {
		return fmt.Errorf("invalid trace field %q: %w", path, err)
	}
	if !exists {
		actual = "<absent>"
	}
	if actual != expected {
		return assertionFailure(fmt.Sprintf("%s field %q equals %q", scope, path, expected), actual)
	}
	return nil
}

func (s *scenarioState) assertEntryPointField(index int, path, expected string) error {
	return s.assertEntityField("entry point", index, path, expected, "entryPoints")
}

func (s *scenarioState) assertRouteField(index int, path, expected string) error {
	return s.assertEntityField("route", index, path, expected, "routes")
}

func (s *scenarioState) assertFindingField(index int, path, expected string) error {
	return s.assertEntityField("finding", index, path, expected, "findings")
}

func (s *scenarioState) assertEntityField(kind string, index int, field, expected, list string) error {
	if index < 1 {
		return assertionFailure(fmt.Sprintf("%s index is at least 1", kind), strconv.Itoa(index))
	}
	return s.assertField(kind, fmt.Sprintf("%s[%d].%s", list, index-1, field), expected)
}

func (s *scenarioState) assertRuleField(routeIndex, ruleIndex int, field, expected string) error {
	return s.assertField(
		fmt.Sprintf("rule %d of route %d", ruleIndex, routeIndex),
		fmt.Sprintf("routes[%d].rules[%d].%s", routeIndex-1, ruleIndex-1, field),
		expected,
	)
}

func (s *scenarioState) assertDestinationField(routeIndex, ruleIndex, destinationIndex int, field, expected string) error {
	return s.assertField(
		fmt.Sprintf("destination %d of rule %d of route %d", destinationIndex, ruleIndex, routeIndex),
		fmt.Sprintf("routes[%d].rules[%d].destinations[%d].%s", routeIndex-1, ruleIndex-1, destinationIndex-1, field),
		expected,
	)
}

func (s *scenarioState) assertEndpointField(routeIndex, ruleIndex, destinationIndex, endpointIndex int, field, expected string) error {
	return s.assertField(
		fmt.Sprintf("endpoint %d of destination %d of rule %d of route %d", endpointIndex, destinationIndex, ruleIndex, routeIndex),
		fmt.Sprintf("routes[%d].rules[%d].destinations[%d].service.endpoints[%d].%s",
			routeIndex-1, ruleIndex-1, destinationIndex-1, endpointIndex-1, field),
		expected,
	)
}

func (s *scenarioState) rule(routeIndex, ruleIndex int) (model.Rule, error) {
	if routeIndex < 1 || routeIndex > len(s.analysis.Routes) {
		return model.Rule{}, fmt.Errorf("route %d is absent", routeIndex)
	}
	rules := s.analysis.Routes[routeIndex-1].Rules
	if ruleIndex < 1 || ruleIndex > len(rules) {
		return model.Rule{}, fmt.Errorf("rule %d of route %d is absent", ruleIndex, routeIndex)
	}
	return rules[ruleIndex-1], nil
}

func assertionFailure(expected, actual string) error {
	return fmt.Errorf("expected %s, got %s", expected, actual)
}

func summarizeRoutes(routes []model.Route) string {
	if len(routes) == 0 {
		return "no matching route"
	}
	var values []string
	for _, route := range routes {
		values = append(values, fmt.Sprintf("%s %s/%s", route.Resource.Kind, route.Resource.Namespace, route.Resource.Name))
	}
	return strings.Join(values, "; ")
}

func summarizeMatches(matches []model.Match) string {
	if len(matches) == 0 {
		return "no match conditions"
	}
	var values []string
	for _, match := range matches {
		values = append(values, fmt.Sprintf("%s %q", match.PathType, match.Path))
	}
	return strings.Join(values, "; ")
}

func summarizeDestinations(destinations []model.Destination) string {
	if len(destinations) == 0 {
		return "no destinations"
	}
	var values []string
	for _, destination := range destinations {
		value := fmt.Sprintf("%s %s/%s", destination.Kind, destination.Namespace, destination.Name)
		if destination.Port != nil {
			value += fmt.Sprintf(":%d", *destination.Port)
		}
		if destination.Subset != "" {
			value += " subset " + destination.Subset
		}
		values = append(values, value)
	}
	return strings.Join(values, "; ")
}

func formatSplit(destinations []weightedDestination) string {
	values := make([]string, 0, len(destinations))
	for _, destination := range destinations {
		values = append(values, fmt.Sprintf("%d%% %q", destination.percent, destination.subset))
	}
	return strings.Join(values, " and ")
}

func registerSteps(ctx *godog.ScenarioContext, state *scenarioState) {
	ctx.Step(`^the manifests for "([^"]+)"$`, state.assertManifests)
	ctx.Step(`^I trace the hostname "([^"]+)"$`, state.traceHostname)
	ctx.Step(`^the hostname should be found$`, func() error { return state.assertHostnameFound(true) })
	ctx.Step(`^the hostname should not be found$`, func() error { return state.assertHostnameFound(false) })
	ctx.Step(`^a route should match$`, func() error { return state.assertRouteMatch(true) })
	ctx.Step(`^no route should match$`, func() error { return state.assertRouteMatch(false) })
	ctx.Step(`^it should be exposed through gateway "([^"]+)"$`, state.assertGateway)
	ctx.Step(`^route (\d+) should be "([^"]+)" "([^"]+)"$`, state.assertRoute)
	ctx.Step(`^rule (\d+) of route (\d+) should match path prefix "([^"]+)"$`,
		func(ruleIndex, routeIndex int, path string) error {
			return state.assertPathPrefix(routeIndex, ruleIndex, path)
		})
	ctx.Step(`^rule (\d+) of route (\d+) should send traffic to service "([^"]+)" on port (\d+)$`,
		func(ruleIndex, routeIndex int, service string, port int) error {
			return state.assertDestination(routeIndex, ruleIndex, service, port)
		})
	ctx.Step(`^the traffic split for rule (\d+) of route (\d+) should be (\d+)% "([^"]+)" and (\d+)% "([^"]+)"$`,
		func(ruleIndex, routeIndex, firstPercent int, firstSubset string, secondPercent int, secondSubset string) error {
			return state.assertTrafficSplit(routeIndex, ruleIndex, []weightedDestination{
				{subset: firstSubset, percent: firstPercent},
				{subset: secondSubset, percent: secondPercent},
			})
		})
	ctx.Step(`^there should be no findings with severity "(info|warning|error)"$`,
		func(severity string) error { return state.assertNoFindings(model.FindingSeverity(severity)) })
	ctx.Step(`^there should be a finding with severity "(info|warning|error)" and message containing "([^"]+)"$`,
		func(severity, message string) error {
			return state.assertFinding(model.FindingSeverity(severity), message)
		})

	ctx.Step(`^trace field "([^"]+)" should equal "([^"]*)"$`,
		func(field, expected string) error { return state.assertField("trace", field, expected) })
	ctx.Step(`^entry point (\d+) field "([^"]+)" should equal "([^"]*)"$`, state.assertEntryPointField)
	ctx.Step(`^route (\d+) field "([^"]+)" should equal "([^"]*)"$`, state.assertRouteField)
	ctx.Step(`^rule (\d+) of route (\d+) field "([^"]+)" should equal "([^"]*)"$`,
		func(ruleIndex, routeIndex int, field, expected string) error {
			return state.assertRuleField(routeIndex, ruleIndex, field, expected)
		})
	ctx.Step(`^destination (\d+) of rule (\d+) of route (\d+) field "([^"]+)" should equal "([^"]*)"$`,
		func(destinationIndex, ruleIndex, routeIndex int, field, expected string) error {
			return state.assertDestinationField(routeIndex, ruleIndex, destinationIndex, field, expected)
		})
	ctx.Step(`^endpoint (\d+) of destination (\d+) of rule (\d+) of route (\d+) field "([^"]+)" should equal "([^"]*)"$`,
		func(endpointIndex, destinationIndex, ruleIndex, routeIndex int, field, expected string) error {
			return state.assertEndpointField(routeIndex, ruleIndex, destinationIndex, endpointIndex, field, expected)
		})
	ctx.Step(`^finding (\d+) field "([^"]+)" should equal "([^"]*)"$`, state.assertFindingField)
}
