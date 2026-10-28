package diagnose

import (
	"testing"

	"github.com/ninoamine/istio-route/internal/model"
	"github.com/ninoamine/istio-route/internal/source"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	istioclientv1 "istio.io/client-go/pkg/apis/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDiagnoseWarnings(t *testing.T) {
	weight := func(value int32) *int32 { return &value }
	tests := []struct {
		name      string
		analysis  model.Analysis
		resources []source.Resource
		wantCode  string
	}{
		{
			name:     "unbound VirtualService",
			analysis: model.Analysis{Hostname: "shop.example.com"},
			resources: []source.Resource{testResource(
				&istioclientv1.VirtualService{
					ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "route"},
					Spec: istiov1alpha3.VirtualService{
						Hosts:    []string{"shop.example.com"},
						Gateways: []string{"infra/missing"},
					},
				},
				"networking.istio.io", "v1", "VirtualService", "shop", "route",
			)},
			wantCode: "unbound-virtualservice",
		},
		{
			name: "subset without DestinationRule",
			analysis: model.Analysis{
				Hostname: "shop.example.com",
				Routes: []model.Route{{
					Origin: model.OriginIstio,
					Rules: []model.Rule{{Index: 1, Destinations: []model.Destination{{
						Kind: model.DestinationKindService, Namespace: "shop", Name: "api", Subset: "v2",
					}}}},
				}},
			},
			wantCode: "missing-destination-rule-subset",
		},
		{
			name: "missing ReferenceGrant",
			analysis: model.Analysis{
				Hostname: "shop.example.com",
				Routes: []model.Route{{
					Origin: model.OriginGatewayAPI,
					Rules: []model.Rule{{Index: 1, Destinations: []model.Destination{{
						Kind: model.DestinationKindService, Namespace: "backend", Name: "api",
						ReferenceGrant: &model.GrantResult{Allowed: false, Reason: "no ReferenceGrant"},
					}}}},
				}},
			},
			wantCode: "missing-reference-grant",
		},
		{
			name: "invalid Istio weight total",
			analysis: model.Analysis{
				Hostname: "shop.example.com",
				Routes: []model.Route{{
					Origin: model.OriginIstio,
					Rules: []model.Rule{{Index: 1, Destinations: []model.Destination{
						{Kind: model.DestinationKindService, Namespace: "shop", Name: "api", Weight: weight(40)},
						{Kind: model.DestinationKindService, Namespace: "shop", Name: "web", Weight: weight(30)},
					}}},
				}},
			},
			wantCode: "invalid-istio-weights",
		},
		{
			name: "all Gateway API weights are zero",
			analysis: model.Analysis{
				Hostname: "shop.example.com",
				Routes: []model.Route{{
					Origin: model.OriginGatewayAPI,
					Rules: []model.Rule{{Index: 1, Destinations: []model.Destination{
						{Kind: model.DestinationKindService, Namespace: "shop", Name: "api", Weight: weight(0)},
					}}},
				}},
			},
			wantCode: "zero-gateway-backend-weights",
		},
		{
			name: "wildcard overlaps exact host",
			analysis: model.Analysis{
				Hostname: "shop.example.com",
				EntryPoints: []model.EntryPoint{
					{
						Origin: model.OriginIstio,
						Resource: model.ResourceIdentity{
							Group: "networking.istio.io", Version: "v1", Kind: "Gateway",
							Namespace: "istio-system", Name: "wildcard",
						},
						Name: "wildcard", Hostname: "*.example.com",
					},
					{
						Origin: model.OriginIstio,
						Resource: model.ResourceIdentity{
							Group: "networking.istio.io", Version: "v1", Kind: "Gateway",
							Namespace: "istio-system", Name: "exact",
						},
						Name: "exact", Hostname: "shop.example.com",
					},
				},
			},
			wantCode: "wildcard-hostname-overlap",
		},
		{
			name: "cross-family hostname conflict",
			analysis: model.Analysis{
				Hostname: "shop.example.com",
				Routes: []model.Route{
					{Origin: model.OriginIstio, Resource: model.ResourceIdentity{Version: "v1", Kind: "VirtualService", Namespace: "shop", Name: "istio-route"}},
					{Origin: model.OriginGatewayAPI, Resource: model.ResourceIdentity{Group: gatewayGroup, Version: "v1", Kind: "HTTPRoute", Namespace: "shop", Name: "gateway-route"}},
				},
			},
			wantCode: "mixed-api-hostname",
		},
		{
			name:     "unmatched hostname",
			analysis: model.Analysis{Hostname: "shop.example.com"},
			wantCode: "no-matching-route",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Diagnose(&tt.analysis, source.Snapshot{Resources: tt.resources}); err != nil {
				t.Fatalf("Diagnose() error = %v", err)
			}
			if finding := findFinding(tt.analysis.Findings, tt.wantCode); finding == nil {
				t.Fatalf("findings = %#v, want code %q", tt.analysis.Findings, tt.wantCode)
			} else if finding.Severity != model.FindingSeverityWarning {
				t.Errorf("finding severity = %q, want warning", finding.Severity)
			}
		})
	}
}

func TestDiagnoseFindingsAreDeterministicallyOrdered(t *testing.T) {
	analysis := model.Analysis{Hostname: "shop.example.com"}
	resources := []source.Resource{
		testResource(
			&istioclientv1.VirtualService{
				ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "route"},
				Spec: istiov1alpha3.VirtualService{
					Hosts: []string{"shop.example.com"}, Gateways: []string{"infra/missing"},
				},
			},
			"networking.istio.io", "v1", "VirtualService", "shop", "route",
		),
	}
	if err := Diagnose(&analysis, source.Snapshot{Resources: resources}); err != nil {
		t.Fatalf("Diagnose() error = %v", err)
	}
	for i := 1; i < len(analysis.Findings); i++ {
		if analysis.Findings[i-1].Code > analysis.Findings[i].Code {
			t.Fatalf("findings are not ordered: %#v", analysis.Findings)
		}
	}
}

func TestDiagnoseTreatsSingleZeroWeightIstioDestinationAsDefault(t *testing.T) {
	analysis := model.Analysis{
		Hostname: "shop.example.com",
		Routes: []model.Route{{
			Origin: model.OriginIstio,
			Resource: model.ResourceIdentity{
				Group: "networking.istio.io", Version: "v1", Kind: "VirtualService",
				Namespace: "shop", Name: "route",
			},
			Rules: []model.Rule{{Index: 1, Destinations: []model.Destination{{
				Kind: model.DestinationKindService, Namespace: "shop", Name: "api", Weight: new(int32),
			}}}},
		}},
	}
	if err := Diagnose(&analysis, source.Snapshot{}); err != nil {
		t.Fatalf("Diagnose() error = %v", err)
	}
	if finding := findFinding(analysis.Findings, "invalid-istio-weights"); finding != nil {
		t.Fatalf("single zero-weight destination reported as invalid: %#v", *finding)
	}
}
