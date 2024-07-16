package iosnapshot

import (
	"encoding/json"
	"fmt"
)

func formatOutput(format string, genericOutput interface{}) ([]byte, error) {
	switch format {
	case "json":
		return json.MarshalIndent(genericOutput, "", "    ")
	case "", "json_compact":
		return json.Marshal(genericOutput)
	case "yaml":
		// There may be a case in the future, where yaml formatting is necessary
		// Since it is not required yet, we do not add support
		return nil, fmt.Errorf("%s format is not yet supported", format)
	default:
		return nil, fmt.Errorf("invalid format of %s", format)
	}

}
