package options

import (
	"log"
	"strings"
)

type InputMapStringString struct {
	Entries []string `json:"values"`
}

func (m *InputMapStringString) MustMap() map[string]string {
	// check nil since this can be called on optional values
	if m == nil {
		return nil
	}
	goMap := make(map[string]string)

	for _, val := range m.Entries {
		parts := strings.SplitN(val, "=", 2)

		if len(parts) != 2 {
			log.Fatalf("'%v': invalid key-value format. must be KEY=VALUE", val)
		}
		key, value := parts[0], parts[1]
		goMap[key] = value
	}
	return goMap
}
