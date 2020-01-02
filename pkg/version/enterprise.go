package version

import (
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/solo-io/go-utils/errors"
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
	return GetLatestEnterpriseVersionWithMaxVersion(stableOnly, &versionutils.Zero)
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
	return LatestVersionFromRepoWithMaxVersion(file, stableOnly, &versionutils.Zero)
}

func LatestVersionFromRepoWithMaxVersion(file string, stableOnly bool, maxVersion *versionutils.Version) (string, error) {
	ind, err := repo.LoadIndexFile(file)
	if err != nil {
		return "", err
	}

	ind.SortEntries()
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
			if err == nil && versionConstraintSatisfied {
				return chartVersion.Version, nil
			}

		}
	}

	return "", errors.Errorf("Couldn't find any %s versions in index file %s", GlooEE, file)
}
