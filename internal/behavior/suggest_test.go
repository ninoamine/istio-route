package behavior

import "testing"

func TestSuggestStepSuggestions(t *testing.T) {
	tests := []struct {
		name string
		step string
		want string
	}{
		{
			name: "exact step",
			step: `Then the hostname should be found`,
			want: `Then the hostname should be found`,
		},
		{
			name: "near match ignores values",
			step: `When I trace the host "shop.example.com"`,
			want: `When I trace the hostname "shop.example.com"`,
		},
		{
			name: "close match",
			step: `the hostname should be found`,
			want: `Then the hostname should be found`,
		},
		{
			name: "unrelated step has no suggestion",
			step: `the moon is made of green cheese`,
			want: "",
		},
		{
			name: "empty step has no suggestion",
			step: "  ",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := suggestStep(tt.step); got != tt.want {
				t.Errorf("suggestStep(%q) = %q, want %q", tt.step, got, tt.want)
			}
		})
	}
}

func TestClosestStepUsesStableOrderForTies(t *testing.T) {
	candidates := []string{"Then route is good", "Then route is best"}
	for range 3 {
		if got := closestStep("Then route is ", candidates); got != candidates[0] {
			t.Errorf("closestStep() = %q, want first equally close candidate %q", got, candidates[0])
		}
	}
}

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		first, second string
		want          int
	}{
		{first: "kitten", second: "sitting", want: 3},
		{first: "route", second: "route", want: 0},
		{first: "gateway", second: "gate", want: 3},
		{first: "café", second: "caff", want: 1},
	}
	for _, tt := range tests {
		if got := levenshtein(tt.first, tt.second); got != tt.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tt.first, tt.second, got, tt.want)
		}
	}
}
