package version

import (
	"math"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/rotisserie/eris"
	"github.com/solo-io/go-utils/githubutils"
	"github.com/solo-io/go-utils/versionutils"
	"github.com/spf13/afero"
	"helm.sh/helm/v3/pkg/repo"
)

const EnterpriseHelmRepoIndex = "https://storage.googleapis.com/gloo-ee-helm/index.yaml"
const GlooEE = "gloo-ee"

// The version of GlooE installed by the CLI.
// Calculated from the largest semver gloo-ee version in the helm repo index
func GetLatestEnterpriseVersion(stableOnly bool) (string, error) {
	return GetLatestEnterpriseVersionWithMaxVersion(stableOnly, &versionutils.Version{
		Major:            math.MaxInt32,
		Minor:            math.MaxInt32,
		Patch:            math.MaxInt32,
		ReleaseCandidate: math.MaxInt32,
	})
}

// Calculated from the largest gloo-ee version in the helm repo index with version constraints
func GetLatestEnterpriseVersionWithMaxVersion(stableOnly bool, maxVersion *versionutils.Version) (string, error) {
	fs := afero.NewOsFs()
	tmpFile, err := afero.TempFile(fs, "", "")
	if err != nil {
		return "", err
	}
	if err := githubutils.DownloadFile(EnterpriseHelmRepoIndex, tmpFile); err != nil {
		return "", err
	}
	defer fs.Remove(tmpFile.Name())
	return LatestVersionFromRepoWithMaxVersion(tmpFile.Name(), stableOnly, maxVersion)
}

func LatestVersionFromRepo(file string, stableOnly bool) (string, error) {
	return LatestVersionFromRepoWithMaxVersion(file, stableOnly, &versionutils.Version{
		Major:            math.MaxInt32,
		Minor:            math.MaxInt32,
		Patch:            math.MaxInt32,
		ReleaseCandidate: math.MaxInt32,
	})
}

func LatestVersionFromRepoWithMaxVersion(file string, stableOnly bool, maxVersion *versionutils.Version) (string, error) {
	ind, err := repo.LoadIndexFile(file)
	if err != nil {
		return "", err
	}

	// we can't depend on ind.SortEntries() because this doesn't properly sort rc releases
	// e.g., it would sort 1.0.0-rc1, 1.0.0-rc10, 1.0.0-rc2, ... 1.0.0-rc9, which is incorrect
	// instead, we use our version comparison logic to get the largest tag
	largestVersion := &versionutils.Zero
	var largestTag string
	if chartVersions, ok := ind.Entries[GlooEE]; ok && len(chartVersions) > 0 {
		for _, chartVersion := range chartVersions {

			if stableOnly {
				stableOnlyConstraint, _ := semver.NewConstraint("*")
				test, err := semver.NewVersion(chartVersion.Version)
				if err != nil || !stableOnlyConstraint.Check(test) {
					continue
				}
			}

			tag := "v" + strings.TrimPrefix(chartVersion.Version, "v")
			version, err := versionutils.ParseVersion(tag)
			if err != nil {
				continue
			}
			versionConstraintSatisfied, err := maxVersion.IsGreaterThanOrEqualTo(version)
			if err != nil || !versionConstraintSatisfied {
				continue
			}

			isLargest, err := version.IsGreaterThanOrEqualTo(largestVersion)
			if err == nil && isLargest {
				largestVersion = version
				largestTag = chartVersion.Version
			}

		}
	}

	if largestTag == "" {
		return "", eris.Errorf("Couldn't find any %s versions in index file %s that satisfies constraints: [stable]: %v, [maxVersion]: %v",
			GlooEE, file, stableOnly, maxVersion)
	}

	return largestTag, nil
}
