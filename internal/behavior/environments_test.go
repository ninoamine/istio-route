package behavior

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

func TestParseManifestSets(t *testing.T) {
	tests := []struct {
		name        string
		files       []string
		named       []string
		want        []ManifestSet
		wantErrText string
	}{
		{
			name:  "unnamed files form one set",
			files: []string{"prod.yaml", "services.yaml"},
			want:  []ManifestSet{{Files: []string{"prod.yaml", "services.yaml"}}},
		},
		{
			name:  "repeated names append files",
			named: []string{"prod=prod.yaml", "staging=staging.yaml", "prod=services.yaml"},
			want: []ManifestSet{
				{Name: "prod", Files: []string{"prod.yaml", "services.yaml"}},
				{Name: "staging", Files: []string{"staging.yaml"}},
			},
		},
		{
			name:        "missing environment separator",
			named:       []string{"prod.yaml"},
			wantErrText: "name=file",
		},
		{
			name:        "empty environment name",
			named:       []string{"=prod.yaml"},
			wantErrText: "name cannot be empty",
		},
		{
			name:        "empty manifest path",
			named:       []string{"prod="},
			wantErrText: "file path cannot be empty",
		},
		{
			name:        "invalid environment tag name",
			named:       []string{"prod,staging=manifests.yaml"},
			wantErrText: "name may contain only",
		},
		{
			name:        "duplicate manifest definition",
			named:       []string{"prod=prod.yaml", "prod=prod.yaml"},
			wantErrText: "duplicate manifest file",
		},
		{
			name:        "mixed named and unnamed sets",
			files:       []string{"prod.yaml"},
			named:       []string{"staging=staging.yaml"},
			wantErrText: "cannot mix",
		},
		{
			name:        "stdin may only be used once",
			named:       []string{"prod=-", "staging=-"},
			wantErrText: "standard input may be specified only once",
		},
		{
			name:        "no sets",
			wantErrText: "at least one rendered manifest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseManifestSets(tt.files, tt.named)
			if tt.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrText) {
					t.Fatalf("ParseManifestSets() error = %v, want substring %q", err, tt.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseManifestSets() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseManifestSets() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestEnvironmentTagExpression(t *testing.T) {
	tests := []struct {
		name         string
		current      string
		environments []string
		want         string
	}{
		{
			name:         "matching environment includes common and matching scenarios",
			current:      "prod",
			environments: []string{"prod", "staging"},
			want:         "~@prod&&~@staging,@prod",
		},
		{
			name:         "unmatched tags remain ordinary tags",
			current:      "staging",
			environments: []string{"prod", "staging"},
			want:         "~@prod&&~@staging,@staging",
		},
		{
			name:         "single named environment does not filter scenarios",
			current:      "prod",
			environments: []string{"prod"},
			want:         "~@prod,@prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := environmentTagExpression(tt.current, tt.environments); got != tt.want {
				t.Errorf("environmentTagExpression() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEnvironmentTagSelection(t *testing.T) {
	feature := filepath.Join(t.TempDir(), "environments.feature")
	contents := `Feature: Environment selection
  @prod
  Scenario: production
    Given the hostname should be found

  @staging
  Scenario: staging
    Given the hostname should be found

  @qa
  Scenario: unmatched environment tag
    Given the hostname should be found

  @smoke
  Scenario: ordinary tag
    Given the hostname should be found

  Scenario: shared
    Given the hostname should be found
`
	if err := os.WriteFile(feature, []byte(contents), 0o600); err != nil {
		t.Fatalf("write feature: %v", err)
	}

	tests := []struct {
		name     string
		expected []string
	}{
		{
			name:     "prod",
			expected: []string{"production", "unmatched environment tag", "ordinary tag", "shared"},
		},
		{
			name:     "staging",
			expected: []string{"staging", "unmatched environment tag", "ordinary tag", "shared"},
		},
	}
	names := []string{"prod", "staging"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suite := godog.TestSuite{Options: &godog.Options{
				Paths: []string{feature},
				Tags:  environmentTagExpression(tt.name, names),
			}}
			features, err := suite.RetrieveFeatures()
			if err != nil {
				t.Fatalf("RetrieveFeatures() error = %v", err)
			}
			var got []string
			for _, parsedFeature := range features {
				for _, pickle := range parsedFeature.Pickles {
					got = append(got, pickle.Name)
				}
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("selected scenarios = %#v, want %#v", got, tt.expected)
			}
		})
	}
}

func TestAssertManifestsChecksKnownAndCurrentEnvironment(t *testing.T) {
	state := &scenarioState{
		manifestName:     "prod",
		environmentNames: []string{"prod", "staging"},
	}
	tests := []struct {
		name        string
		manifest    string
		wantErr     bool
		wantUnknown bool
	}{
		{name: "matching environment", manifest: "prod"},
		{
			name:     "known but conflicting environment",
			manifest: "staging",
			wantErr:  true,
		},
		{name: "unknown environment", manifest: "qa", wantErr: true, wantUnknown: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := state.assertManifests(tt.manifest)
			if (err != nil) != tt.wantErr {
				t.Fatalf("assertManifests() error = %v, want error %t", err, tt.wantErr)
			}
			if errors.Is(err, ErrUnknownEnvironment) != tt.wantUnknown {
				t.Errorf("assertManifests() error = %v, unknown-environment = %t", err, errors.Is(err, ErrUnknownEnvironment))
			}
			if got, want := errors.Is(err, ErrConflictingEnvironment), tt.name == "known but conflicting environment"; got != want {
				t.Errorf("assertManifests() error = %v, conflicting-environment = %t; want %t", err, got, want)
			}
		})
	}
}
