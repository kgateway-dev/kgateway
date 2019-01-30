package main

import (
	"github.com/solo-io/go-utils/versionutils"
	"github.com/solo-io/solo-kit/pkg/utils/log"
)

func main() {
	tomlTree, err := version.ParseToml()
	if err != nil {
		fatal(err)
	}

	soloKitVersion, err := version.GetVersion(version.SoloKitPkg, tomlTree)
	if err != nil {
		fatal(err)
	}
	err = version.PinGitVersion("../solo-kit", soloKitVersion)
	if err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	log.Fatalf("unable to pin repos!: %v", err)
}