package version

import (
	"github.com/solo-io/go-utils/versionutils"
	"math"
)

const GlooFedHelmRepoIndex = "https://storage.googleapis.com/gloo-fed-helm/index.yaml"
const GlooFed = "gloo-fed"

// The version of GlooE installed by the CLI.
// Calculated from the largest semver gloo-ee version in the helm repo index
func GetLatestGlooFedVersion(stableOnly bool) (string, error) {
	return GetLatestHelmChartVersionWithMaxVersion(GlooFedHelmRepoIndex, GlooFed, stableOnly, &versionutils.Version{
		Major: math.MaxInt32,
		Minor: math.MaxInt32,
		Patch: math.MaxInt32,
	})
}