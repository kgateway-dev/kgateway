package main

import (
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/go-utils/versionutils"
	"github.com/solo-io/go-utils/versionutils/dep"
	"github.com/solo-io/go-utils/versionutils/git"
)

func main() {
	tomlTree, err := versionutils.ParseFullToml()
	fatalCheck(err, "parsing error")

	soloKitVersion, err := versionutils.GetDependencyVersionInfo(version.SoloKitPkg, tomlTree)
	fatalCheck(err, "getting solo-kit version")

	targetVersion := soloKitVersion.Version
	if soloKitVersion.Type == dep.Version {
		// If the toml version attribute is "version", we are looking for a tag
		targetVersion = git.AppendTagPrefix(targetVersion)
	}
	fatalCheck(git.PinDependencyVersion("../solo-kit", targetVersion), "consider git fetching in solo-kit repo")
}

func fatalCheck(err error, msg string) {
	if err != nil {
		log.Fatalf("Error (%v) unable to pin repos!: %v", msg, err)
	}
}
