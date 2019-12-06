package version

import (
	"github.com/solo-io/go-utils/githubutils"
	"github.com/spf13/afero"
	"helm.sh/helm/v3/pkg/repo"
)

const EnterpriseHelmRepoIndex = "https://storage.googleapis.com/gloo-ee-helm/index.yaml"
const GlooEE = "gloo-ee"

// The version of GlooE installed by the CLI.
// Calculated from the latest gloo-ee version in the helm repo index
func GetEnterpriseTag() (string, error) {
	fs := afero.NewOsFs()
	tmpFile, err := afero.TempFile(fs, "", "")
	if err != nil {
		return "", err
	}
	if err := githubutils.DownloadFile(EnterpriseHelmRepoIndex, tmpFile); err != nil {
		return "", err
	}
	defer fs.Remove(tmpFile.Name())
	return LatestVersionFromRepo(tmpFile.Name())
}

func LatestVersionFromRepo(file string) (string, error) {
	ind, err := repo.LoadIndexFile(file)
	if err != nil {
		return "", err
	}
	version, err := ind.Get(GlooEE, "")
	if err != nil {
		return "", err
	}
	return version.Version, nil
}
