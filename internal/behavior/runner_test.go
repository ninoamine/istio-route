package behavior

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ninoamine/istio-route/internal/model"
)

func TestRunReportsPassFailureAndInvalidSteps(t *testing.T) {
	tests := []struct {
		name           string
		step           string
		wantFailed     bool
		wantInvalid    bool
		wantSuggestion bool
	}{
		{
			name: "passing assertion",
			step: "Then the hostname should not be found",
		},
		{
			name:       "assertion failure",
			step:       "Then the hostname should be found",
			wantFailed: true,
		},
		{
			name:           "undefined step",
			step:           `When I trace the host "shop.example.com"`,
			wantFailed:     true,
			wantInvalid:    true,
			wantSuggestion: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			featurePath := writeBehaviorFeature(t, "result.feature", fmt.Sprintf(`Feature: Run result
  Scenario: Scenario result
    When I trace the hostname "shop.example.com"
    %s
`, tt.step))
			var output, diagnostics bytes.Buffer
			result, err := Run(context.Background(), RunOptions{
				Paths: []string{featurePath},
				Environments: []Environment{{
					Trace: func(context.Context, string) (model.Analysis, error) {
						return model.Analysis{}, nil
					},
				}},
				Output:      &output,
				Diagnostics: &diagnostics,
			})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if result.Failed != tt.wantFailed || result.Invalid != tt.wantInvalid {
				t.Errorf("Run() result = %#v, want failed=%t invalid=%t", result, tt.wantFailed, tt.wantInvalid)
			}
			if tt.wantSuggestion && !strings.Contains(diagnostics.String(), "closest supported step") {
				t.Errorf("diagnostics missing step suggestion:\n%s", diagnostics.String())
			}
			if !strings.Contains(output.String(), "1 scenarios:") {
				t.Errorf("report missing the single scenario summary:\n%s", output.String())
			}
		})
	}
}

