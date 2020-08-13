package options

import (
	"log"
	"strings"
)

type InputMapStringString struct {
	Entries []string `json:"values"`
}

func (m *InputMapStringString) MustMap() map[string]string {
	return m.MustMapLax(false)
}

func (m *InputMapStringString) MustMapLax(allowSeparatorInValue bool) map[string]string {
	// check nil since this can be called on optional values
	if m == nil {
		return nil
	}
	goMap := make(map[string]string)

	var splits int
	if allowSeparatorInValue {
		splits = 2
	} else {
		splits = -1
	}

	for _, val := range m.Entries {
		parts := strings.SplitN(val, "=", splits)

		if len(parts) != 2 {
			log.Fatalf("'%v': invalid key-value format. must be KEY=VALUE", val)
		}
		key, value := parts[0], parts[1]
		goMap[key] = value
	}
	return goMap
}
