package store

import (
	"encoding/json"
	"fmt"
)

func marshalJSON(value any) (string, error) {
	if value == nil {
		return "null", nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal json: %w", err)
	}
	return string(data), nil
}

func unmarshalJSON(text string, target any) error {
	if text == "" {
		text = "null"
	}
	if err := json.Unmarshal([]byte(text), target); err != nil {
		return fmt.Errorf("unmarshal json: %w", err)
	}
	return nil
}
