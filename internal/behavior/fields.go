package behavior

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ninoamine/istio-route/internal/model"
)

func lookupAnalysisField(analysis model.Analysis, path string) (string, bool, error) {
	data, err := json.Marshal(analysis)
	if err != nil {
		return "", false, fmt.Errorf("encode trace fields: %w", err)
	}
	var root any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return "", false, fmt.Errorf("decode trace fields: %w", err)
	}
	return lookupField(root, path)
}

func lookupField(root any, path string) (string, bool, error) {
	if path == "" {
		return "", false, fmt.Errorf("field path is empty")
	}

	current := root
	for remaining := path; remaining != ""; {
		if remaining[0] == '.' || remaining[0] == '[' {
			return "", false, fmt.Errorf("invalid field path %q", path)
		}
		fieldEnd := strings.IndexAny(remaining, ".[")
		if fieldEnd == -1 {
			fieldEnd = len(remaining)
		}
		field := remaining[:fieldEnd]
		if field == "" {
			return "", false, fmt.Errorf("invalid field path %q", path)
		}
		if !isFieldName(field) {
			return "", false, fmt.Errorf("invalid field name %q in field path %q", field, path)
		}
		object, ok := current.(map[string]any)
		if !ok {
			return "", false, fmt.Errorf("field %q cannot be read from %T", field, current)
		}

		current, ok = object[field]
		if !ok {
			return "<absent>", false, nil
		}
		remaining = remaining[fieldEnd:]

		for strings.HasPrefix(remaining, "[") {
			end := strings.IndexByte(remaining, ']')
			if end < 0 {
				return "", false, fmt.Errorf("invalid field path %q", path)
			}
			index, err := strconv.Atoi(remaining[1:end])
			if err != nil || index < 0 {
				return "", false, fmt.Errorf("invalid list index in field path %q", path)
			}
			list, ok := current.([]any)
			if !ok {
				return "", false, fmt.Errorf("index %d cannot be read from %T", index, current)
			}
			if index >= len(list) {
				return "<absent>", false, nil
			}
			current = list[index]
			remaining = remaining[end+1:]
		}

		if remaining == "" {
			break
		}
		if remaining[0] != '.' {
			return "", false, fmt.Errorf("invalid field path %q", path)
		}
		remaining = remaining[1:]
		if remaining == "" {
			return "", false, fmt.Errorf("invalid field path %q", path)
		}
	}

	switch value := current.(type) {
	case string:
		return value, true, nil
	case json.Number:
		return value.String(), true, nil
	case bool:
		return strconv.FormatBool(value), true, nil
	case nil:
		return "null", true, nil
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return "", false, fmt.Errorf("encode trace field %q: %w", path, err)
		}
		return string(data), true, nil
	}
}

func isFieldName(name string) bool {
	for i, char := range name {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || char == '_' ||
			(i > 0 && char >= '0' && char <= '9') {
			continue
		}
		return false
	}
	return true
}
