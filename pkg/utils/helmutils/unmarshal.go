package helmutils

import (
	"maps"
	"os"

	k8syamlutil "sigs.k8s.io/yaml"
)

// UnmarshalValuesFile reads a file and returns a map containing the unmarshalled values
func UnmarshalValuesFile(filePath string) (map[string]any, error) {
	mapFromFile := map[string]any{}

	bytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// NOTE: This is not the default golang yaml.Unmarshal, because that implementation
	// does not unmarshal into a map[string]interface{}; it unmarshals the file into a map[interface{}]interface{}
	// https://github.com/go-yaml/yaml/issues/139
	if err := k8syamlutil.Unmarshal(bytes, &mapFromFile); err != nil {
		return nil, err
	}

	return mapFromFile, nil
}

// MergeMaps comes from Helm internals: https://github.com/helm/helm/blob/release-3.0/pkg/cli/values/options.go#L88
func MergeMaps(a, b map[string]any) map[string]any {
	out := make(map[string]any, len(a))
	maps.Copy(out, a)
	for k, v := range b {
		if v, ok := v.(map[string]any); ok {
			if bv, ok := out[k]; ok {
				if bv, ok := bv.(map[string]any); ok {
					out[k] = MergeMaps(bv, v)
					continue
				}
			}
		}
		out[k] = v
	}
	return out
}
