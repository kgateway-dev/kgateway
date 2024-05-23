package iosnapshot

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v2"
)

func getGenericMaps(snapshot map[string]json.Marshaler) (map[string]interface{}, error) {
	genericMaps := map[string]interface{}{}
	for id, obj := range snapshot {
		jsn, err := obj.MarshalJSON()
		if err != nil {
			return nil, err
		}
		genericMap := map[string]interface{}{}
		if err := json.Unmarshal(jsn, &genericMap); err != nil {
			return nil, err
		}
		genericMaps[id] = genericMap
	}
	return genericMaps, nil
}

func formatMap(format string, genericMaps map[string]interface{}) ([]byte, error) {
	switch format {
	case "json":
		return json.MarshalIndent(genericMaps, "", "    ")
	case "", "json_compact":
		return json.Marshal(genericMaps)
	case "yaml":
		return yaml.Marshal(genericMaps)
	default:
		return nil, fmt.Errorf("invalid format of %s", format)
	}

}

func getContentType(format string) string {
	switch format {
	case "", "json", "json_compact":
		return "application/json"
	case "yaml":
		return "text/x-yaml"
	default:
		return "application/json"
	}
}
