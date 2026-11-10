package render

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ninoamine/istio-route/internal/simulate"
	"github.com/ninoamine/istio-route/internal/source"
	"sigs.k8s.io/yaml"
)

func simulateImpactResult(t *testing.T) simulate.Result {
	t.Helper()
	snapshot, err := (&source.FilesSource{
		Files: []string{"../../testdata/manifests/simulate-impact.yaml"},
	}).Snapshot(context.Background())
	if err != nil {
		t.Fatalf("load simulate fixture: %v", err)
	}
	route := simulate.RouteRef{Namespace: "shop", Name: "primary"}
	result, _, err := simulate.Run(context.Background(), simulate.Request{
		WaypointName: "waypoint", Namespace: "mesh", Route: &route,
	}, snapshot)
	if err != nil {
		t.Fatalf("run simulation: %v", err)
	}
	return result
}

func TestSimulateTextGolden(t *testing.T) {
	got, err := RenderSimulation(simulateImpactResult(t), FormatText, Options{})
	if err != nil {
		t.Fatalf("RenderSimulation() error = %v", err)
	}
	path := "../../testdata/golden/simulate-impact.txt"
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatalf("update simulate-impact.txt: %v", err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read simulate-impact.txt: %v", err)
	}
	if got != string(want) {
		t.Fatalf("simulate output differs from simulate-impact.txt:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestSimulateFormatGoldens(t *testing.T) {
	result := simulateImpactResult(t)
	tests := []struct {
		format string
		file   string
	}{
		{format: FormatJSON, file: "simulate-impact.json"},
		{format: FormatYAML, file: "simulate-impact.yaml"},
		{format: FormatMermaid, file: "simulate-impact.mmd"},
	}
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			got, err := RenderSimulation(result, tt.format, Options{})
			if err != nil {
				t.Fatalf("RenderSimulation() error = %v", err)
			}
			path := "../../testdata/golden/" + tt.file
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
					t.Fatalf("update %s: %v", tt.file, err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", tt.file, err)
			}
			if got != string(want) {
				t.Fatalf("%s output differs from %s:\n--- got ---\n%s\n--- want ---\n%s", tt.format, tt.file, got, want)
			}
		})
	}
}

func TestSimulateFormatsAgreeOnEntries(t *testing.T) {
	result := simulateImpactResult(t)
	jsonOut, err := RenderSimulation(result, FormatJSON, Options{})
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	var decoded simulate.Result
	if err := json.Unmarshal([]byte(jsonOut), &decoded); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if !reflect.DeepEqual(decoded.Entries, result.Entries) {
		t.Fatalf("JSON entries differ from simulation entries")
	}
	if decoded.Totals != result.Totals {
		t.Fatalf("JSON totals = %+v, want %+v", decoded.Totals, result.Totals)
	}

	yamlOut, err := RenderSimulation(result, FormatYAML, Options{})
	if err != nil {
		t.Fatalf("yaml: %v", err)
	}
	var decodedYAML simulate.Result
	if err := yaml.Unmarshal([]byte(yamlOut), &decodedYAML); err != nil {
		t.Fatalf("decode yaml: %v", err)
	}
	if !reflect.DeepEqual(decodedYAML.Entries, result.Entries) {
		t.Fatalf("YAML entries differ from simulation entries")
	}
	if decodedYAML.Totals != result.Totals {
		t.Fatalf("YAML totals = %+v, want %+v", decodedYAML.Totals, result.Totals)
	}

	text, err := RenderSimulation(result, FormatText, Options{})
	if err != nil {
		t.Fatalf("text: %v", err)
	}
	mermaid, err := RenderSimulation(result, FormatMermaid, Options{})
	if err != nil {
		t.Fatalf("mermaid: %v", err)
	}
	textEntries := strings.Count(text, "\n  - listener ")
	mermaidEntries := 0
	for _, line := range strings.Split(mermaid, "\n") {
		if strings.HasPrefix(line, "  entry_") && strings.Contains(line, `["`) {
			mermaidEntries++
		}
	}
	if textEntries != len(result.Entries) || mermaidEntries != len(result.Entries) {
		t.Fatalf("entry counts disagree: entries=%d text=%d mermaid=%d",
			len(result.Entries), textEntries, mermaidEntries)
	}
	for _, entry := range result.Entries {
		if !strings.Contains(mermaid, string(entry.Outcome)+": listener "+entry.Listener) {
			t.Errorf("mermaid missing entry %s on listener %s", entry.Outcome, entry.Listener)
		}
	}
}

func simulateListingResult(t *testing.T) simulate.Result {
	t.Helper()
	snapshot, err := (&source.FilesSource{
		Files: []string{"../../testdata/manifests/simulate-listing.yaml"},
	}).Snapshot(context.Background())
	if err != nil {
		t.Fatalf("load listing fixture: %v", err)
	}
	result, _, err := simulate.Run(context.Background(), simulate.Request{
		WaypointName: "waypoint", Namespace: "mesh",
	}, snapshot)
	if err != nil {
		t.Fatalf("run listing: %v", err)
	}
	return result
}

func TestSimulateListingGolden(t *testing.T) {
	got, err := RenderSimulation(simulateListingResult(t), FormatText, Options{})
	if err != nil {
		t.Fatalf("RenderSimulation() error = %v", err)
	}
	path := "../../testdata/golden/simulate-listing.txt"
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatalf("update simulate-listing.txt: %v", err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read simulate-listing.txt: %v", err)
	}
	if got != string(want) {
		t.Fatalf("listing output differs from simulate-listing.txt:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestSimulateTextNoColorHasNoANSIEscapes(t *testing.T) {
	got, err := RenderSimulation(simulateImpactResult(t), FormatText, Options{Color: false})
	if err != nil {
		t.Fatalf("RenderSimulation() error = %v", err)
	}
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("no-color output contains ANSI escapes:\n%s", got)
	}
}

func TestRenderSimulationDispatch(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		wantErr string
	}{
		{name: "unknown format", format: "csv", wantErr: `unsupported output format "csv"`},
		{name: "empty format", format: "", wantErr: `unsupported output format ""`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := RenderSimulation(simulate.Result{}, tt.format, Options{})
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("RenderSimulation(%q) error = %v, want %q", tt.format, err, tt.wantErr)
			}
		})
	}
}
