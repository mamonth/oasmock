package loader

import (
	"bytes"
	"encoding/json"

	"gopkg.in/yaml.v3"
)

// decodeYAMLDocument unmarshals YAML bytes into a generic mapping.
func decodeYAMLDocument(data []byte) (map[string]any, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

// decodeJSONDocument unmarshals JSON bytes into a generic mapping.
func decodeJSONDocument(data []byte) (map[string]any, error) {
	var doc map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(data), &doc); err != nil {
		return nil, err
	}
	return doc, nil
}
