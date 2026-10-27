package resolve

import (
	"testing"

	"google.golang.org/protobuf/types/known/durationpb"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
)

func TestIstioDestinationAndStringHelpers(t *testing.T) {
	tests := []struct {
		name        string
		namespace   string
		destination string
		ruleNS      string
		ruleHost    string
		wantMatch   bool
		wantNS      string
		wantName    string
	}{
		{name: "short service", namespace: "shop", destination: "api", ruleNS: "shop", ruleHost: "api", wantMatch: true, wantNS: "shop", wantName: "api"},
		{name: "service namespace", namespace: "shop", destination: "api.other.svc.cluster.local", ruleNS: "other", ruleHost: "api", wantMatch: true, wantNS: "other", wantName: "api"},
		{name: "service cluster domain", namespace: "shop", destination: "api.other.svc.cluster.local", ruleNS: "other", ruleHost: "api.other.svc.cluster.local", wantMatch: true, wantNS: "other", wantName: "api"},
		{name: "different short service", namespace: "shop", destination: "web", ruleNS: "shop", ruleHost: "api", wantMatch: false, wantNS: "shop", wantName: "web"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNamespace, gotName := serviceReference(tt.namespace, tt.destination)
			if gotNamespace != tt.wantNS || gotName != tt.wantName {
				t.Errorf("serviceReference() = %q/%q, want %q/%q", gotNamespace, gotName, tt.wantNS, tt.wantName)
			}
			if got := istioRuleHostMatches(tt.destination, tt.ruleNS, tt.ruleHost); got != tt.wantMatch {
				t.Errorf("istioRuleHostMatches() = %t, want %t", got, tt.wantMatch)
			}
		})
	}
	if got := firstHost(nil); got != "" {
		t.Errorf("firstHost(nil) = %q, want empty", got)
	}
	if got := firstHost([]string{"api.example.com", "web.example.com"}); got != "api.example.com" {
		t.Errorf("firstHost() = %q, want first hostname", got)
	}

	hosts := matchingServiceEntryHosts("api.external.example", []string{"*.external.example", "other.example"})
	if len(hosts) != 1 || hosts[0] != "*.external.example" {
		t.Errorf("matchingServiceEntryHosts() = %v, want matching wildcard only", hosts)
	}
}

func TestIstioFiltersPreserveConfiguredActions(t *testing.T) {
	route := &istiov1alpha3.HTTPRoute{
		Rewrite: &istiov1alpha3.HTTPRewrite{Uri: "/", Authority: "api.example.com"},
		Redirect: &istiov1alpha3.HTTPRedirect{
			Uri: "/new", Authority: "shop.example.com", RedirectCode: 302,
		},
		Timeout: durationpb.New(2_000_000_000),
		Retries: &istiov1alpha3.HTTPRetry{
			Attempts: 3, RetryOn: "5xx", PerTryTimeout: durationpb.New(1_000_000_000),
		},
		Mirror: &istiov1alpha3.Destination{Host: "mirror.example.com", Subset: "canary"},
	}
	filters := istioFilters(route)
	if len(filters) != 5 {
		t.Fatalf("istioFilters() returned %d filters, want 5: %#v", len(filters), filters)
	}
	wantTypes := []string{"Rewrite", "Redirect", "Timeout", "Retries", "Mirror"}
	for i, want := range wantTypes {
		if filters[i].Type != want {
			t.Errorf("filter %d type = %q, want %q", i, filters[i].Type, want)
		}
	}
	if filters[1].Details["code"] != "302" || filters[3].Details["attempts"] != "3" {
		t.Errorf("redirect/retry details = %#v / %#v", filters[1].Details, filters[3].Details)
	}
}
