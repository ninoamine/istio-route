package behavior

import (
	"context"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	messages "github.com/cucumber/messages/go/v34"
)

func TestWriteJUnit(t *testing.T) {
	tests := []struct {
		name     string
		scenario scenarioResult
	}{
		{
			name: "passing outline execution",
			scenario: scenarioResult{
				feature: "Shop",
				name:    "Destination v1",
				uri:     "shop.feature",
			},
		},
		{
			name: "assertion failure",
			scenario: scenarioResult{
				feature: "Shop",
				name:    "Wrong destination",
				uri:     "shop.feature",
				err:     errors.New(`expected "api-v1", got "api-v2"`),
				steps: []stepEvent{{
					text:   `Then destination 1 should be "api-v1"`,
					status: "failed",
					line:   12,
					err:    errors.New(`expected "api-v1", got "api-v2"`),
				}},
			},
		},
		{
			name: "invalid step is a JUnit error",
			scenario: scenarioResult{
				feature:     "Shop",
				name:        "Unsupported wording",
				environment: "staging",
				uri:         "shop.feature",
				err:         ErrUnknownEnvironment,
				steps: []stepEvent{{
					text:   `the manifests for "qa"`,
					status: "failed",
					line:   20,
					err:    ErrUnknownEnvironment,
				}},
			},
		},
		{
			name: "expanded environment execution remains distinct",
			scenario: scenarioResult{
				feature:     "Shop",
				name:        "Destination v1",
				environment: "prod",
				uri:         "shop.feature",
			},
		},
	}
	scenarios := make([]scenarioResult, 0, len(tests))
	for _, tt := range tests {
		scenarios = append(scenarios, tt.scenario)
	}

	path := filepath.Join(t.TempDir(), "results.xml")
	if err := writeJUnit(context.Background(), path, scenarios); err != nil {
		t.Fatalf("writeJUnit() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read JUnit XML: %v", err)
	}

	var report struct {
		Tests    int `xml:"tests,attr"`
		Failures int `xml:"failures,attr"`
		Errors   int `xml:"errors,attr"`
		Cases    []struct {
			Name    string `xml:"name,attr"`
			Failure *struct {
				Message string `xml:"message,attr"`
				Text    string `xml:",chardata"`
			} `xml:"failure"`
			Error *struct {
				Message string `xml:"message,attr"`
				Text    string `xml:",chardata"`
			} `xml:"error"`
		} `xml:"testcase"`
	}
	if err := xml.Unmarshal(data, &report); err != nil {
		t.Fatalf("parse JUnit XML: %v\n%s", err, data)
	}
	if report.Tests != 4 || report.Failures != 1 || report.Errors != 1 || len(report.Cases) != 4 {
		t.Fatalf("JUnit counts = tests %d, failures %d, errors %d, cases %d; want 4, 1, 1, 4",
			report.Tests, report.Failures, report.Errors, len(report.Cases))
	}
	if report.Cases[1].Failure == nil ||
		!strings.Contains(report.Cases[1].Failure.Text, "shop.feature:12") ||
		!strings.Contains(report.Cases[1].Failure.Message, "expected") {
		t.Errorf("JUnit failure does not preserve step details: %#v", report.Cases[1])
	}
	if report.Cases[2].Error == nil || !strings.Contains(report.Cases[2].Name, "[staging]") {
		t.Errorf("JUnit error does not preserve environment or invalid status: %#v", report.Cases[2])
	}
	if !strings.Contains(report.Cases[3].Name, "[prod]") {
		t.Errorf("JUnit execution does not identify its environment: %#v", report.Cases[3])
	}
}

