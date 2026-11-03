package cli

import (
	"bytes"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTestCommand(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantErr     error
		wantErrText string
		wantOutput  []string
		wantStderr  []string
	}{
		{
			name: "passing feature with background and outline",
			args: []string{
				"-f", "../../testdata/manifests/behavior.yaml",
				"../../testdata/features/routing-passes.feature",
			},
			wantOutput: []string{
				"Shop traffic routing",
				"Production hostname is attached to the gateway",
				"Destination 1 preserves subset v1",
				"Destination 2 preserves subset v2",
				"An unrelated hostname has no route",
				"passed",
			},
		},
		{
			name: "Gateway API feature",
			args: []string{
				"-f", "../../testdata/manifests/gateway-api.yaml",
				"../../testdata/features/gateway-api.feature",
			},
			wantOutput: []string{
				"Gateway API routing",
				"HTTPRoute is attached to the gateway listener",
				"passed",
			},
		},
		{
			name: "failing assertion reports expected and actual",
			args: []string{
				"-f", "../../testdata/manifests/behavior.yaml",
				"../../testdata/features/routing-fails.feature",
			},
			wantErr:    ErrTestFailure,
			wantOutput: []string{`expected route 1 is "HTTPRoute" "shop/shop", got VirtualService shop/shop`},
		},
		{
			name:        "requires rendered manifests",
			args:        []string{"../../testdata/features/routing-passes.feature"},
			wantErr:     ErrTestInvalid,
			wantErrText: "at least one rendered manifest file is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			command := NewRootCommand(nil, stdout, stderr)
			command.SetArgs(append([]string{"test"}, tt.args...))
			err := command.Execute()
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("Execute() error = %v, want errors.Is(_, %v)", err, tt.wantErr)
			}
			if tt.wantErrText != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErrText)) {
				t.Fatalf("Execute() error = %v, want substring %q", err, tt.wantErrText)
			}
			if tt.wantErr == nil && err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			for _, expected := range tt.wantOutput {
				if !strings.Contains(stdout.String(), expected) {
					t.Errorf("stdout missing %q:\n%s", expected, stdout.String())
				}
			}
			for _, expected := range tt.wantStderr {
				if !strings.Contains(stderr.String(), expected) {
					t.Errorf("stderr missing %q:\n%s", expected, stderr.String())
				}
			}
		})
	}
}

func TestTestCommandReadsManifestsFromStdin(t *testing.T) {
	manifest, err := os.ReadFile("../../testdata/manifests/behavior.yaml")
	if err != nil {
		t.Fatalf("read fixture manifest: %v", err)
	}
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	command := NewRootCommand(nil, stdout, stderr)
	command.SetIn(bytes.NewReader(manifest))
	command.SetArgs([]string{
		"test", "-f", "-", "../../testdata/features/routing-passes.feature",
	})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout.String(), "Production hostname is attached to the gateway") {
		t.Errorf("stdout does not include executed feature:\n%s", stdout.String())
	}
}

func TestTestCommandRunsNamedEnvironments(t *testing.T) {
	reportPath := filepath.Join(t.TempDir(), "environments.xml")
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	command := NewRootCommand(nil, stdout, stderr)
	command.SetArgs([]string{
		"test",
		"--manifests", "prod=../../testdata/manifests/behavior-prod.yaml",
		"--manifests", "staging=../../testdata/manifests/behavior-staging.yaml",
		"--junit", reportPath,
		"../../testdata/features/environments.feature",
	})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	report := stdout.String()
	if !strings.Contains(report, "6 scenarios: 6 passed, 0 failed") {
		t.Errorf("stdout missing six passing environment executions:\n%s", report)
	}
	if got := strings.Count(report, "PASS: [prod]"); got != 3 {
		t.Errorf("production execution count = %d, want 3:\n%s", got, report)
	}
	if got := strings.Count(report, "PASS: [staging]"); got != 3 {
		t.Errorf("staging execution count = %d, want 3:\n%s", got, report)
	}
	if strings.Contains(report, "FAIL:") {
		t.Errorf("stdout includes a failed cross-environment assertion:\n%s", report)
	}
	xmlData, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read multi-environment JUnit report: %v", err)
	}
	if !bytes.Contains(xmlData, []byte(`tests="6"`)) ||
		bytes.Count(xmlData, []byte("<testcase ")) != 6 ||
		!bytes.Contains(xmlData, []byte(`name="[prod] Production uses the production destination"`)) ||
		!bytes.Contains(xmlData, []byte(`name="[staging] Staging uses the staging destination"`)) {
		t.Errorf("JUnit report does not include each environment execution:\n%s", xmlData)
	}
}

