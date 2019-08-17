package main

import (
	"github.com/solo-io/go-utils/log"
	version "github.com/solo-io/go-utils/versionutils"
)

func main() {
	tomlTree, err := version.ParseToml()
	fatalCheck(err, "parsing error")

	soloKitVersion, err := version.GetVersion(version.SoloKitPkg, tomlTree)
	fatalCheck(err, "getting solo-kit version")

	fatalCheck(version.PinGitVersion("../solo-kit", soloKitVersion), "consider git fetching in solo-kit repo")
}

func fatalCheck(err error, msg string) {
	if err != nil {
		log.Fatalf("Error (%v) unable to pin repos!: %v", msg, err)
	}
}
