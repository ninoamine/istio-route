package render

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ninoamine/istio-route/internal/audit"
	"github.com/ninoamine/istio-route/internal/source"
	"sigs.k8s.io/yaml"
)

func TestAuditRenderFormatsGolden(t *testing.T) {
	snapshot, err := (&source.FilesSource{
		Files: []string{"../../testdata/manifests/gateway-audit-healthy.yaml"},
	}).Snapshot(context.Background())
	if err != nil {
		t.Fatalf("load healthy Gateway fixture: %v", err)
	}
	result, _, err := audit.Analyze(context.Background(), audit.Request{
		GatewayName: "edge", Namespace: "infra", Source: snapshot.Metadata,
	}, snapshot)
	if err != nil {
		t.Fatalf("analyze healthy Gateway fixture: %v", err)
	}
	tests := []struct {
		format string
		file   string
	}{
		{format: FormatText, file: "gateway-audit-healthy.txt"},
		{format: FormatJSON, file: "gateway-audit-healthy.json"},
		{format: FormatYAML, file: "gateway-audit-healthy.yaml"},
		{format: FormatMermaid, file: "gateway-audit-healthy.mmd"},
	}
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			got, err := RenderAudit(result, tt.format, Options{})
			if err != nil {
				t.Fatalf("RenderAudit() error = %v", err)
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
				t.Fatalf("audit output differs from %s:\n--- got ---\n%s\n--- want ---\n%s", tt.file, got, want)
			}
		})
	}
}

type auditSummary struct {
	rules    map[string]string
	findings []string
}

func TestAuditRenderFormatsAgreeOnPrecedence(t *testing.T) {
	snapshot, err := (&source.FilesSource{
		Files: []string{"../../testdata/manifests/gateway-audit-precedence-exact-vs-prefix.yaml"},
	}).Snapshot(context.Background())
	if err != nil {
		t.Fatalf("load precedence Gateway fixture: %v", err)
	}
	result, _, err := audit.Analyze(context.Background(), audit.Request{
		GatewayName: "edge", Namespace: "infra", Source: snapshot.Metadata,
	}, snapshot)
	if err != nil {
		t.Fatalf("analyze precedence Gateway fixture: %v", err)
	}
	want := summarizeAudit(result)
	if len(want.rules) == 0 || len(want.findings) == 0 {
		t.Fatalf("precedence fixture must have rules and findings, got %d rules and %d findings", len(want.rules), len(want.findings))
	}

	tests := []struct {
		format string
		file   string
	}{
		{format: FormatText, file: "gateway-audit-precedence.txt"},
		{format: FormatJSON, file: "gateway-audit-precedence.json"},
		{format: FormatYAML, file: "gateway-audit-precedence.yaml"},
		{format: FormatMermaid, file: "gateway-audit-precedence.mmd"},
	}
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			got, err := RenderAudit(result, tt.format, Options{})
			if err != nil {
				t.Fatalf("RenderAudit() error = %v", err)
			}
			assertGolden(t, got, tt.file)

			switch tt.format {
			case FormatText:
				assertSummary(t, want, summarizeText(t, got))
			case FormatJSON:
				var decoded audit.Result
				if err := json.Unmarshal([]byte(got), &decoded); err != nil {
					t.Fatalf("decode JSON audit: %v", err)
				}
				assertSummary(t, want, summarizeAudit(decoded))
			case FormatYAML:
				var decoded audit.Result
				if err := yaml.Unmarshal([]byte(got), &decoded); err != nil {
					t.Fatalf("decode YAML audit: %v", err)
				}
				assertSummary(t, want, summarizeAudit(decoded))
			case FormatMermaid:
				assertMermaidPrecedence(t, result, got)
			}
		})
	}
}

// summarizeAudit keys each rule by route identity and rule index so every
// format can be compared against the same logical view of the audit.
func summarizeAudit(result audit.Result) auditSummary {
	summary := auditSummary{rules: map[string]string{}}
	for _, route := range result.Routes {
		for _, rule := range route.Rules {
			key := fmt.Sprintf("%s %s/%s rule %d", route.Identity.Kind, route.Identity.Namespace, route.Identity.Name, rule.Index)
			summary.rules[key] = string(rule.Precedence)
		}
	}
	for _, finding := range result.Findings {
		summary.findings = append(summary.findings, fmt.Sprintf("%s %s: %s", finding.Severity, finding.Code, finding.Message))
	}
	return summary
}

func summarizeText(t *testing.T, text string) auditSummary {
	t.Helper()
	summary := auditSummary{rules: map[string]string{}}
	var route string
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "  HTTPRoute ") || strings.HasPrefix(line, "  GRPCRoute "):
			route = strings.TrimSpace(line)
		case strings.HasPrefix(line, "    rule "):
			index, state, ok := strings.Cut(strings.TrimPrefix(line, "    rule "), ": ")
			if !ok {
				t.Fatalf("malformed rule line %q", line)
			}
			summary.rules[route+" rule "+index] = state
		case hasSeverityPrefix(line):
			summary.findings = append(summary.findings, line)
		}
	}
	return summary
}

func hasSeverityPrefix(line string) bool {
	for _, severity := range []string{"error ", "warning ", "info "} {
		if strings.HasPrefix(line, severity) {
			return true
		}
	}
	return false
}

func assertSummary(t *testing.T, want, got auditSummary) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("format summary differs:\n got: %#v\nwant: %#v", got, want)
	}
}

// assertMermaidPrecedence checks the Mermaid view, which draws precedence only
// on edges to Service backends. Every rule in the fixture has one, so each rule
// state and every finding must appear.
func assertMermaidPrecedence(t *testing.T, result audit.Result, got string) {
	t.Helper()
	for _, route := range result.Routes {
		routeID := mermaidID(route.Identity.Kind, route.Identity.Namespace+"/"+route.Identity.Name)
		for _, rule := range route.Rules {
			if len(rule.Backends) == 0 {
				t.Fatalf("Mermaid cannot show rule %d of %s/%s: it has no backends", rule.Index, route.Identity.Namespace, route.Identity.Name)
			}
			for _, backend := range rule.Backends {
				serviceID := mermaidID("service", backend.Reference.Namespace+"/"+backend.Reference.Name)
				edge := fmt.Sprintf("  %s -->|%s| %s", routeID, rule.Precedence, serviceID)
				if !strings.Contains(got, edge) {
					t.Fatalf("Mermaid output missing edge %q", edge)
				}
			}
		}
	}
	for _, finding := range result.Findings {
		label := fmt.Sprintf("%s %s: %s", escapeMermaid(string(finding.Severity)), escapeMermaid(finding.Code), escapeMermaid(finding.Message))
		if !strings.Contains(got, `["`+label+`"]`) {
			t.Fatalf("Mermaid output missing finding %q", label)
		}
	}
}

func assertGolden(t *testing.T, got, file string) {
	t.Helper()
	path := "../../testdata/golden/" + file
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatalf("update %s: %v", file, err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	if got != string(want) {
		t.Fatalf("audit output differs from %s:\n--- got ---\n%s\n--- want ---\n%s", file, got, want)
	}
}
