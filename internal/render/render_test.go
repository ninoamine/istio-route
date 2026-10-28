package render

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ninoamine/istio-route/internal/model"
	"sigs.k8s.io/yaml"
)

func TestRenderFormatsMixedRoutingGolden(t *testing.T) {
	analysis := mixedRoutingAnalysis()
	tests := []struct {
		name string
		file string
	}{
		{name: "json", file: "mixed-routing.json"},
		{name: "yaml", file: "mixed-routing.yaml"},
		{name: "mermaid", file: "mixed-routing.mmd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(analysis, tt.name, Options{})
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			goldenPath := "../../testdata/golden/" + tt.file
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.WriteFile(goldenPath, []byte(got), 0o600); err != nil {
					t.Fatalf("update %s golden: %v", tt.file, err)
				}
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read %s golden: %v", tt.file, err)
			}
			if got != string(want) {
				t.Fatalf("Render() mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
			}
			if tt.name == "json" {
				var decoded model.Analysis
				if err := json.Unmarshal([]byte(got), &decoded); err != nil {
					t.Fatalf("decode JSON report: %v", err)
				}
				if !reflect.DeepEqual(decoded, analysis) {
					t.Errorf("JSON report changed analysis:\n got %#v\nwant %#v", decoded, analysis)
				}
			}
			if tt.name == "yaml" {
				var decoded model.Analysis
				if err := yaml.Unmarshal([]byte(got), &decoded); err != nil {
					t.Fatalf("decode YAML report: %v", err)
				}
				if !reflect.DeepEqual(decoded, analysis) {
					t.Errorf("YAML report changed analysis:\n got %#v\nwant %#v", decoded, analysis)
				}
			}
		})
	}
}

func TestRenderRejectsUnsupportedFormat(t *testing.T) {
	_, err := Render(model.Analysis{Hostname: "shop.example.com"}, "xml", Options{})
	if err == nil || !strings.Contains(err.Error(), "unsupported output format") {
		t.Fatalf("Render() error = %v, want unsupported format error", err)
	}
}

func mixedRoutingAnalysis() model.Analysis {
	weight := int32(100)
	return model.Analysis{
		Hostname: "shop.example.com",
		Source:   model.SourceSnapshot{Mode: model.SourceModeFiles, Files: []string{"mixed.yaml"}},
		EntryPoints: []model.EntryPoint{
			{
				Origin: model.OriginIstio,
				Resource: model.ResourceIdentity{
					Group: "networking.istio.io", Version: "v1", Kind: "Gateway",
					Namespace: "istio-system", Name: "public-gw",
				},
				Name: "https", Hostname: "shop.example.com", Port: 443, Protocol: "HTTPS",
			},
			{
				Origin: model.OriginGatewayAPI,
				Resource: model.ResourceIdentity{
					Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway",
					Namespace: "infra", Name: "edge-gw",
				},
				Name: "https", Hostname: "shop.example.com", Port: 443, Protocol: "HTTPS",
				Conditions: []model.Condition{{Type: "Programmed", Status: model.ConditionStatusTrue}},
			},
		},
		Routes: []model.Route{
			{
				Origin: model.OriginIstio,
				Resource: model.ResourceIdentity{
					Group: "networking.istio.io", Version: "v1", Kind: "VirtualService",
					Namespace: "shop", Name: "shop-vs",
				},
				ParentRefs: []model.ResourceIdentity{{
					Group: "networking.istio.io", Version: "v1", Kind: "Gateway",
					Namespace: "istio-system", Name: "public-gw",
				}},
				Rules: []model.Rule{{
					Index: 1, Matches: []model.Match{{PathType: "prefix", Path: "/api"}},
					Destinations: []model.Destination{{
						Kind: model.DestinationKindService, Namespace: "shop", Name: "api",
						Port: int32Pointer(8080), Weight: &weight,
					}},
				}},
			},
			{
				Origin: model.OriginGatewayAPI,
				Resource: model.ResourceIdentity{
					Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute",
					Namespace: "shop", Name: "shop-route",
				},
				ParentRefs: []model.ResourceIdentity{{
					Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway",
					Namespace: "infra", Name: "edge-gw",
				}},
				Conditions: []model.Condition{{Type: "Accepted", Status: model.ConditionStatusTrue}},
				Rules: []model.Rule{{
					Index:   1,
					Filters: []model.Filter{{Type: "URLRewrite", Details: map[string]string{"path": "/v2"}}},
					Destinations: []model.Destination{{
						Kind: model.DestinationKindService, Namespace: "shop", Name: "api-v2",
						Port: int32Pointer(8080), Weight: &weight,
					}},
				}},
			},
		},
		Findings: []model.Finding{{
			Code: "mixed-api-hostname", Severity: model.FindingSeverityWarning,
			Message: "hostname is served by both routing APIs",
		}},
	}
}
