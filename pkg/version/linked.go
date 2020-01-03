package version

import (
	"bytes"
	"os/exec"
	"strings"
)

var UndefinedVersion = "undefined"

// This will be set by the linker during build
var Version = UndefinedVersion

func IsReleaseVersion() bool {
	return Version != UndefinedVersion && checkedoutAtTag()
}

func checkedoutAtTag() bool {
	version := VersionFromGitDescribe()
	parts := strings.Split(version, "-")
	return len(parts) == 1
}

const FallbackVersion = "git-describe-error" // default version set if running "make glooctl"
func VersionFromGitDescribe() string {
	cmd := exec.Command("git", "describe", "--tags", "--dirty", "--always")
	outBuf := bytes.NewBuffer([]byte{})
	errBuf := bytes.NewBuffer([]byte{})
	cmd.Stdout = outBuf
	cmd.Stderr = errBuf
	err := cmd.Run()
	if err != nil {
		return FallbackVersion
	}
	versionOutputLines := strings.Split(outBuf.String(), "\n")
	if len(versionOutputLines) != 2 {
		return FallbackVersion
	}
	return versionOutputLines[0]
}
