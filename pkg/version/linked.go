package version

import (
	"strings"
)

var UndefinedVersion = "undefined"

// This will be set by the linker during build
var Version = UndefinedVersion

func IsReleaseVersion() bool {
	if Version == UndefinedVersion {
		return false
	}
	// if not a tagged release, linked version will look like: 1.3.2-8-gc032db6d8
	// thus if we can split the version into more than one part, then it is not a release version
	parts := strings.Split(Version, "-")
	return len(parts) == 1
}
