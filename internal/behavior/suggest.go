package behavior

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	stepKeywordPattern = regexp.MustCompile(`(?i)^(given|when|then|and|but)\s+`)
	stepValuePattern   = regexp.MustCompile(`"[^"]*"|\d+`)
)

func suggestStep(step string) string {
	return closestStep(step, supportedStepExamples)
}

func closestStep(step string, candidates []string) string {
	step = strings.TrimSpace(step)
	if step == "" || len(candidates) == 0 {
		return ""
	}
	normalized := normalizeStep(step)
	best := ""
	bestDistance := 0
	for _, candidate := range candidates {
		distance := levenshtein(normalized, normalizeStep(candidate))
		if distance < bestDistance {
			best = candidate
			bestDistance = distance
		} else if best == "" {
			best = candidate
			bestDistance = distance
		}
	}
	maxDistance := max(5, utf8.RuneCountInString(normalized)/4)
	if bestDistance > maxDistance {
		return ""
	}
	return best
}

func normalizeStep(step string) string {
	step = stepKeywordPattern.ReplaceAllString(strings.TrimSpace(step), "")
	step = stepValuePattern.ReplaceAllString(step, `""`)
	return strings.ToLower(strings.Join(strings.Fields(step), " "))
}

func levenshtein(first, second string) int {
	firstRunes, secondRunes := []rune(first), []rune(second)
	previous := make([]int, len(secondRunes)+1)
	for i := range previous {
		previous[i] = i
	}
	for i := 1; i <= len(firstRunes); i++ {
		current := make([]int, len(secondRunes)+1)
		current[0] = i
		for j := 1; j <= len(secondRunes); j++ {
			cost := 1
			if firstRunes[i-1] == secondRunes[j-1] {
				cost = 0
			}
			current[j] = min(
				current[j-1]+1,
				previous[j]+1,
				previous[j-1]+cost,
			)
		}
		previous = current
	}
	return previous[len(secondRunes)]
}