func TestWriteJUnitErrors(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		path string
		want string
	}{
		{
			name: "canceled context",
			ctx:  canceledContext(t),
			path: filepath.Join(t.TempDir(), "report.xml"),
			want: "context canceled",
		},
		{
			name: "unwritable report path",
			ctx:  context.Background(),
			path: filepath.Join(t.TempDir(), "missing", "report.xml"),
			want: "write report file",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := writeJUnit(tt.ctx, tt.path, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("writeJUnit() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func canceledContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestWriteConsoleReportIncludesFeatureStepLocationAndEnvironment(t *testing.T) {
	var output strings.Builder
	err := writeConsoleReport(&output, []scenarioResult{{
		feature:     "Shop routing",
		name:        "Uses the wrong service",
		environment: "prod",
		uri:         "shop.feature",
		line:        10,
		err:         errors.New(`expected "api-v1", got "api-v2"`),
		steps: []stepEvent{{
			text:   `destination 1 field "name" should equal "api-v1"`,
			status: "failed",
			line:   12,
			err:    errors.New(`expected "api-v1", got "api-v2"`),
		}},
	}})
	if err != nil {
		t.Fatalf("writeConsoleReport() error = %v", err)
	}
	for _, expected := range []string{
		"Feature: Shop routing",
		"FAIL: [prod] Uses the wrong service",
		"shop.feature:12: destination 1 field",
		`expected "api-v1", got "api-v2"`,
		"1 scenarios: 0 passed, 1 failed",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("report missing %q:\n%s", expected, output.String())
		}
	}
}

func TestWriteConsoleReportMatchesGolden(t *testing.T) {
	failure := errors.New(`expected "api-v1", got "api-v2"`)
	scenarios := []scenarioResult{
		{
			feature: "Shop routing",
			name:    "Routes API to v1",
			uri:     "routing.feature",
		},
		{
			feature: "Shop routing",
			name:    "Routes to expected service",
			uri:     "routing.feature",
			line:    10,
			err:     failure,
			steps: []stepEvent{{
				text:   `Then destination 1 of rule 1 of route 1 should send traffic to service "api-v1" on port 8080`,
				status: "failed",
				line:   12,
				err:    failure,
			}},
		},
	}

	var output strings.Builder
	if err := writeConsoleReport(&output, scenarios); err != nil {
		t.Fatalf("writeConsoleReport() error = %v", err)
	}
	goldenPath := filepath.Join("..", "..", "testdata", "golden", "behavior-report.txt")
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read console report golden file: %v", err)
	}
	if got := output.String(); got != string(golden) {
		t.Errorf("console report differs from golden file\n--- got ---\n%s\n--- want ---\n%s", got, golden)
	}
}

func TestReportFormatterCollectsFeaturePickleAndStepEvents(t *testing.T) {
	collector := &formatterCollector{
		features:   make(map[string]featureEvents),
		pickleIDs:  make(map[string]scenarioEvents),
		stepEvents: make(map[string][]stepEvent),
	}
	formatter := &reportFormatter{collector: collector}
	formatter.Feature(&messages.GherkinDocument{
		Feature: &messages.Feature{
			Name: "Routing",
			Children: []*messages.FeatureChild{{
				Scenario: &messages.Scenario{
					Steps: []*messages.Step{{
						Id:       "ast-step-1",
						Location: &messages.Location{Line: 7},
						Text:     `the hostname should be found`,
					}},
				},
			}},
		},
	}, "routing.feature", nil)
	pickle := &messages.Pickle{
		Id:       "pickle-1",
		Uri:      "routing.feature",
		Name:     "Host is routed",
		Location: &messages.Location{Line: 6},
	}
	formatter.Pickle(pickle)
	step := &messages.PickleStep{AstNodeIds: []string{"ast-step-1"}, Text: "the hostname should be found"}
	formatter.Ambiguous(pickle, step, nil, godog.ErrAmbiguous)

	if got := collector.feature("routing.feature").name; got != "Routing" {
		t.Errorf("feature name = %q, want Routing", got)
	}
	if got := collector.scenario("pickle-1"); got.name != "Host is routed" || got.line != 6 {
		t.Errorf("scenario event = %#v, want expanded scenario and source location", got)
	}
	events := collector.steps("pickle-1")
	if len(events) != 1 || events[0].status != "ambiguous" || events[0].line != 7 {
		t.Errorf("step events = %#v, want ambiguous event at routing.feature:7", events)
	}
}

func TestInvalidStepErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "undefined", err: godog.ErrUndefined, want: true},
		{name: "ambiguous", err: godog.ErrAmbiguous, want: true},
		{name: "pending", err: godog.ErrPending, want: true},
		{name: "unknown environment", err: ErrUnknownEnvironment, want: true},
		{name: "conflicting environment", err: ErrConflictingEnvironment, want: true},
		{name: "assertion failure", err: errors.New("assertion failed")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isInvalidStepError(tt.err); got != tt.want {
				t.Errorf("isInvalidStepError(%v) = %t, want %t", tt.err, got, tt.want)
			}
		})
	}
}
