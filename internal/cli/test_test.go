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
