package utils

import (
	"bytes"
	"encoding/json"
	"fmt"

	"sigs.k8s.io/yaml"
)

// DecodeYAMLMap decodes a YAML mapping into a generic map. Numbers are decoded as
// json.Number rather than float64, so encoding the map again with yaml.Marshal keeps
// every value, including integers above 2^53. Key order and number formatting may
// change. An empty document decodes to an empty map.
func DecodeYAMLMap(data []byte) (map[string]any, error) {
	// Convert to JSON, which keeps integers exact, then decode without float64.
	jsn, err := yaml.YAMLToJSON(data)
	if err != nil {
		return nil, fmt.Errorf("does not parse as YAML: %w", err)
	}
	var m map[string]any
	decoder := json.NewDecoder(bytes.NewReader(jsn))
	decoder.UseNumber()
	if err := decoder.Decode(&m); err != nil {
		return nil, fmt.Errorf("is not a YAML mapping: %w", err)
	}

	// Treat an empty document as an empty mapping.
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}
