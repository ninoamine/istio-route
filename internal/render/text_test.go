package render

import (
	"os"
	"strings"
	"testing"

	"github.com/ninoamine/istio-route/internal/model"
)

func TestTextMixedRoutingGolden(t *testing.T) {
	weight90, weight10, weight100 := int32(90), int32(10), int32(100)
	analysis := model.Analysis{
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
				TLS: &model.TLSConfig{
					Mode: "SIMPLE",
					CertificateRefs: []model.ResourceIdentity{{
						Version: "v1", Kind: "Secret", Namespace: "istio-system", Name: "shop-tls",
					}},
				},
			},
			{
				Origin: model.OriginGatewayAPI,
				Resource: model.ResourceIdentity{
					Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway",
					Namespace: "infra", Name: "edge-gw",
				},
				Name: "https", Hostname: "shop.example.com", Port: 443, Protocol: "HTTPS",
				GatewayClass: &model.GatewayClassRef{
					Identity: model.ResourceIdentity{
						Group: "gateway.networking.k8s.io", Version: "v1", Kind: "GatewayClass", Name: "istio",
					},
					Controller: "istio.io/gateway-controller",
				},
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
				Rules: []model.Rule{
					{
						Index:   1,
						Matches: []model.Match{{PathType: "prefix", Path: "/api"}},
						Destinations: []model.Destination{
							{Kind: model.DestinationKindService, Namespace: "shop", Name: "api-svc", Subset: "v1", Weight: &weight90, Port: int32Pointer(8080)},
							{Kind: model.DestinationKindService, Namespace: "shop", Name: "api-svc", Subset: "v2", Weight: &weight10, Port: int32Pointer(8080)},
						},
					},
					{
						Index:   2,
						Matches: []model.Match{{PathType: "default"}},
						Destinations: []model.Destination{{
							Kind: model.DestinationKindService, Namespace: "shop", Name: "web-svc",
							Weight: &weight100, Port: int32Pointer(80),
						}},
					},
				},
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
				Conditions: []model.Condition{
					{Type: "Accepted", Status: model.ConditionStatusTrue},
					{Type: "ResolvedRefs", Status: model.ConditionStatusTrue},
				},
				Rules: []model.Rule{
					{
						Index: 1,
						Matches: []model.Match{{
							PathType: "PathPrefix", Path: "/v2", Headers: map[string]string{"x-canary": "true"},
						}},
						Filters: []model.Filter{{Type: "URLRewrite", Details: map[string]string{"path": "/v2 → /"}}},
						Destinations: []model.Destination{{
							Kind: model.DestinationKindService, Namespace: "shop", Name: "api-v2",
							Weight: &weight100, Port: int32Pointer(8080),
						}},
					},
					{
						Index:   2,
						Matches: []model.Match{{PathType: "default"}},
						Destinations: []model.Destination{{
							Kind: model.DestinationKindService, Namespace: "shop", Name: "web",
							Weight: &weight100, Port: int32Pointer(80),
						}},
					},
				},
			},
		},
		Findings: []model.Finding{{
			Code: "mixed-api-hostname", Severity: model.FindingSeverityWarning,
			Message: "shop.example.com is served by both an Istio VirtualService and a Gateway API HTTPRoute",
		}},
	}

	got := Text(analysis)
	want, err := os.ReadFile("../../testdata/golden/mixed-routing.txt")
	if err != nil {
		t.Fatalf("read text golden: %v", err)
	}
	if got != string(want) {
		t.Fatalf("Text() mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestTextRendersWorkloadAndConditionStates(t *testing.T) {
	weight := int32(50)
	ready, notReady := true, false
	analysis := model.Analysis{
		Hostname: "shop.example.com",
		EntryPoints: []model.EntryPoint{{
			Origin: model.OriginGatewayAPI,
			Resource: model.ResourceIdentity{
				Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway",
				Namespace: "infra", Name: "edge",
			},
			Name: "https", Port: 443, Protocol: "HTTPS",
			TLS: &model.TLSConfig{Mode: "Terminate"},
			Conditions: []model.Condition{
				{Type: "Programmed", Status: model.ConditionStatusFalse},
				{Type: "Programmed", Status: model.ConditionStatusUnknown},
			},
		}},
		Routes: []model.Route{{
			Origin: model.OriginGatewayAPI,
			Resource: model.ResourceIdentity{
				Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute",
				Namespace: "shop", Name: "route",
			},
			ParentRefs: []model.ResourceIdentity{{
				Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway",
				Namespace: "infra", Name: "edge",
			}},
			Conditions: []model.Condition{
				{Type: "Accepted", Status: model.ConditionStatusFalse},
				{Type: "ResolvedRefs", Status: model.ConditionStatusUnknown},
			},
			Rules: []model.Rule{{
				Index: 1,
				Matches: []model.Match{{
					PathType: "PathPrefix", Path: "/api", Headers: map[string]string{"x-canary": "true"},
					QueryParams: map[string]string{"region": "west"}, Method: "GET",
				}},
				Filters: []model.Filter{{Type: "Headers"}, {Type: "Rewrite", Details: map[string]string{"path": "/"}}},
				Destinations: []model.Destination{
					{
						Kind: model.DestinationKindExternal, Name: "outside.example",
						Weight: &weight,
						External: &model.ExternalDestination{
							Hosts: []string{"outside.example"}, Location: "MESH_EXTERNAL", Resolution: "DNS",
						},
					},
					{
						Kind: model.DestinationKindService, Namespace: "shop", Name: "api", Subset: "v2",
						Weight:         &weight,
						ReferenceGrant: &model.GrantResult{Allowed: false, Reason: "not authorized"},
						Service: &model.Service{
							Identity:     model.ResourceIdentity{Version: "v1", Kind: "Service", Namespace: "shop", Name: "api"},
							Availability: model.AvailabilityPresent,
							Endpoints: []model.Endpoint{
								{Addresses: []string{"10.0.0.1"}, Ready: &ready},
								{
									Addresses: []string{"10.0.0.2"}, Ready: &notReady,
									Pod: &model.Pod{
										Identity: model.ResourceIdentity{Version: "v1", Kind: "Pod", Namespace: "shop", Name: "api-2"},
										Ready:    &notReady, Phase: "Running", Availability: model.AvailabilityPresent,
									},
								},
							},
						},
					},
					{
						Kind: model.DestinationKindService, Name: "missing",
						Service: &model.Service{
							Identity:     model.ResourceIdentity{Version: "v1", Kind: "Service", Name: "missing"},
							Availability: model.AvailabilityNotFound,
						},
					},
				},
			}},
		}},
		Findings: []model.Finding{
			{Code: "warning", Severity: model.FindingSeverityWarning, Message: "warn at the end"},
			{Code: "info", Severity: model.FindingSeverityInfo, Message: "not rendered"},
		},
	}

	got := Text(analysis)
	for _, want := range []string{
		"✘ Programmed", "? Programmed", "✘ Accepted", "? ResolvedRefs",
		"path prefix /api", "header x-canary=true", "query region=west", "method GET",
		"filter: Headers", "filter: Rewrite path=/",
		"outside.example (50%)", "external destination: hosts=outside.example",
		"api.shop subset v2 (50%)", "ReferenceGrant: not allowed", "Service shop/api (present)",
		"Endpoint 10.0.0.1 (ready)", "Endpoint 10.0.0.2 (not ready)",
		"Pod shop/api-2 (not ready, phase Running)", "Service missing (not-found)",
		"⚠ warn at the end",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Text() output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "not rendered") {
		t.Errorf("Text() rendered non-warning finding: %s", got)
	}
}

func int32Pointer(value int32) *int32 {
	return &value
}
