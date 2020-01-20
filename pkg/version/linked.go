package version

import (
	"bytes"
	"os/exec"
	"strings"

	"github.com/rotisserie/eris"
)

var UndefinedVersion = "undefined"

// This will be set by the linker during build
var Version = UndefinedVersion

var InvalidVersionError = func(err error) error {
	return eris.Wrapf(err, "invalid version")
}

func IsReleaseVersion() bool {
	if Version == UndefinedVersion {
		return false
	}
	// if not a tagged release, linked version will look like: 1.3.2-8-gc032db6d8
	// thus if we can split the version into more than one part, then it is not a release version
	parts := strings.Split(Version, "-")
	return len(parts) == 1
}

// VersionFromGitDescribe is the canonical means of deriving
func VersionFromGitDescribe() (string, error) {
	cmd := exec.Command("git", "describe", "--tags", "--dirty", "--always")
	outBuf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})
	cmd.Stdout = outBuf
	cmd.Stderr = errBuf
	err := cmd.Run()
	if err != nil {
		return "", InvalidVersionError(err)
	}
	return strings.TrimSpace(outBuf.String()), nil
}
