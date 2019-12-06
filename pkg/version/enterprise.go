package version

import (
	"github.com/solo-io/go-utils/githubutils"
	"github.com/spf13/afero"
	"helm.sh/helm/v3/pkg/repo"
)

// The version of GlooE installed by the CLI
// This will be set by the linker during build
var EnterpriseTag = GetEnterpriseTag()

var EnterpriseHelmRepoIndex = "https://storage.googleapis.com/gloo-ee-helm/index.yaml"

// Calculate the latest gloo-ee version from the helm repo index, using the helm library
func GetEnterpriseTag() string {
	fs := afero.NewOsFs()
	tmpFile, err := afero.TempFile(fs, "", "")
	if err := githubutils.DownloadFile(EnterpriseHelmRepoIndex, tmpFile); err != nil {
		return UndefinedVersion
	}
	defer fs.Remove(tmpFile.Name())
	ind, err := repo.LoadIndexFile(tmpFile.Name())
	if err != nil {
		return UndefinedVersion
	}
	version, err := ind.Get("gloo-ee", "")
	if err != nil {
		return UndefinedVersion
	}
	return version.Version
}