func TestTestCommandRunsGatewayAuditScenariosOffline(t *testing.T) {
	reportPath := filepath.Join(t.TempDir(), "gateway-audit.xml")
	manifestSets := []struct {
		name string
		file string
	}{
		{name: "healthy", file: "gateway-audit-healthy.yaml"},
		{name: "rejected", file: "gateway-audit-rejected.yaml"},
		{name: "missing-backend", file: "gateway-audit-missing-backend.yaml"},
		{name: "unused-grant", file: "gateway-audit-unused-grant.yaml"},
		{name: "duplicate", file: "gateway-audit-duplicate.yaml"},
		{name: "overlap", file: "gateway-audit-overlap.yaml"},
		{name: "no-ready-endpoints", file: "gateway-audit-no-ready-endpoints.yaml"},
		{name: "waypoint-selected", file: "gateway-audit-waypoint.yaml"},
		{name: "waypoint-missing", file: "gateway-audit-missing-waypoint-target.yaml"},
		{name: "unknown-gateway", file: "gateway-audit-healthy.yaml"},
	}
	args := []string{"test"}
	for _, set := range manifestSets {
		args = append(args, "--manifests", set.name+"=../../testdata/manifests/"+set.file)
	}
	args = append(args, "--junit", reportPath, "../../testdata/features/gateway-audit-overlap.feature")

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	command := NewRootCommand(nil, stdout, stderr)
	command.SetArgs(args)
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout.String(), "10 scenarios: 10 passed, 0 failed") {
		t.Errorf("stdout does not report ten passing audit scenarios:\n%s", stdout.String())
	}
	if strings.Contains(stderr.String(), "unsupported or ambiguous step") {
		t.Errorf("audit steps were not registered:\n%s", stderr.String())
	}

	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read audit JUnit report: %v", err)
	}
	var report struct {
		Tests    int `xml:"tests,attr"`
		Failures int `xml:"failures,attr"`
		Cases    []struct {
			Name string `xml:"name,attr"`
		} `xml:"testcase"`
	}
	if err := xml.Unmarshal(data, &report); err != nil {
		t.Fatalf("parse audit JUnit report: %v\n%s", err, data)
	}
	if report.Tests != 10 || report.Failures != 0 || len(report.Cases) != 10 {
		t.Errorf("audit JUnit report = tests %d, failures %d, cases %d; want 10, 0, 10", report.Tests, report.Failures, len(report.Cases))
	}
}

func TestTestCommandRejectsUnknownManifestAssertion(t *testing.T) {
	feature := filepath.Join(t.TempDir(), "unknown-environment.feature")
	contents := `Feature: Unknown environment
  Scenario: Assert an environment that was not supplied
    Given the manifests for "qa"
`
	if err := os.WriteFile(feature, []byte(contents), 0o600); err != nil {
		t.Fatalf("write feature: %v", err)
	}

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	command := NewRootCommand(nil, stdout, stderr)
	command.SetArgs([]string{
		"test",
		"--manifests", "prod=../../testdata/manifests/behavior-prod.yaml",
		feature,
	})
	if err := command.Execute(); !errors.Is(err, ErrTestInvalid) {
		t.Fatalf("Execute() error = %v, want invalid behavior input", err)
	}
	if !strings.Contains(stdout.String(), "unknown manifest environment") {
		t.Errorf("stdout missing unknown-environment diagnostic:\n%s", stdout.String())
	}
}

func TestTestCommandRejectsConflictingManifestAssertion(t *testing.T) {
	feature := filepath.Join(t.TempDir(), "conflicting-environment.feature")
	contents := `Feature: Conflicting environment
  @prod
  Scenario: Assert a different environment
    Given the manifests for "staging"
`
	if err := os.WriteFile(feature, []byte(contents), 0o600); err != nil {
		t.Fatalf("write feature: %v", err)
	}

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	command := NewRootCommand(nil, stdout, stderr)
	command.SetArgs([]string{
		"test",
		"--manifests", "prod=../../testdata/manifests/behavior-prod.yaml",
		"--manifests", "staging=../../testdata/manifests/behavior-staging.yaml",
		feature,
	})
	if err := command.Execute(); !errors.Is(err, ErrTestInvalid) {
		t.Fatalf("Execute() error = %v, want invalid behavior input", err)
	}
	if !strings.Contains(stdout.String(), "conflicting manifest environment") {
		t.Errorf("stdout missing conflicting-environment diagnostic:\n%s", stdout.String())
	}
}

