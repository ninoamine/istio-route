package model

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestResourceIdentityValidate(t *testing.T) {
	tests := []struct {
		name     string
		identity ResourceIdentity
		wantErr  bool
	}{
		{
			name: "namespaced resource",
			identity: ResourceIdentity{
				Group:     "networking.istio.io",
				Version:   "v1",
				Kind:      "VirtualService",
				Namespace: "shop",
				Name:      "shop-vs",
			},
		},
		{
			name: "cluster-scoped resource",
			identity: ResourceIdentity{
				Group:   "gateway.networking.k8s.io",
				Version: "v1",
				Kind:    "GatewayClass",
				Name:    "istio",
			},
		},
		{
			name: "missing kind",
			identity: ResourceIdentity{
				Version: "v1",
				Name:    "shop-vs",
			},
			wantErr: true,
		},
		{
			name: "missing version",
			identity: ResourceIdentity{
				Kind: "Service",
				Name: "api",
			},
			wantErr: true,
		},
		{
			name: "missing name",
			identity: ResourceIdentity{
				Version: "v1",
				Kind:    "Service",
			},
			wantErr: true,
		},
		{
			name: "invalid namespace",
			identity: ResourceIdentity{
				Version:   "v1",
				Kind:      "Service",
				Namespace: "not a namespace",
				Name:      "api",
			},
			wantErr: true,
		},
		{
			name: "invalid name",
			identity: ResourceIdentity{
				Version: "v1",
				Kind:    "Service",
				Name:    "not a name",
			},
			wantErr: true,
		},
		{
			name: "invalid group",
			identity: ResourceIdentity{
				Group:   "not a group",
				Version: "v1",
				Kind:    "Service",
				Name:    "api",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.identity.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}

func TestAnalysisValidate(t *testing.T) {
	weight := int32(90)
	tests := []struct {
		name     string
		analysis Analysis
		wantErr  string
	}{
		{
			name: "valid analysis",
			analysis: Analysis{
				Hostname: "shop.example.com",
				Source:   SourceSnapshot{Mode: SourceModeCluster},
				Routes: []Route{{
					Origin:    OriginIstio,
					Resource:  ResourceIdentity{Group: "networking.istio.io", Version: "v1", Kind: "VirtualService", Namespace: "shop", Name: "shop-vs"},
					Hostnames: []string{"shop.example.com"},
					Rules: []Rule{{
						Index: 1,
						Destinations: []Destination{{
							Kind:      DestinationKindService,
							Namespace: "shop",
							Name:      "api",
							Weight:    &weight,
						}},
					}},
				}},
			},
		},
		{
			name: "missing hostname",
			analysis: Analysis{
				Source: SourceSnapshot{Mode: SourceModeFiles},
			},
			wantErr: "hostname",
		},
		{
			name: "invalid source mode",
			analysis: Analysis{
				Hostname: "shop.example.com",
				Source:   SourceSnapshot{Mode: "remote"},
			},
			wantErr: "source mode",
		},
		{
			name: "negative destination weight",
			analysis: Analysis{
				Hostname: "shop.example.com",
				Source:   SourceSnapshot{Mode: SourceModeCluster},
				Routes: []Route{{
					Origin:   OriginIstio,
					Resource: ResourceIdentity{Version: "v1", Kind: "VirtualService", Namespace: "shop", Name: "shop-vs"},
					Rules: []Rule{{
						Index: 1,
						Destinations: []Destination{{
							Kind:      DestinationKindService,
							Namespace: "shop",
							Name:      "api",
							Weight:    int32Pointer(-1),
						}},
					}},
				}},
			},
			wantErr: "negative weight",
		},
		{
			name: "external destination requires details",
			analysis: Analysis{
				Hostname: "shop.example.com",
				Source:   SourceSnapshot{Mode: SourceModeCluster},
				Routes: []Route{{
					Origin:   OriginIstio,
					Resource: ResourceIdentity{Version: "v1", Kind: "VirtualService", Namespace: "shop", Name: "shop-vs"},
					Rules: []Rule{{
						Index: 1,
						Destinations: []Destination{{
							Kind: DestinationKindExternal,
							Name: "external.example.com",
						}},
					}},
				}},
			},
			wantErr: "external details",
		},
		{
			name: "finding requires a code",
			analysis: Analysis{
				Hostname: "shop.example.com",
				Source:   SourceSnapshot{Mode: SourceModeCluster},
				Findings: []Finding{{
					Severity: FindingSeverityWarning,
					Message:  "route is not accepted",
				}},
			},
			wantErr: "finding code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.analysis.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestAnalysisSerialization(t *testing.T) {
	weight := int32(100)
	generation := int64(4)
	want := Analysis{
		Hostname: "shop.example.com",
		Source: SourceSnapshot{
			Mode:      SourceModeFiles,
			Files:     []string{"shop.yaml"},
			Families:  []ResourceFamilyStatus{{Family: "gateway-api", Available: true}},
			Namespace: "shop",
		},
		EntryPoints: []EntryPoint{{
			Origin:   OriginGatewayAPI,
			Resource: ResourceIdentity{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway", Namespace: "infra", Name: "edge"},
			Name:     "https",
			Hostname: "shop.example.com",
			Port:     443,
			Protocol: "HTTPS",
			TLS:      &TLSConfig{Mode: "Terminate", CertificateRefs: []ResourceIdentity{{Version: "v1", Kind: "Secret", Namespace: "infra", Name: "shop-tls"}}},
			Conditions: []Condition{{
				Type:               "Programmed",
				Status:             ConditionStatusUnavailable,
				ObservedGeneration: &generation,
			}},
		}},
		Routes: []Route{{
			Origin:    OriginIstio,
			Resource:  ResourceIdentity{Group: "networking.istio.io", Version: "v1", Kind: "VirtualService", Namespace: "shop", Name: "shop-vs"},
			Hostnames: []string{"shop.example.com"},
			Rules: []Rule{{
				Index: 1,
				Destinations: []Destination{{
					Kind:     DestinationKindExternal,
					Name:     "external.example.com",
					Weight:   &weight,
					External: &ExternalDestination{Hosts: []string{"external.example.com"}, Ports: []ExternalPort{{Number: 443, Protocol: "HTTPS"}}, Location: "MESH_EXTERNAL", Resolution: "DNS"},
				}},
			}},
		}},
		Findings: []Finding{{
			Code:     "mixed-api-hostname",
			Severity: FindingSeverityWarning,
			Message:  "hostname is served by both APIs",
			Resources: []ResourceIdentity{{
				Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute", Namespace: "shop", Name: "shop-route",
			}},
		}},
	}

	tests := []struct {
		name      string
		marshal   func(any) ([]byte, error)
		unmarshal func([]byte, any) error
	}{
		{
			name:      "json",
			marshal:   json.Marshal,
			unmarshal: json.Unmarshal,
		},
		{
			name:    "yaml",
			marshal: yaml.Marshal,
			unmarshal: func(data []byte, value any) error {
				return yaml.Unmarshal(data, value)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := want.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			encoded, err := tt.marshal(want)
			if err != nil {
				t.Fatalf("marshal() error = %v", err)
			}

			var got Analysis
			if err := tt.unmarshal(encoded, &got); err != nil {
				t.Fatalf("unmarshal() error = %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("round-trip mismatch:\n got: %#v\nwant: %#v", got, want)
			}

			var fields map[string]any
			if err := json.Unmarshal(mustJSON(t, want), &fields); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			condition := fields["entryPoints"].([]any)[0].(map[string]any)["conditions"].([]any)[0].(map[string]any)
			if _, ok := condition["reason"]; ok {
				t.Fatal("omitted condition reason unexpectedly serialized")
			}
		})
	}
}

func TestNestedValidation(t *testing.T) {
	validIdentity := ResourceIdentity{Version: "v1", Kind: "Service", Namespace: "shop", Name: "api"}
	validService := Service{
		Identity:     validIdentity,
		Availability: AvailabilityPresent,
	}
	validPod := Pod{
		Identity:     ResourceIdentity{Version: "v1", Kind: "Pod", Namespace: "shop", Name: "api-123"},
		Availability: AvailabilityPresent,
	}

	tests := []struct {
		name     string
		validate func() error
		wantErr  string
	}{
		{
			name: "source rejects empty file path",
			validate: func() error {
				return (SourceSnapshot{Mode: SourceModeFiles, Files: []string{""}}).Validate()
			},
			wantErr: "file path",
		},
		{
			name: "source rejects invalid namespace",
			validate: func() error {
				return (SourceSnapshot{Mode: SourceModeCluster, Namespace: "bad namespace"}).Validate()
			},
			wantErr: "source namespace",
		},
		{
			name: "entry point rejects invalid origin",
			validate: func() error {
				return (EntryPoint{
					Origin:   "unknown",
					Resource: ResourceIdentity{Version: "v1", Kind: "Gateway", Name: "edge"},
					Name:     "https",
				}).Validate()
			},
			wantErr: "origin",
		},
		{
			name: "entry point requires name",
			validate: func() error {
				return (EntryPoint{Origin: OriginIstio, Resource: ResourceIdentity{
					Version: "v1", Kind: "Gateway", Name: "edge",
				}}).Validate()
			},
			wantErr: "entry point name",
		},
		{
			name: "entry point validates certificate identity",
			validate: func() error {
				return (EntryPoint{
					Origin:   OriginIstio,
					Resource: ResourceIdentity{Version: "v1", Kind: "Gateway", Name: "edge"},
					Name:     "https",
					TLS:      &TLSConfig{CertificateRefs: []ResourceIdentity{{Kind: "Secret", Name: "tls"}}},
				}).Validate()
			},
			wantErr: "certificate reference",
		},
		{
			name: "entry point requires GatewayClass controller",
			validate: func() error {
				return (EntryPoint{
					Origin:       OriginGatewayAPI,
					Resource:     ResourceIdentity{Version: "v1", Kind: "Gateway", Name: "edge"},
					Name:         "https",
					GatewayClass: &GatewayClassRef{Identity: ResourceIdentity{Version: "v1", Kind: "GatewayClass", Name: "istio"}},
				}).Validate()
			},
			wantErr: "controller",
		},
		{
			name: "route rejects invalid origin",
			validate: func() error {
				return (Route{Origin: "unknown", Resource: validIdentity}).Validate()
			},
			wantErr: "origin",
		},
		{
			name: "route validates parent references",
			validate: func() error {
				return (Route{
					Origin: OriginGatewayAPI, Resource: validIdentity,
					ParentRefs: []ResourceIdentity{{Kind: "Gateway", Name: "edge"}},
				}).Validate()
			},
			wantErr: "parent reference",
		},
		{
			name: "route validates conditions",
			validate: func() error {
				return (Route{
					Origin: OriginGatewayAPI, Resource: validIdentity,
					Conditions: []Condition{{Status: ConditionStatusTrue}},
				}).Validate()
			},
			wantErr: "condition",
		},
		{
			name: "condition rejects invalid status",
			validate: func() error {
				return (Condition{Type: "Accepted", Status: "yes"}).Validate()
			},
			wantErr: "status",
		},
		{
			name: "rule requires positive index",
			validate: func() error {
				return (Rule{}).Validate()
			},
			wantErr: "rule index",
		},
		{
			name: "destination rejects unknown kind",
			validate: func() error {
				return (Destination{Kind: "unknown", Name: "api"}).Validate()
			},
			wantErr: "destination kind",
		},
		{
			name: "destination validates nested service",
			validate: func() error {
				return (Destination{
					Kind: DestinationKindService, Name: "api",
					Service: &Service{Identity: validIdentity},
				}).Validate()
			},
			wantErr: "availability",
		},
		{
			name: "service requires Service identity",
			validate: func() error {
				return (Service{
					Identity:     ResourceIdentity{Version: "v1", Kind: "Pod", Name: "api"},
					Availability: AvailabilityPresent,
				}).Validate()
			},
			wantErr: "kind must be Service",
		},
		{
			name: "service validates availability",
			validate: func() error {
				return (Service{Identity: validIdentity}).Validate()
			},
			wantErr: "availability",
		},
		{
			name: "service allows explicit unavailable and not-found status",
			validate: func() error {
				for _, availability := range []Availability{AvailabilityNotFound, AvailabilityUnavailable} {
					if err := (Service{
						Identity: validIdentity, Availability: availability,
					}).Validate(); err != nil {
						return err
					}
				}
				return nil
			},
		},
		{
			name: "service validates endpoints",
			validate: func() error {
				return (Service{
					Identity: validIdentity, Availability: AvailabilityPresent,
					Endpoints: []Endpoint{{Target: &ResourceIdentity{Kind: "Pod", Name: "api"}}},
				}).Validate()
			},
			wantErr: "endpoint",
		},
		{
			name: "endpoint validates Pod kind",
			validate: func() error {
				return (Endpoint{Pod: &Pod{
					Identity:     ResourceIdentity{Version: "v1", Kind: "Service", Name: "api"},
					Availability: AvailabilityPresent,
				}}).Validate()
			},
			wantErr: "pod identity kind",
		},
		{
			name: "valid endpoint preserves optional readiness",
			validate: func() error {
				ready := true
				return (Endpoint{Ready: &ready, Pod: &validPod}).Validate()
			},
		},
		{
			name: "finding requires message",
			validate: func() error {
				return (Finding{Code: "issue", Severity: FindingSeverityWarning}).Validate()
			},
			wantErr: "message",
		},
		{
			name: "finding rejects unsupported severity",
			validate: func() error {
				return (Finding{Code: "issue", Severity: "fatal", Message: "issue"}).Validate()
			},
			wantErr: "severity",
		},
		{
			name: "finding validates related resources",
			validate: func() error {
				return (Finding{
					Code: "issue", Severity: FindingSeverityWarning, Message: "issue",
					Resources: []ResourceIdentity{{Kind: "Service", Name: "api"}},
				}).Validate()
			},
			wantErr: "resource",
		},
		{
			name: "valid service and finding",
			validate: func() error {
				if err := validService.Validate(); err != nil {
					return err
				}
				return (Finding{Code: "issue", Severity: FindingSeverityInfo, Message: "informational"}).Validate()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validate() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func int32Pointer(value int32) *int32 {
	return &value
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return encoded
}
