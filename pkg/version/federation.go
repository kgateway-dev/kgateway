package version

import (
	"math"

	"github.com/solo-io/go-utils/versionutils"
	"github.com/solo-io/go-utils/versionutils/git"
)

const GlooFedHelmRepoIndex = "https://storage.googleapis.com/gloo-fed-helm/index.yaml"
const GlooFed = "gloo-fed"

// The version of GlooE installed by the CLI.
// Calculated from the largest semver gloo-ee version in the helm repo index
func GetLatestGlooFedVersion(stableOnly bool) (string, error) {

	maxVersion := &versionutils.Version{
		Major: math.MaxInt32,
		Minor: math.MaxInt32,
		Patch: math.MaxInt32,
	}

	if Version != UndefinedVersion {
		version, err := versionutils.ParseVersion(git.AppendTagPrefix(Version))
		if err != nil {
			return "", err
		}
		maxVersion.Major = version.Major
		maxVersion.Minor = version.Minor
	}

	return GetLatestHelmChartVersionWithMaxVersion(GlooFedHelmRepoIndex, GlooFed, stableOnly, maxVersion)
}
