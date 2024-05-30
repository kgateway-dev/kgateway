package iosnapshot

import (
	"encoding/json"
	"fmt"

	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
)

func apiSnapshotToGenericMap(snap *v1snap.ApiSnapshot) (map[string]interface{}, error) {
	genericMap := map[string]interface{}{}

	if snap == nil {
		return genericMap, nil
	}

	jsn, err := json.Marshal(snap)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(jsn, &genericMap); err != nil {
		return nil, err
	}
	return genericMap, nil
}

func formatMap(format string, genericMaps map[string]interface{}) ([]byte, error) {
	switch format {
	case "json":
		return json.MarshalIndent(genericMaps, "", "    ")
	case "", "json_compact":
		return json.Marshal(genericMaps)
	case "yaml":
		// There may be a case in the future, where yaml formatting is necessary
		// Since it is not required yet, we do not add support
		return nil, fmt.Errorf("%s format is not yet supported", format)
	default:
		return nil, fmt.Errorf("invalid format of %s", format)
	}

}
