package behavior

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"
)

func writeConsoleReport(output io.Writer, scenarios []scenarioResult) error {
	groups := make(map[string][]scenarioResult)
	var featureOrder []string
	for _, scenario := range scenarios {
		if _, exists := groups[scenario.uri]; !exists {
			featureOrder = append(featureOrder, scenario.uri)
		}
		groups[scenario.uri] = append(groups[scenario.uri], scenario)
	}

	passed, failed := 0, 0
	for _, uri := range featureOrder {
		featureScenarios := groups[uri]
		if _, err := fmt.Fprintf(output, "Feature: %s (%s)\n", featureScenarios[0].feature, uri); err != nil {
			return err
		}
		for _, scenario := range featureScenarios {
			if scenario.err == nil {
				passed++
				if _, err := fmt.Fprintf(output, "  PASS: %s%s\n", environmentLabel(scenario.environment), scenario.name); err != nil {
					return err
				}
				continue
			}
			failed++
			if _, err := fmt.Fprintf(output, "  FAIL: %s%s (%s:%d)\n", environmentLabel(scenario.environment), scenario.name, scenario.uri, scenario.line); err != nil {
				return err
			}
			details := failedStepDetails(scenario)
			if details == "" {
				details = scenario.err.Error()
				if _, err := fmt.Fprintf(output, "    %s\n", details); err != nil {
					return err
				}
				continue
			}
			for _, line := range strings.Split(details, "\n") {
				if _, err := fmt.Fprintf(output, "    %s\n", line); err != nil {
					return err
				}
			}
		}
	}
	_, err := fmt.Fprintf(output, "%d scenarios: %d passed, %d failed\n", len(scenarios), passed, failed)
	return err
}

func environmentLabel(name string) string {
	if name == "" {
		return ""
	}
	return "[" + name + "] "
}

func failedStepDetails(scenario scenarioResult) string {
	var details []string
	for _, event := range scenario.steps {
		if event.status == "passed" || event.status == "skipped" {
			continue
		}
		location := scenario.uri
		if event.line > 0 {
			location = fmt.Sprintf("%s:%d", location, event.line)
		}
		message := fmt.Sprintf("%s: %s", location, event.text)
		if event.err != nil {
			if event.status == "undefined" || event.status == "ambiguous" || event.status == "pending" {
				message += ": " + event.status + " step"
			} else {
				message += ": " + event.err.Error()
			}
		}
		details = append(details, message)
	}
	return strings.Join(details, "\n")
}

type junitReport struct {
	XMLName   xml.Name    `xml:"testsuite"`
	Name      string      `xml:"name,attr"`
	Tests     int         `xml:"tests,attr"`
	Failures  int         `xml:"failures,attr"`
	Errors    int         `xml:"errors,attr"`
	TestCases []junitCase `xml:"testcase"`
}

type junitCase struct {
	Name      string       `xml:"name,attr"`
	ClassName string       `xml:"classname,attr,omitempty"`
	Failure   *junitResult `xml:"failure,omitempty"`
	Error     *junitResult `xml:"error,omitempty"`
}

type junitResult struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

func writeJUnit(ctx context.Context, path string, scenarios []scenarioResult) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("write JUnit report: %w", err)
	}
	report := junitReport{Name: "istio-route behavior", Tests: len(scenarios)}
	for _, scenario := range scenarios {
		testCase := junitCase{
			Name:      environmentLabel(scenario.environment) + scenario.name,
			ClassName: scenario.uri,
		}
		if scenario.err != nil {
			text := scenario.err.Error()
			if details := failedStepDetails(scenario); details != "" {
				text += "\n" + details
			}
			result := &junitResult{Message: scenario.err.Error(), Text: text}
			if isInvalidStepError(scenario.err) {
				report.Errors++
				testCase.Error = result
			} else {
				report.Failures++
				testCase.Failure = result
			}
		}
		report.TestCases = append(report.TestCases, testCase)
	}
	data, err := xml.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode JUnit XML: %w", err)
	}
	data = append([]byte(xml.Header), data...)
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write report file: %w", err)
	}
	return nil
}
