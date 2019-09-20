package version

import (
	"github.com/pkg/errors"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/go-utils/versionutils"
	"github.com/solo-io/go-utils/versionutils/dep"
	gitutils "github.com/solo-io/go-utils/versionutils/git"
)

const SoloKitPkg = "github.com/solo-io/solo-kit"

var attributeTypes = map[dep.VersionType]string{
	dep.Version:  "version",
	dep.Branch:   "branch",
	dep.Revision: "revision",
}

func CheckVersions() error {
	tomlTree, err := versionutils.ParseFullToml()
	if err != nil {
		return err
	}

	log.Printf("Checking expected solo kit version...")
	expectedVersion, err := versionutils.GetDependencyVersionInfo(SoloKitPkg, tomlTree)
	if err != nil {
		return err
	}
	log.Printf("Expecting solo-kit with %s [%s]", attributeTypes[expectedVersion.Type], expectedVersion.Version)

	log.Printf("Checking repo versions...")
	actualVersion, err := gitutils.GetGitRefInfo("../solo-kit")
	if err != nil {
		return err
	}
	log.Printf("Found solo-kit ref. Tag [%s], Branch [%s], Commit [%s]",
		actualVersion.Tag, actualVersion.Branch, actualVersion.Hash)

	switch expectedVersion.Type {
	case dep.Version:
		expectedTaggedVersion := gitutils.AppendTagPrefix(expectedVersion.Version)
		if actualVersion.Tag != expectedTaggedVersion {
			return errors.Errorf("Expected solo kit tag [%s], found solo kit tag [%s] in repo. "+
				"Run 'make pin-repos' or fix manually.", expectedTaggedVersion, actualVersion.Tag)
		}
	case dep.Branch:
		if actualVersion.Branch != expectedVersion.Version {
			return errors.Errorf("Expected solo kit branch [%s], found solo kit branch [%s] in repo. "+
				"Run 'make pin-repos' or fix manually.", expectedVersion.Version, actualVersion.Branch)
		}
	case dep.Revision:
		if actualVersion.Hash != expectedVersion.Version {
			return errors.Errorf("Expected solo kit revision [%s], found solo kit commit [%s] in repo. "+
				"Run 'make pin-repos' or fix manually.", expectedVersion.Version, actualVersion.Hash)
		}
	default:
		return errors.Errorf("Unexpected dep version attribute type: [%d]", expectedVersion.Type)
	}

	log.Printf("Versions are pinned correctly.")
	return nil
}