func TestTestCommandWritesJUnitReport(t *testing.T) {
	reportPath := filepath.Join(t.TempDir(), "report.xml")
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	command := NewRootCommand(nil, stdout, stderr)
	command.SetArgs([]string{
		"test",
		"-f", "../../testdata/manifests/behavior.yaml",
		"--junit", reportPath,
		"../../testdata/features/routing-passes.feature",
	})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read JUnit report: %v", err)
	}
	var report struct {
		Tests    int `xml:"tests,attr"`
		Failures int `xml:"failures,attr"`
		Cases    []struct {
			Name    string `xml:"name,attr"`
			Failure *struct {
				Message string `xml:"message,attr"`
			} `xml:"failure"`
		} `xml:"testcase"`
	}
	if err := xml.Unmarshal(data, &report); err != nil {
		t.Fatalf("parse JUnit XML: %v\n%s", err, data)
	}
	if report.Tests != 4 || report.Failures != 0 || len(report.Cases) != 4 {
		t.Errorf("JUnit report = tests %d, failures %d, cases %d; want 4, 0, 4", report.Tests, report.Failures, len(report.Cases))
	}
	foundV1, foundV2 := false, false
	for _, testCase := range report.Cases {
		foundV1 = foundV1 || strings.Contains(testCase.Name, "subset v1")
		foundV2 = foundV2 || strings.Contains(testCase.Name, "subset v2")
	}
	if !foundV1 || !foundV2 {
		t.Errorf("JUnit cases do not distinguish outline examples: %#v", report.Cases)
	}
}

func TestTestCommandJUnitIncludesAssertionFailures(t *testing.T) {
	reportPath := filepath.Join(t.TempDir(), "failed.xml")
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	command := NewRootCommand(nil, stdout, stderr)
	command.SetArgs([]string{
		"test",
		"-f", "../../testdata/manifests/behavior.yaml",
		"--junit", reportPath,
		"../../testdata/features/routing-fails.feature",
	})
	if err := command.Execute(); !errors.Is(err, ErrTestFailure) {
		t.Fatalf("Execute() error = %v, want behavior assertion failure", err)
	}
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read JUnit report: %v", err)
	}
	if !bytes.Contains(data, []byte(`failures="1"`)) ||
		!bytes.Contains(data, []byte("expected route 1")) {
		t.Errorf("JUnit report does not include assertion failure:\n%s", data)
	}
}

func TestTestCommandDiscoversNestedFeatureFiles(t *testing.T) {
	featureDir := t.TempDir()
	nestedDir := filepath.Join(featureDir, "nested")
	if err := os.Mkdir(nestedDir, 0o700); err != nil {
		t.Fatalf("create nested feature directory: %v", err)
	}
	feature := filepath.Join(nestedDir, "nested.feature")
	contents := `Feature: Nested feature
  Scenario: Test a rendered route
    When I trace the hostname "shop.example.com"
    Then the hostname should be found
`
	if err := os.WriteFile(feature, []byte(contents), 0o600); err != nil {
		t.Fatalf("write nested feature: %v", err)
	}
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	command := NewRootCommand(nil, stdout, stderr)
	command.SetArgs([]string{"test", "-f", "../../testdata/manifests/behavior.yaml", featureDir})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout.String(), "Feature: Nested feature") {
		t.Errorf("stdout missing nested feature report:\n%s", stdout.String())
	}
}

func TestTestCommandSuggestsClosestUndefinedStep(t *testing.T) {
	feature := filepath.Join(t.TempDir(), "undefined.feature")
	contents := `Feature: Unknown step
  Scenario: Typo
    When I trace the host "shop.example.com"
`
	if err := os.WriteFile(feature, []byte(contents), 0o600); err != nil {
		t.Fatalf("write feature: %v", err)
	}
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	command := NewRootCommand(nil, stdout, stderr)
	command.SetArgs([]string{"test", "-f", "../../testdata/manifests/behavior.yaml", feature})
	if err := command.Execute(); !errors.Is(err, ErrTestInvalid) {
		t.Fatalf("Execute() error = %v, want invalid behavior input", err)
	}
	if !strings.Contains(stderr.String(), `closest supported step:`) ||
		!strings.Contains(stderr.String(), `trace the hostname`) {
		t.Errorf("stderr missing closest-step suggestion:\n%s", stderr.String())
	}
}

