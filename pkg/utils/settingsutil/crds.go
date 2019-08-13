package settingsutil

import (
	"os"
)

func GetSkipCrdCreation() bool {
	return os.Getenv("AUTO_CREATE_CRDS") != "1"
}
