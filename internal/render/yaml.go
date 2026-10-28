package render

import (
	"fmt"

	"github.com/ninoamine/istio-route/internal/model"
	"sigs.k8s.io/yaml"
)

func YAML(analysis model.Analysis) (string, error) {
	contents, err := yaml.Marshal(analysis)
	if err != nil {
		return "", fmt.Errorf("encode YAML report: %w", err)
	}
	return string(contents), nil
}
