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
	return Version != UndefinedVersion
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
