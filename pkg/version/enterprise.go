package version

import (
	"math"

	"github.com/Masterminds/semver/v3"
	"github.com/rotisserie/eris"
	"github.com/solo-io/go-utils/githubutils"
	"github.com/solo-io/go-utils/versionutils"
	"github.com/solo-io/go-utils/versionutils/git"
	"github.com/spf13/afero"
	"helm.sh/helm/v3/pkg/repo"
)

const EnterpriseHelmRepoIndex = "https://storage.googleapis.com/gloo-ee-helm/index.yaml"
const GlooEE = "gloo-ee"

// The version of GlooE installed by the CLI.
// Calculated from the largest semver gloo-ee version in the helm repo index
func GetLatestEnterpriseVersion(stableOnly bool) (string, error) {
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

	return GetLatestHelmChartVersionWithMaxVersion(EnterpriseHelmRepoIndex, GlooEE, stableOnly, maxVersion)
}

// Calculated from the largest gloo-ee version in the helm repo index with version constraints
func GetLatestHelmChartVersionWithMaxVersion(helmRepoIndex, repoName string, stableOnly bool, maxVersion *versionutils.Version) (string, error) {
	fs := afero.NewOsFs()
	tmpFile, err := afero.TempFile(fs, "", "")
	if err != nil {
		return "", err
	}
	if err := githubutils.DownloadFile(helmRepoIndex, tmpFile); err != nil {
		return "", err
	}
	defer fs.Remove(tmpFile.Name())
	return LatestVersionFromRepoWithMaxVersion(tmpFile.Name(), repoName, stableOnly, maxVersion)
}

func LatestVersionFromRepo(file, repoName string, stableOnly bool) (string, error) {
	return LatestVersionFromRepoWithMaxVersion(file, repoName, stableOnly, &versionutils.Version{
		Major: math.MaxInt32,
		Minor: math.MaxInt32,
		Patch: math.MaxInt32,
	})
}

func LatestVersionFromRepoWithMaxVersion(file, repoName string, stableOnly bool, maxVersion *versionutils.Version) (string, error) {
	ind, err := repo.LoadIndexFile(file)
	if err != nil {
		return "", err
	}

	// we can't depend on ind.SortEntries() because this doesn't properly sort rc releases
	// e.g., it would sort 1.0.0-rc1, 1.0.0-rc10, 1.0.0-rc2, ... 1.0.0-rc9, which is incorrect
	// instead, we use our version comparison logic to get the largest tag
	zero := versionutils.Zero()
	largestVersion := &zero
	var largestTag string
	if chartVersions, ok := ind.Entries[repoName]; ok && len(chartVersions) > 0 {
		for _, chartVersion := range chartVersions {

			if stableOnly {
				stableOnlyConstraint, _ := semver.NewConstraint("*")
				test, err := semver.NewVersion(chartVersion.Version)
				if err != nil || !stableOnlyConstraint.Check(test) {
					continue
				}
			}

			version, err := versionutils.ParseVersion(git.AppendTagPrefix(chartVersion.Version))
			if err != nil {
				continue
			}

			// with the default implementation of MustIsGreaterThanOrEqualTo,
			// all rc releases will be larger than equivalent beta releases. (1.0.0-rc1 > 1.0.0-beta8)
			// since those are the only allowed labels in Gloo, this tiebreak is acceptable
			versionConstraintSatisfied := maxVersion.MustIsGreaterThanOrEqualTo(*version)
			if !versionConstraintSatisfied {
				continue
			}

			isLargest := version.MustIsGreaterThanOrEqualTo(*largestVersion)
			if isLargest {
				largestVersion = version
				largestTag = chartVersion.Version
			}

		}
	}

	if largestTag == "" {
		return "", eris.Errorf("Couldn't find any %s versions in index file %s that satisfies constraints: [stable]: %v, [maxVersion]: %v",
			repoName, file, stableOnly, maxVersion)
	}

	return largestTag, nil
}