func TestRunRejectsInvalidOptionsAndInputs(t *testing.T) {
	featurePath := writeBehaviorFeature(t, "valid.feature", `Feature: Valid
  Scenario: Valid
    When I trace the hostname "shop.example.com"
    Then the hostname should not be found
`)
	trace := func(context.Context, string) (model.Analysis, error) {
		return model.Analysis{}, nil
	}
	base := RunOptions{
		Paths: []string{featurePath},
		Environments: []Environment{{
			Trace: trace,
		}},
		Output:      &bytes.Buffer{},
		Diagnostics: &bytes.Buffer{},
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name    string
		ctx     context.Context
		options RunOptions
		want    string
	}{
		{name: "missing output", options: RunOptions{Diagnostics: base.Diagnostics}, want: "output writer"},
		{name: "missing diagnostics", options: RunOptions{Output: base.Output}, want: "diagnostics writer"},
		{name: "missing feature path", options: RunOptions{Output: base.Output, Diagnostics: base.Diagnostics}, want: "feature file or directory"},
		{name: "no environments", options: RunOptions{Paths: base.Paths, Output: base.Output, Diagnostics: base.Diagnostics}, want: "at least one manifest environment"},
		{name: "missing trace function", options: RunOptions{Paths: base.Paths, Environments: []Environment{{}}, Output: base.Output, Diagnostics: base.Diagnostics}, want: "trace function is required"},
		{name: "duplicate environment", options: RunOptions{Paths: base.Paths, Environments: []Environment{{Name: "prod", Trace: trace}, {Name: "prod", Trace: trace}}, Output: base.Output, Diagnostics: base.Diagnostics}, want: "duplicate manifest environment"},
		{name: "mixed unnamed and named environments", options: RunOptions{Paths: base.Paths, Environments: []Environment{{Trace: trace}, {Name: "prod", Trace: trace}}, Output: base.Output, Diagnostics: base.Diagnostics}, want: "cannot mix unnamed and named"},
		{name: "canceled context", ctx: canceled, options: base, want: "context canceled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			_, err := Run(ctx, tt.options)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Run() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestRunPropagatesOutputAndReportErrors(t *testing.T) {
	validFeature := writeBehaviorFeature(t, "valid.feature", `Feature: Valid
  Scenario: Valid
    When I trace the hostname "shop.example.com"
    Then the hostname should not be found
`)
	undefinedFeature := writeBehaviorFeature(t, "undefined.feature", `Feature: Undefined
  Scenario: Undefined
    When I trace the host "shop.example.com"
`)
	trace := func(context.Context, string) (model.Analysis, error) {
		return model.Analysis{}, nil
	}
	environment := []Environment{{Trace: trace}}

	tests := []struct {
		name    string
		options RunOptions
		want    string
	}{
		{
			name: "console writer error",
			options: RunOptions{
				Paths: []string{validFeature}, Environments: environment,
				Output: errorWriter{}, Diagnostics: &bytes.Buffer{},
			},
			want: "write behavior test output",
		},
		{
			name: "suggestion writer error",
			options: RunOptions{
				Paths: []string{undefinedFeature}, Environments: environment,
				Output: &bytes.Buffer{}, Diagnostics: errorWriter{},
			},
			want: "write step suggestion",
		},
		{
			name: "JUnit report error",
			options: RunOptions{
				Paths: []string{validFeature}, Environments: environment,
				Output: &bytes.Buffer{}, Diagnostics: &bytes.Buffer{},
				JUnitPath: filepath.Join(t.TempDir(), "missing", "report.xml"),
			},
			want: "write JUnit report",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Run(context.Background(), tt.options)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Run() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestDiscoverFeatureFiles(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "features", "first.feature")
	second := filepath.Join(root, "features", "nested", "second.feature")
	writeFeatureFile(t, first, "Feature: First\n")
	writeFeatureFile(t, second, "Feature: Second\n")
	textFile := filepath.Join(root, "features", "notes.txt")
	writeFeatureFile(t, textFile, "not a feature")
	emptyDirectory := filepath.Join(root, "empty")
	if err := os.Mkdir(emptyDirectory, 0o700); err != nil {
		t.Fatalf("create empty feature directory: %v", err)
	}

	tests := []struct {
		name      string
		paths     []string
		wantCount int
		wantError string
	}{
		{name: "empty paths", wantError: "at least one feature"},
		{name: "recursive directory", paths: []string{filepath.Join(root, "features")}, wantCount: 2},
		{name: "duplicate file inputs", paths: []string{first, first}, wantCount: 1},
		{name: "empty directory", paths: []string{emptyDirectory}, wantError: "contains no .feature files"},
		{name: "missing input", paths: []string{filepath.Join(root, "missing.feature")}, wantError: "read feature path"},
		{name: "wrong extension", paths: []string{textFile}, wantError: "must have the .feature extension"},
		{name: "direct feature file", paths: []string{second}, wantCount: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := discoverFeatureFiles(tt.paths)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("discoverFeatureFiles() error = %v, want substring %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("discoverFeatureFiles() error = %v", err)
			}
			if len(got) != tt.wantCount {
				t.Errorf("discoverFeatureFiles() returned %d paths, want %d", len(got), tt.wantCount)
			}
		})
	}
}

func TestRunRejectsFeatureWithoutScenarios(t *testing.T) {
	featurePath := writeBehaviorFeature(t, "empty.feature", "Feature: Empty\n")
	var output, diagnostics bytes.Buffer
	result, err := Run(context.Background(), RunOptions{
		Paths: []string{featurePath},
		Environments: []Environment{{
			Name: "staging",
			Trace: func(context.Context, string) (model.Analysis, error) {
				return model.Analysis{}, nil
			},
		}},
		Output:      &output,
		Diagnostics: &diagnostics,
	})
	if err == nil || !strings.Contains(err.Error(), "no scenarios match") || !result.Invalid {
		t.Fatalf("Run() = (%#v, %v), want invalid result for no matching scenarios", result, err)
	}
}

func TestRun100ScenariosAcrossTwoEnvironmentsWithinFiveSeconds(t *testing.T) {
	const scenarioCount = 100
	var feature strings.Builder
	feature.WriteString("Feature: Performance\n")
	for index := range scenarioCount {
		fmt.Fprintf(&feature, "\n  Scenario: Scenario %03d\n", index+1)
		fmt.Fprintf(&feature, "    When I trace the hostname \"host-%03d.example.test\"\n", index+1)
		feature.WriteString("    Then the hostname should not be found\n")
	}

	featurePath := filepath.Join(t.TempDir(), "performance.feature")
	if err := os.WriteFile(featurePath, []byte(feature.String()), 0o600); err != nil {
		t.Fatalf("write performance feature: %v", err)
	}
	trace := func(context.Context, string) (model.Analysis, error) {
		return model.Analysis{}, nil
	}
	environments := []Environment{
		{Name: "prod", Trace: trace},
		{Name: "staging", Trace: trace},
	}
	names := []string{"prod", "staging"}

	start := time.Now()
	for _, environment := range environments {
		result, scenarios, err := runEnvironment(context.Background(), []string{featurePath}, environment, names)
		if err != nil {
			t.Fatalf("run %s scenarios: %v", environment.Name, err)
		}
		if result.Failed || result.Invalid {
			t.Errorf("%s run result = %#v, want all scenarios to pass", environment.Name, result)
		}
		if len(scenarios) != scenarioCount {
			t.Errorf("%s scenario count = %d, want %d", environment.Name, len(scenarios), scenarioCount)
		}
	}
	elapsed := time.Since(start)
	if elapsed >= 5*time.Second {
		t.Fatalf("100 scenarios across two environments took %s, want under 5s", elapsed)
	}
}

func writeBehaviorFeature(t *testing.T, name, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	writeFeatureFile(t, path, contents)
	return path
}

func writeFeatureFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create feature directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write feature %q: %v", path, err)
	}
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func BenchmarkRun100ScenariosAcrossTwoEnvironments(b *testing.B) {
	const scenarioCount = 100
	var feature strings.Builder
	feature.WriteString("Feature: Performance\n")
	for index := range scenarioCount {
		fmt.Fprintf(&feature, "\n  Scenario: Scenario %03d\n", index+1)
		fmt.Fprintf(&feature, "    When I trace the hostname \"host-%03d.example.test\"\n", index+1)
		feature.WriteString("    Then the hostname should not be found\n")
	}
	featurePath := filepath.Join(b.TempDir(), "performance.feature")
	if err := os.WriteFile(featurePath, []byte(feature.String()), 0o600); err != nil {
		b.Fatalf("write performance feature: %v", err)
	}
	trace := func(context.Context, string) (model.Analysis, error) {
		return model.Analysis{}, nil
	}
	environments := []Environment{
		{Name: "prod", Trace: trace},
		{Name: "staging", Trace: trace},
	}
	names := []string{"prod", "staging"}
	paths := []string{featurePath}
	ctx := context.Background()

	b.ResetTimer()
	for range b.N {
		for _, environment := range environments {
			result, scenarios, err := runEnvironment(ctx, paths, environment, names)
			if err != nil {
				b.Fatalf("run %s scenarios: %v", environment.Name, err)
			}
			if result.Failed || result.Invalid || len(scenarios) != scenarioCount {
				b.Fatalf("%s run result = %#v with %d scenarios", environment.Name, result, len(scenarios))
			}
		}
	}
	b.StopTimer()
}
