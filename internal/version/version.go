package version

import (
	"fmt"
	"runtime/debug"
)

var (
	// UndefinedVersion is the version of the kgateway controller
	// if the version is not set.
	UndefinedVersion = "undefined"
	// Version is the version of the kgateway controller.
	// This is set by the linker during build.
	Version string
	// ref is the version of the kgateway controller.
	// Constructed from the build info during init
	ref *version
)

type version struct {
	controller string
	commit     string
	date       string
}

func String() string {
	return fmt.Sprintf("controller version %s, commit %s, built at %s", ref.controller, ref.commit, ref.date)
}

func init() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		Version = UndefinedVersion
		return
	}
	v := Version
	if v == "" {
		// TODO(tim): use info.Main.Version instead of UndefinedVersion.
		v = UndefinedVersion
	}
	ref = &version{
		controller: v,
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			ref.commit = setting.Value
		case "vcs.time":
			ref.date = setting.Value
		}
	}
}
