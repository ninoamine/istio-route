package audit

import (
	"testing"

	"github.com/ninoamine/istio-route/internal/model"
)

func TestDuplicateFindings(t *testing.T) {
	tests := []struct {
		name        string
		first       Route
		second      Route
		wantFinding bool
	}{
		{
			name: "same listener and normalized hostname",
			first: duplicateTestRoute("HTTPRoute", "first", []string{"HTTPS"}, []string{"API.Example.com."},
				Match{Path: &PathMatch{Type: "PathPrefix", Value: "/v1"}}),
			second: duplicateTestRoute("HTTPRoute", "second", []string{"HTTPS"}, []string{"api.example.com"},
				Match{Path: &PathMatch{Type: "PathPrefix", Value: "/v1"}}),
			wantFinding: true,
		},
		{
			name:  "default path equivalent to explicit prefix root",
			first: duplicateTestRoute("HTTPRoute", "first", []string{"https"}, []string{"api.example.com"}, Match{}),
			second: duplicateTestRoute("HTTPRoute", "second", []string{"https"}, []string{"api.example.com"},
				Match{Path: &PathMatch{Type: "PathPrefix", Value: "/"}}),
			wantFinding: true,
		},
		{
			name: "different paths are not duplicates",
			first: duplicateTestRoute("HTTPRoute", "first", []string{"https"}, []string{"api.example.com"},
				Match{Path: &PathMatch{Type: "Exact", Value: "/first"}}),
			second: duplicateTestRoute("HTTPRoute", "second", []string{"https"}, []string{"api.example.com"},
				Match{Path: &PathMatch{Type: "Exact", Value: "/second"}}),
		},
		{
			name: "header order and names are equivalent",
			first: duplicateTestRoute("HTTPRoute", "first", []string{"https"}, []string{"api.example.com"}, Match{
				Headers: []StringMatch{{Name: "X-Version", Type: "Exact", Value: "v1"}, {Name: "X-Mode", Type: "Exact", Value: "fast"}},
			}),
			second: duplicateTestRoute("HTTPRoute", "second", []string{"https"}, []string{"api.example.com"}, Match{
				Headers: []StringMatch{{Name: "x-mode", Value: "fast"}, {Name: "x-version", Value: "v1"}},
			}),
			wantFinding: true,
		},
		{
			name: "method is part of identity",
			first: duplicateTestRoute("HTTPRoute", "first", []string{"https"}, []string{"api.example.com"},
				Match{Method: "GET"}),
			second: duplicateTestRoute("HTTPRoute", "second", []string{"https"}, []string{"api.example.com"},
				Match{Method: "POST"}),
		},
		{
			name: "query parameter values are part of identity",
			first: duplicateTestRoute("HTTPRoute", "first", []string{"https"}, []string{"api.example.com"},
				Match{QueryParams: []StringMatch{{Name: "version", Value: "v1"}}}),
			second: duplicateTestRoute("HTTPRoute", "second", []string{"https"}, []string{"api.example.com"},
				Match{QueryParams: []StringMatch{{Name: "version", Value: "v2"}}}),
		},
		{
			name: "different header values are not duplicates",
			first: duplicateTestRoute("HTTPRoute", "first", []string{"https"}, []string{"api.example.com"},
				Match{Headers: []StringMatch{{Name: "X-Version", Value: "v1"}}}),
			second: duplicateTestRoute("HTTPRoute", "second", []string{"https"}, []string{"api.example.com"},
				Match{Headers: []StringMatch{{Name: "X-Version", Value: "v2"}}}),
		},
		{
			name: "query parameter names remain case-sensitive",
			first: duplicateTestRoute("HTTPRoute", "first", []string{"https"}, []string{"api.example.com"},
				Match{QueryParams: []StringMatch{{Name: "Version", Value: "v1"}}}),
			second: duplicateTestRoute("HTTPRoute", "second", []string{"https"}, []string{"api.example.com"},
				Match{QueryParams: []StringMatch{{Name: "version", Value: "v1"}}}),
		},
		{
			name:   "different listeners are not duplicates",
			first:  duplicateTestRoute("HTTPRoute", "first", []string{"https"}, []string{"api.example.com"}, Match{}),
			second: duplicateTestRoute("HTTPRoute", "second", []string{"http"}, []string{"api.example.com"}, Match{}),
		},
		{
			name:   "different hostnames are not duplicates",
			first:  duplicateTestRoute("HTTPRoute", "first", []string{"https"}, []string{"a.example.com"}, Match{}),
			second: duplicateTestRoute("HTTPRoute", "second", []string{"https"}, []string{"b.example.com"}, Match{}),
		},
		{
			name: "GRPC service and method are exact match fields",
			first: duplicateTestRoute("GRPCRoute", "first", []string{"grpc"}, []string{"api.example.com"},
				Match{GRPC: &GRPCMatch{Service: "store.v1.Store", Method: "Get"}}),
			second: duplicateTestRoute("GRPCRoute", "second", []string{"grpc"}, []string{"api.example.com"},
				Match{GRPC: &GRPCMatch{Service: "store.v1.Store", Method: "Get"}}),
			wantFinding: true,
		},
		{
			name: "different GRPC methods are not duplicates",
			first: duplicateTestRoute("GRPCRoute", "first", []string{"grpc"}, []string{"api.example.com"},
				Match{GRPC: &GRPCMatch{Service: "store.v1.Store", Method: "Get"}}),
			second: duplicateTestRoute("GRPCRoute", "second", []string{"grpc"}, []string{"api.example.com"},
				Match{GRPC: &GRPCMatch{Service: "store.v1.Store", Method: "Put"}}),
		},
		{
			name: "different GRPC services are not duplicates",
			first: duplicateTestRoute("GRPCRoute", "first", []string{"grpc"}, []string{"api.example.com"},
				Match{GRPC: &GRPCMatch{Service: "store.v1.Store", Method: "Get"}}),
			second: duplicateTestRoute("GRPCRoute", "second", []string{"grpc"}, []string{"api.example.com"},
				Match{GRPC: &GRPCMatch{Service: "store.v2.Store", Method: "Get"}}),
		},
		{
			name:   "HTTP and GRPC kinds are not duplicates",
			first:  duplicateTestRoute("HTTPRoute", "first", []string{"https"}, []string{"api.example.com"}, Match{}),
			second: duplicateTestRoute("GRPCRoute", "second", []string{"https"}, []string{"api.example.com"}, Match{}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := duplicateFindings(Result{Routes: []Route{tt.first, tt.second}})
			if (len(findings) > 0) != tt.wantFinding {
				t.Fatalf("duplicateFindings() returned %#v, want finding=%t", findings, tt.wantFinding)
			}
			if !tt.wantFinding {
				return
			}
			if len(findings) != 1 {
				t.Fatalf("duplicateFindings() returned %d findings, want 1", len(findings))
			}
			finding := findings[0]
			if finding.Code != "duplicate-route-rule" ||
				finding.Severity != model.FindingSeverityWarning ||
				finding.Category != string(CategoryDuplicated) ||
				len(finding.Resources) != 2 || finding.Message == "" || finding.SuggestedFix == "" {
				t.Errorf("duplicate finding lacks required metadata: %#v", finding)
			}
		})
	}
}

func duplicateTestRoute(kind, name string, listeners, hostnames []string, match Match) Route {
	return Route{
		Identity:  testIdentity(kind, "shop", name),
		Hostnames: hostnames,
		Rules:     []Rule{{Index: 1, ListenerNames: listeners, Matches: []Match{match}, Precedence: PrecedenceWinner}},
	}
}
