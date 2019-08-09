package settings

import (
	"os"
)

func SkipCrdCreation() bool {
	return os.Getenv("AUTO_CREATE_CRDS") != "1"
}
