package helm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

type helmValuesSchemaNode struct {
	Properties map[string]*helmValuesSchemaNode `json:"properties,omitempty"`
}

// TestHelmValuesSchemaMatchesDefaults ensures values.schema.json stays in sync with values.yaml.
// It's a lightweight guard against silent schema drift: if a field is added to values.yaml without
// a matching schema entry, additionalProperties:false won't catch it (the new field is simply
// unvalidated), so we catch it here instead.
func TestHelmValuesSchemaMatchesDefaults(t *testing.T) {
	valuesPath := filepath.Join("..", "..", "install", "helm", "kgateway", "values.yaml")
	schemaPath := filepath.Join("..", "..", "install", "helm", "kgateway", "values.schema.json")

	values, err := loadHelmValues(valuesPath)
	require.NoError(t, err)

	schema, err := loadHelmValuesSchema(schemaPath)
	require.NoError(t, err)

	missingFromSchema, missingFromValues := compareHelmValuesSchemaKeys(values, schema, "")
	if len(missingFromSchema) > 0 || len(missingFromValues) > 0 {
		t.Fatalf("values.yaml and values.schema.json are out of sync:\n%s",
			formatHelmValuesSchemaDiff(missingFromSchema, missingFromValues))
	}
}

func compareHelmValuesSchemaKeys(values map[string]any, schema *helmValuesSchemaNode, prefix string) (missingFromSchema, missingFromValues []string) {
	valuesKeys := keysOfHelmValuesMap(values)
	var schemaKeys []string
	if schema != nil {
		schemaKeys = make([]string, 0, len(schema.Properties))
		for k := range schema.Properties {
			schemaKeys = append(schemaKeys, k)
		}
	}

	valuesSet := helmValuesKeySet(valuesKeys)
	schemaSet := helmValuesKeySet(schemaKeys)

	missingFromSchema = sortedHelmValuesKeyDiff(valuesSet, schemaSet, prefix)
	missingFromValues = sortedHelmValuesKeyDiff(schemaSet, valuesSet, prefix)

	for _, k := range sortedHelmValuesKeyIntersect(valuesSet, schemaSet) {
		nestedValues, ok := values[k].(map[string]any)
		if !ok || len(nestedValues) == 0 {
			continue
		}

		nestedSchema := schema.Properties[k]
		// Open maps and default-empty extension points need not mirror every schema option.
		if nestedSchema == nil || len(nestedSchema.Properties) == 0 {
			continue
		}

		missing, extra := compareHelmValuesSchemaKeys(nestedValues, nestedSchema, prefix+k+".")
		missingFromSchema = append(missingFromSchema, missing...)
		missingFromValues = append(missingFromValues, extra...)
	}

	return missingFromSchema, missingFromValues
}

func formatHelmValuesSchemaDiff(missingFromSchema, missingFromValues []string) string {
	var out strings.Builder
	if len(missingFromSchema) > 0 {
		out.WriteString("keys in values.yaml but missing from values.schema.json:\n")
		for _, key := range missingFromSchema {
			out.WriteString("  - ")
			out.WriteString(key)
			out.WriteString("\n")
		}
	}
	if len(missingFromValues) > 0 {
		out.WriteString("keys in values.schema.json but missing from values.yaml:\n")
		for _, key := range missingFromValues {
			out.WriteString("  - ")
			out.WriteString(key)
			out.WriteString("\n")
		}
	}
	return out.String()
}

func keysOfHelmValuesMap(values map[string]any) []string {
	out := make([]string, 0, len(values))
	for k := range values {
		out = append(out, k)
	}
	return out
}

func helmValuesKeySet(keys []string) map[string]struct{} {
	out := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		out[key] = struct{}{}
	}
	return out
}

func sortedHelmValuesKeyDiff(a, b map[string]struct{}, prefix string) []string {
	var out []string
	for k := range a {
		if _, ok := b[k]; !ok {
			out = append(out, prefix+k)
		}
	}
	slices.Sort(out)
	return out
}

func sortedHelmValuesKeyIntersect(a, b map[string]struct{}) []string {
	var out []string
	for k := range a {
		if _, ok := b[k]; ok {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

func loadHelmValues(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	out := map[string]any{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func loadHelmValuesSchema(path string) (*helmValuesSchemaNode, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var out helmValuesSchemaNode
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