func TestTestCommandRejectsMalformedFeatureAndManifestInput(t *testing.T) {
	feature := filepath.Join(t.TempDir(), "malformed.feature")
	if err := os.WriteFile(feature, []byte("not a Gherkin feature"), 0o600); err != nil {
		t.Fatalf("write malformed feature: %v", err)
	}
	manifest := filepath.Join(t.TempDir(), "malformed.yaml")
	if err := os.WriteFile(manifest, []byte("apiVersion: [\n"), 0o600); err != nil {
		t.Fatalf("write malformed manifest: %v", err)
	}
	emptyManifest := filepath.Join(t.TempDir(), "empty.yaml")
	if err := os.WriteFile(emptyManifest, nil, 0o600); err != nil {
		t.Fatalf("write empty manifest: %v", err)
	}
	emptyDirectory := t.TempDir()
	noScenarios := filepath.Join(t.TempDir(), "empty.feature")
	if err := os.WriteFile(noScenarios, []byte("Feature: Empty\n"), 0o600); err != nil {
		t.Fatalf("write feature without scenarios: %v", err)
	}

	for _, args := range [][]string{
		{"test", "-f", "../../testdata/manifests/behavior.yaml", feature},
		{"test", "-f", manifest, "../../testdata/features/routing-passes.feature"},
		{"test", "-f", emptyManifest, "../../testdata/features/routing-passes.feature"},
		{"test", "-f", "../../testdata/manifests/behavior.yaml", emptyDirectory},
		{"test", "-f", "../../testdata/manifests/behavior.yaml", noScenarios},
		{"test", "-f", "../../testdata/manifests/behavior.yaml", filepath.Join(t.TempDir(), "missing.feature")},
		{"test", "-f", "-", "-f", "-", "../../testdata/features/routing-passes.feature"},
	} {
		command := NewRootCommand(nil, new(bytes.Buffer), new(bytes.Buffer))
		command.SetArgs(args)
		if err := command.Execute(); !errors.Is(err, ErrTestInvalid) {
			t.Errorf("Execute(%v) error = %v, want invalid behavior input", args, err)
		}
	}
}

func TestTestCommandInvalidInputStatus(t *testing.T) {
	malformedFeature := filepath.Join(t.TempDir(), "malformed.feature")
	if err := os.WriteFile(malformedFeature, []byte("not a Gherkin feature"), 0o600); err != nil {
		t.Fatalf("write malformed feature: %v", err)
	}
	undefinedFeature := filepath.Join(t.TempDir(), "undefined.feature")
	if err := os.WriteFile(undefinedFeature, []byte(`Feature: Unsupported step
	  Scenario: Invalid vocabulary
	    When the route is really fast
	`), 0o600); err != nil {
		t.Fatalf("write undefined-step feature: %v", err)
	}

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "malformed feature",
			args: []string{"test", "-f", "../../testdata/manifests/behavior.yaml", malformedFeature},
		},
		{
			name: "unknown step",
			args: []string{"test", "-f", "../../testdata/manifests/behavior.yaml", undefinedFeature},
		},
		{
			name: "unreadable manifest",
			args: []string{"test", "-f", filepath.Join(t.TempDir(), "missing.yaml"), "../../testdata/features/routing-passes.feature"},
		},
		{
			name: "unwritable report",
			args: []string{
				"test", "-f", "../../testdata/manifests/behavior.yaml",
				"--junit", filepath.Join(t.TempDir(), "missing", "report.xml"),
				"../../testdata/features/routing-passes.feature",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := NewRootCommand(nil, new(bytes.Buffer), new(bytes.Buffer))
			command.SetArgs(tt.args)
			if err := command.Execute(); !errors.Is(err, ErrTestInvalid) {
				t.Fatalf("Execute() error = %v, want invalid behavior input", err)
			}
		})
	}
}

func TestTestCommandInvalidInputTakesPrecedenceOverAssertionFailure(t *testing.T) {
	undefinedFeature := filepath.Join(t.TempDir(), "undefined.feature")
	if err := os.WriteFile(undefinedFeature, []byte(`Feature: Unsupported step
	  Scenario: Invalid vocabulary
	    When the route is really fast
	`), 0o600); err != nil {
		t.Fatalf("write undefined-step feature: %v", err)
	}
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	command := NewRootCommand(nil, stdout, stderr)
	command.SetArgs([]string{
		"test",
		"-f", "../../testdata/manifests/behavior.yaml",
		"../../testdata/features/routing-fails.feature",
		undefinedFeature,
	})
	if err := command.Execute(); !errors.Is(err, ErrTestInvalid) {
		t.Fatalf("Execute() error = %v, want invalid input to take precedence", err)
	}
	if !strings.Contains(stdout.String(), "FAIL:") ||
		!strings.Contains(stdout.String(), "expected route 1") {
		t.Errorf("stdout missing assertion failure details:\n%s", stdout.String())
	}
}
