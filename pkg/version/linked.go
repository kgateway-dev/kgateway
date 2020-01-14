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

func IsReleaseVersion() (bool, error) {
	atTag, err := checkedoutAtTag()
	if err != nil {
		return false, err
	}
	return Version != UndefinedVersion && atTag, nil
}

func checkedoutAtTag() (bool, error) {
	version, err := VersionFromGitDescribe()
	if err != nil {
		return false, err
	}
	parts := strings.Split(version, "-")
	return len(parts) == 1, nil
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
