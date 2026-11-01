package behavior

import (
	"testing"

	"github.com/ninoamine/istio-route/internal/model"
)

func TestLookupField(t *testing.T) {
	root := map[string]any{
		"hostname": "shop.example.com",
		"routes": []any{
			map[string]any{
				"resource": map[string]any{"kind": "VirtualService"},
				"rules": []any{
					map[string]any{"matches": []any{map[string]any{"path": "/api"}}},
				},
			},
		},
		"available": true,
	}

	tests := []struct {
		name       string
		path       string
		wantValue  string
		wantExists bool
		wantErr    bool
	}{
		{name: "scalar", path: "hostname", wantValue: "shop.example.com", wantExists: true},
		{name: "object", path: "routes[0].resource.kind", wantValue: "VirtualService", wantExists: true},
		{name: "nested indexed list", path: "routes[0].rules[0].matches[0].path", wantValue: "/api", wantExists: true},
		{name: "boolean", path: "available", wantValue: "true", wantExists: true},
		{name: "missing field", path: "routes[0].resource.name", wantValue: "<absent>"},
		{name: "out of range", path: "routes[1].resource.kind", wantValue: "<absent>"},
		{name: "invalid index", path: "routes[no].resource.kind", wantErr: true},
		{name: "invalid field name", path: "route name", wantErr: true},
		{name: "empty path", path: ".", wantErr: true},
		{name: "trailing separator", path: "hostname.", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, exists, err := lookupField(root, tt.path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("lookupField() error = %v, wantErr %t", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if got != tt.wantValue || exists != tt.wantExists {
				t.Errorf("lookupField() = (%q, %t), want (%q, %t)", got, exists, tt.wantValue, tt.wantExists)
			}
		})
	}
}

func TestLookupAnalysisField(t *testing.T) {
	analysis := model.Analysis{
		Hostname: "shop.example.com",
		Routes: []model.Route{{
			Resource: model.ResourceIdentity{Kind: "VirtualService", Name: "shop"},
		}},
	}
	got, exists, err := lookupAnalysisField(analysis, "routes[0].resource.kind")
	if err != nil {
		t.Fatalf("lookupAnalysisField() error = %v", err)
	}
	if !exists || got != "VirtualService" {
		t.Errorf("lookupAnalysisField() = (%q, %t), want (VirtualService, true)", got, exists)
	}
}
