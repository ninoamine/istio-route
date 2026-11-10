package simulate

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ninoamine/istio-route/internal/model"
	"sigs.k8s.io/yaml"
)

func TestValidate(t *testing.T) {
	route := RouteRef{Namespace: "app", Name: "reviews"}
	tests := []struct {
		name    string
		check   func() error
		wantErr bool
	}{
		{name: "valid outcome", check: func() error { return ClassificationStillCovered.Validate() }},
		{name: "invalid outcome", check: func() error { return Classification("maybe").Validate() }, wantErr: true},
		{name: "valid attachment state", check: func() error { return AttachmentUnconfirmed.Validate() }},
		{name: "invalid attachment state", check: func() error { return AttachmentState("pending").Validate() }, wantErr: true},
		{name: "invalid validation state", check: func() error { return ValidationState("valid").Validate() }, wantErr: true},
		{name: "invalid result kind", check: func() error { return ResultKind("summary").Validate() }, wantErr: true},
		{name: "route requires namespace", check: func() error { return RouteRef{Name: "reviews"}.Validate() }, wantErr: true},
		{name: "route requires name", check: func() error { return RouteRef{Namespace: "app"}.Validate() }, wantErr: true},
		{name: "rule index zero", check: func() error { return RuleRef{Route: route, Index: 0}.Validate() }, wantErr: true},
		{name: "rule index one", check: func() error { return RuleRef{Route: route, Index: 1}.Validate() }},
		{name: "provider index zero", check: func() error { return Provider{Route: route}.Validate() }, wantErr: true},
		{name: "listener missing name", check: func() error {
			return WaypointListener{Port: 15008, Protocol: "HBONE"}.Validate()
		}, wantErr: true},
		{name: "attachment missing listener", check: func() error {
			return ListenerAttachment{Route: route, State: AttachmentAccepted}.Validate()
		}, wantErr: true},
		{name: "predicate with no kind", check: func() error { return Predicate{}.Validate() }, wantErr: true},
		{name: "predicate with two kinds", check: func() error {
			return Predicate{
				All:  []Predicate{{Atom: &Atom{Dimension: "method", Value: "GET"}}},
				Atom: &Atom{Dimension: "method", Value: "GET"},
			}.Validate()
		}, wantErr: true},
		{name: "predicate negation of atom", check: func() error {
			return Predicate{Not: &Predicate{Atom: &Atom{Dimension: "path", MatchType: "PathPrefix", Value: "/api/v2"}}}.Validate()
		}},
		{name: "header atom requires name", check: func() error {
			return Predicate{Atom: &Atom{Dimension: "header", Value: "x"}}.Validate()
		}, wantErr: true},
		{name: "unsupported dimension", check: func() error {
			return Predicate{Atom: &Atom{Dimension: "cookie", Value: "x"}}.Validate()
		}, wantErr: true},
		{name: "provider none must not carry provider", check: func() error {
			return ProviderView{State: ProviderNone, Provider: &Provider{Route: route, RuleIndex: 1}}.Validate()
		}, wantErr: true},
		{name: "provider known requires provider", check: func() error {
			return ProviderView{State: ProviderKnown}.Validate()
		}, wantErr: true},
		{name: "provider unknown is valid", check: func() error {
			return ProviderView{State: ProviderUnknown}.Validate()
		}},
		{name: "entry missing listener", check: func() error {
			return Entry{
				Rule: RuleRef{Route: route, Index: 1}, Region: Predicate{Atom: &Atom{Dimension: "method", Value: "GET"}},
				Outcome: ClassificationNoChange, Before: ProviderView{State: ProviderNone}, After: ProviderView{State: ProviderNone},
			}.Validate()
		}, wantErr: true},
		{name: "entry with invalid outcome", check: func() error {
			return Entry{
				Listener: "mesh", Rule: RuleRef{Route: route, Index: 1},
				Region:  Predicate{Atom: &Atom{Dimension: "method", Value: "GET"}},
				Outcome: "exclusive", Before: ProviderView{State: ProviderNone}, After: ProviderView{State: ProviderNone},
			}.Validate()
		}, wantErr: true},
		{name: "valid entry", check: func() error {
			return Entry{
				Listener: "mesh", Rule: RuleRef{Route: route, Index: 1},
				Region:  Predicate{Atom: &Atom{Dimension: "path", MatchType: "PathPrefix", Value: "/"}},
				Outcome: ClassificationUnrouted, Before: ProviderView{State: ProviderUnknown}, After: ProviderView{State: ProviderNone},
			}.Validate()
		}},
		{name: "waypoint requires name", check: func() error {
			return Waypoint{Validation: ValidationConfirmed}.Validate()
		}, wantErr: true},
		{name: "result with invalid kind", check: func() error {
			return Result{Kind: "other", Waypoint: Waypoint{Name: "waypoint", Validation: ValidationConfirmed}}.Validate()
		}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.check()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func sampleResult() Result {
	provider := &Provider{
		Route: RouteRef{Namespace: "app", Name: "reviews-v2"}, RuleIndex: 2,
		Backends: []BackendRef{{Namespace: "app", Name: "reviews-v2", Port: ptr(int32(8080)), Weight: ptr(int32(100))}},
	}
	return Result{
		Kind:   ResultImpact,
		Source: model.SourceSnapshot{Mode: model.SourceModeFiles, Files: []string{"simulate.yaml"}},
		Waypoint: Waypoint{
			Namespace: "app", Name: "waypoint", Class: waypointClassName, Validation: ValidationConfirmed,
			Listeners: []WaypointListener{{Name: "mesh", Port: 15008, Protocol: "HBONE"}},
		},
		Scope: Scope{DirectHTTPRouteOnly: true, ServiceParentedExcluded: 1, ExcludedRoutes: []RouteRef{{Namespace: "app", Name: "legacy"}}},
		Routes: []RouteListing{{
			Route: RouteRef{Namespace: "app", Name: "reviews"}, Hostnames: []string{"reviews.example.com"},
			Listeners: []ListenerAttachment{{Route: RouteRef{Namespace: "app", Name: "reviews"}, Listener: "mesh", State: AttachmentUnconfirmed, Reason: "status missing"}},
		}},
		SelectedRoute: &RouteRef{Namespace: "app", Name: "reviews"},
		Entries: []Entry{{
			Listener: "mesh",
			Rule:     RuleRef{Route: RouteRef{Namespace: "app", Name: "reviews"}, Index: 1},
			Region: Predicate{All: []Predicate{
				{Atom: &Atom{Dimension: "hostname", MatchType: "Exact", Value: "reviews.example.com"}},
				{Not: &Predicate{Atom: &Atom{Dimension: "path", MatchType: "PathPrefix", Value: "/api/v2"}}},
			}},
			Outcome: ClassificationStillCovered, Before: ProviderView{State: ProviderKnown, Provider: &Provider{Route: RouteRef{Namespace: "app", Name: "reviews"}, RuleIndex: 1}},
			After: ProviderView{State: ProviderKnown, Provider: provider}, BackendChanged: true,
			Reasons: []string{"served by another route"},
		}},
		RuleSummaries: []RuleSummary{{
			Rule: RuleRef{Route: RouteRef{Namespace: "app", Name: "reviews"}, Index: 1}, Outcomes: []Classification{ClassificationStillCovered},
			Counts: map[Classification]int{ClassificationStillCovered: 1}, Partial: false,
		}},
		Totals:   Totals{StillCovered: 1},
		Warnings: []string{"snapshot is not atomic"},
	}
}

func ptr[T any](v T) *T { return &v }

func TestResultJSONAndYAMLUseContractFieldNames(t *testing.T) {
	result := sampleResult()
	jsonBytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	yamlBytes, err := yaml.Marshal(result)
	if err != nil {
		t.Fatalf("marshal YAML: %v", err)
	}
	for _, field := range []string{
		`"kind"`, `"source"`, `"waypoint"`, `"scope"`, `"routes"`, `"selectedRoute"`,
		`"entries"`, `"ruleSummaries"`, `"totals"`, `"warnings"`, `"outcome"`, `"backendChanged"`,
		`"still-covered"`, `"serviceParentedExcluded"`, `"validation"`, `"ruleIndex"`,
	} {
		if !strings.Contains(string(jsonBytes), field) {
			t.Errorf("JSON output missing %s", field)
		}
	}
	for _, field := range []string{"kind:", "entries:", "ruleSummaries:", "backendChanged:", "outcome:", "still-covered:"} {
		if !strings.Contains(string(yamlBytes), field) {
			t.Errorf("YAML output missing %s", field)
		}
	}

	var fromJSON Result
	if err := json.Unmarshal(jsonBytes, &fromJSON); err != nil {
		t.Fatalf("unmarshal JSON: %v", err)
	}
	if !reflect.DeepEqual(result, fromJSON) {
		t.Errorf("JSON round trip mismatch:\n got %#v\nwant %#v", fromJSON, result)
	}
	var fromYAML Result
	if err := yaml.Unmarshal(yamlBytes, &fromYAML); err != nil {
		t.Fatalf("unmarshal YAML: %v", err)
	}
	if !reflect.DeepEqual(result, fromYAML) {
		t.Errorf("YAML round trip mismatch:\n got %#v\nwant %#v", fromYAML, result)
	}
}
