package audit

import (
	"testing"

	"github.com/ninoamine/istio-route/internal/model"
)

func TestDeficientFindings(t *testing.T) {
	tests := []struct {
		name     string
		result   Result
		wantCode string
		wantSev  model.FindingSeverity
	}{
		{
			name: "winning route with zero ready addresses is an error",
			result: Result{Routes: []Route{{
				Identity: testIdentity("HTTPRoute", "shop", "winner"),
				Rules: []Rule{{
					Index:      1,
					Precedence: PrecedenceWinner,
					Backends: []Backend{{
						Reference: BackendReference{Namespace: "shop", Name: "api", Kind: "Service"},
						Service:   &BackendService{Identity: testIdentity("Service", "shop", "api"), Availability: model.AvailabilityPresent, EndpointSlicesSupplied: true, ReadyAddressCount: 0},
					}},
				}},
			}}},
			wantCode: "deficient-destination",
			wantSev:  model.FindingSeverityError,
		},
		{
			name: "shadowed route with zero ready addresses is a warning",
			result: Result{Routes: []Route{{
				Identity: testIdentity("HTTPRoute", "shop", "shadowed"),
				Rules: []Rule{{
					Index:      1,
					Precedence: PrecedenceShadowed,
					Backends: []Backend{{
						Reference: BackendReference{Namespace: "shop", Name: "api", Kind: "Service"},
						Service:   &BackendService{Identity: testIdentity("Service", "shop", "api"), Availability: model.AvailabilityPresent, EndpointSlicesSupplied: true, ReadyAddressCount: 0},
					}},
				}},
			}}},
			wantCode: "deficient-destination",
			wantSev:  model.FindingSeverityWarning,
		},
		{
			name: "offline input without EndpointSlices is skipped",
			result: Result{Routes: []Route{{
				Identity: testIdentity("HTTPRoute", "shop", "winner"),
				Rules: []Rule{{
					Index:      1,
					Precedence: PrecedenceWinner,
					Backends: []Backend{{
						Reference: BackendReference{Namespace: "shop", Name: "api", Kind: "Service"},
						Service:   &BackendService{Identity: testIdentity("Service", "shop", "api"), Availability: model.AvailabilityPresent, EndpointSlicesSupplied: false, ReadyAddressCount: 0},
					}},
				}},
			}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := deficientFindings(tt.result)
			if tt.wantCode == "" {
				if len(findings) != 0 {
					t.Fatalf("deficientFindings() = %#v, want no findings", findings)
				}
				return
			}
			if len(findings) != 1 {
				t.Fatalf("deficientFindings() returned %d findings, want 1: %#v", len(findings), findings)
			}
			if findings[0].Code != tt.wantCode {
				t.Fatalf("finding code = %q, want %q", findings[0].Code, tt.wantCode)
			}
			if findings[0].Severity != tt.wantSev {
				t.Fatalf("finding severity = %q, want %q", findings[0].Severity, tt.wantSev)
			}
			if findings[0].Category != string(CategoryDeficientDestination) {
				t.Fatalf("finding category = %q, want %q", findings[0].Category, CategoryDeficientDestination)
			}
		})
	}
}
