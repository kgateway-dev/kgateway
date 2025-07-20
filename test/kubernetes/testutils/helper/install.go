package helper

import (
	"fmt"
	"path/filepath"

	"github.com/solo-io/go-utils/log"
	"helm.sh/helm/v3/pkg/repo"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

const (
	TestAssetDir          = "_test"
	HelmRepoIndexFileName = "index.yaml"
)

// Gets the absolute path to a locally-built helm chart. This assumes that the helm index has a reference
// to exactly one version of the helm chart.
func GetLocalChartPath(chartName string) (string, error) {
	rootDir := testutils.GitRootDirectory()
	testAssetDir := filepath.Join(rootDir, TestAssetDir)
	if !fsutils.IsDirectory(testAssetDir) {
		return "", fmt.Errorf("%s does not exist or is not a directory", testAssetDir)
	}

	version, err := getChartVersion(testAssetDir, chartName)
	if err != nil {
		return "", fmt.Errorf("getting Helm chart version: %w", err)
	}
	return filepath.Join(testAssetDir, fmt.Sprintf("%s-%s.tgz", chartName, version)), nil
}

// Parses the Helm index file and returns the version of the chart.
func getChartVersion(testAssetDir string, chartName string) (string, error) {
	// Find helm index file in test asset directory
	helmIndexPath := filepath.Join(testAssetDir, HelmRepoIndexFileName)
	helmIndex, err := repo.LoadIndexFile(helmIndexPath)
	if err != nil {
		return "", fmt.Errorf("parsing Helm index file: %w", err)
	}
	log.Printf("found Helm index file at: %s", helmIndexPath)

	// Read and return version from helm index file
	if chartVersions, ok := helmIndex.Entries[chartName]; !ok {
		return "", fmt.Errorf("index file does not contain entry with key: %s", chartName)
	} else if len(chartVersions) == 0 || len(chartVersions) > 1 {
		return "", fmt.Errorf("expected a single entry with name [%s], found: %v", chartName, len(chartVersions))
	} else {
		version := chartVersions[0].Version
		log.Printf("version of [%s] Helm chart is: %s", chartName, version)
		return version, nil
	}
}
