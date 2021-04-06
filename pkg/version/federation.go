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

	version, err := versionutils.ParseVersion(git.AppendTagPrefix(Version))
	if err != nil {
		return "", err
	}

	return GetLatestHelmChartVersionWithMaxVersion(GlooFedHelmRepoIndex, GlooFed, stableOnly, &versionutils.Version{
		Major: version.Major,
		Minor: version.Minor,
		Patch: math.MaxInt32,
	})
}
