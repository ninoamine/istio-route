package render

import (
	"encoding/json"
	"fmt"

	"github.com/ninoamine/istio-route/internal/model"
)

func JSON(analysis model.Analysis) (string, error) {
	contents, err := json.MarshalIndent(analysis, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode JSON report: %w", err)
	}
	return string(contents) + "\n", nil
}
