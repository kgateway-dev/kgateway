package version

import (
	"github.com/pkg/errors"
	"github.com/solo-io/go-utils/versionutils"
	"github.com/solo-io/solo-kit/pkg/utils/log"
)

func CheckVersions() error {
	log.Printf("Checking expected solo kit and gloo versions...")
	tomlTree, err := versionutils.ParseToml()
	if err != nil {
		return err
	}

	expectedSoloKitVersion, err := versionutils.GetVersion(versionutils.SoloKitPkg, tomlTree)
	if err != nil {
		return err
	}

	log.Printf("Checking repo versions...")
	actualSoloKitVersion, err := versionutils.GetGitVersion("../solo-kit")
	if err != nil {
		return err
	}
	expectedTaggedSoloKitVersion := versionutils.GetTag(expectedSoloKitVersion)
	if expectedTaggedSoloKitVersion != actualSoloKitVersion {
		return errors.Errorf("Expected solo kit version %s, found solo kit version %s in repo. Run 'make pin-repos' or fix manually.", expectedTaggedSoloKitVersion, actualSoloKitVersion)
	}
	log.Printf("Versions are pinned correctly.")
	return nil
}
