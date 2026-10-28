package render

import (
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/ninoamine/istio-route/internal/model"
)

type textStyles struct {
	enabled  bool
	renderer *lipgloss.Renderer
}

func newTextStyles(enabled bool) textStyles {
	renderer := lipgloss.NewRenderer(io.Discard)
	renderer.SetColorProfile(termenv.TrueColor)
	return textStyles{
		enabled:  enabled,
		renderer: renderer,
	}
}

func (s textStyles) root(label string) string {
	return s.style(label, lipgloss.Color("6"))
}

func (s textStyles) node(label string) string {
	switch {
	case strings.HasPrefix(label, "[Istio]"):
		return s.style(label, lipgloss.Color("4"))
	case strings.HasPrefix(label, "[Gateway API]"):
		return s.style(label, lipgloss.Color("5"))
	default:
		return label
	}
}

func (s textStyles) warning(label string) string {
	return s.style(label, lipgloss.Color("3"))
}

func (s textStyles) condition(condition model.Condition) string {
	label := plainConditionLabel(condition)
	switch condition.Status {
	case model.ConditionStatusTrue:
		return s.style(label, lipgloss.Color("2"))
	case model.ConditionStatusFalse:
		return s.style(label, lipgloss.Color("1"))
	default:
		return s.style(label, lipgloss.Color("3"))
	}
}

func (s textStyles) style(value string, color lipgloss.Color) string {
	if !s.enabled {
		return value
	}
	return s.renderer.NewStyle().Foreground(color).Render(value)
}
