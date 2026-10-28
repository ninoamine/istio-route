package render

import (
	"fmt"

	"github.com/ninoamine/istio-route/internal/model"
)

const (
	FormatText    = "text"
	FormatJSON    = "json"
	FormatYAML    = "yaml"
	FormatMermaid = "mermaid"
)

type Options struct {
	Color bool
}

func Render(analysis model.Analysis, format string, options Options) (string, error) {
	switch format {
	case FormatText:
		return TextWithOptions(analysis, options), nil
	case FormatJSON:
		return JSON(analysis)
	case FormatYAML:
		return YAML(analysis)
	case FormatMermaid:
		return Mermaid(analysis), nil
	default:
		return "", fmt.Errorf("unsupported output format %q", format)
	}
}
