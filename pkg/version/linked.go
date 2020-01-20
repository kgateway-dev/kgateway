package version

import (
	"github.com/solo-io/go-utils/versionutils"
	"github.com/solo-io/go-utils/versionutils/git"
)

var UndefinedVersion = "undefined"

// This will be set by the linker during build
var Version = UndefinedVersion

func IsReleaseVersion() bool {
	if Version == UndefinedVersion {
		return false
	}
	tag := git.AppendTagPrefix(Version)
	_, err := versionutils.ParseVersion(tag)
	return err == nil
}
