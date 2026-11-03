package audit

import (
	"testing"
	"time"

	"github.com/ninoamine/istio-route/internal/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestApplyPrecedenceUsesSpecificity(t *testing.T) {
	oldRoute := Route{
		Identity:  testIdentity("HTTPRoute", "shop", "general"),
		Hostnames: []string{"shop.example.com"},
		Rules: []Rule{{
			Index:         1,
			ListenerNames: []string{"https"},
			Matches:       []Match{{Path: &PathMatch{Type: "PathPrefix", Value: "/"}}},
			Precedence:    PrecedenceWinner,
		}},
	}
	specificRoute := Route{
		Identity:  testIdentity("HTTPRoute", "shop", "specific"),
		Hostnames: []string{"shop.example.com"},
		Rules: []Rule{{
			Index:         1,
			ListenerNames: []string{"https"},
			Matches:       []Match{{Path: &PathMatch{Type: "Exact", Value: "/orders"}}},
			Precedence:    PrecedenceWinner,
		}},
	}
	result := Result{Routes: []Route{oldRoute, specificRoute}}
	applyPrecedence(&result)
	if got := result.Routes[0].Rules[0].Precedence; got != PrecedencePartialOverlap {
		t.Fatalf("general route precedence = %q, want %q", got, PrecedencePartialOverlap)
	}
	if got := result.Routes[1].Rules[0].Precedence; got != PrecedenceWinner {
		t.Fatalf("specific route precedence = %q, want %q", got, PrecedenceWinner)
	}
}

func TestApplyPrecedenceUsesCreationTimeAndResourceTieBreakers(t *testing.T) {
	older := &Rule{Index: 1, Matches: []Match{{Path: &PathMatch{Type: "Exact", Value: "/"}}}, CreatedAt: &metav1.Time{Time: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)}}
	newer := &Rule{Index: 1, Matches: []Match{{Path: &PathMatch{Type: "Exact", Value: "/"}}}, CreatedAt: &metav1.Time{Time: time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)}}
	result := Result{Routes: []Route{{Identity: testIdentity("HTTPRoute", "shop", "older"), Hostnames: []string{"shop.example.com"}, Rules: []Rule{*older}}, {Identity: testIdentity("HTTPRoute", "shop", "newer"), Hostnames: []string{"shop.example.com"}, Rules: []Rule{*newer}}}}
	applyPrecedence(&result)
	if got := result.Routes[0].Rules[0].Precedence; got != PrecedenceWinner {
		t.Fatalf("older route precedence = %q, want %q", got, PrecedenceWinner)
	}
	if got := result.Routes[1].Rules[0].Precedence; got != PrecedenceShadowed {
		t.Fatalf("newer route precedence = %q, want %q", got, PrecedenceShadowed)
	}
}

func TestPrecedenceFindingsUseStableCategories(t *testing.T) {
	result := Result{Routes: []Route{{Identity: testIdentity("HTTPRoute", "shop", "general"), Hostnames: []string{"shop.example.com"}, Rules: []Rule{{Index: 1, ListenerNames: []string{"https"}, Matches: []Match{{Path: &PathMatch{Type: "PathPrefix", Value: "/"}}}, Precedence: PrecedenceShadowed}}}, {Identity: testIdentity("HTTPRoute", "shop", "specific"), Hostnames: []string{"shop.example.com"}, Rules: []Rule{{Index: 1, ListenerNames: []string{"https"}, Matches: []Match{{Path: &PathMatch{Type: "Exact", Value: "/"}}}, Precedence: PrecedenceWinner}}}}}
	findings := precedenceFindings(result)
	if len(findings) == 0 {
		t.Fatal("precedenceFindings() returned no findings")
	}
	if findings[0].Category != string(CategoryPrecedence) {
		t.Fatalf("first precedence finding category = %q, want %q", findings[0].Category, CategoryPrecedence)
	}
	if findings[0].Severity != model.FindingSeverityWarning {
		t.Fatalf("first precedence finding severity = %q, want %q", findings[0].Severity, model.FindingSeverityWarning)
	}
}
