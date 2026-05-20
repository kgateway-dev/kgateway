//go:build e2e

package helper

import (
	"fmt"
	"os"
	"path/filepath"

	"helm.sh/helm/v3/pkg/repo"

	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/version"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

const (
	defaultTestAssetDir   = "_test"
	HelmRepoIndexFileName = "index.yaml"
	defaultHelmChartDir   = "install/helm"
)

var logger = logging.New("helper/install")

// Gets the absolute path to a locally-built helm chart.
// It prefers packaged charts from the test asset directory, but falls back to the unpackaged chart directory.
// If assetDir is an empty string, it will default to "_test".
func GetLocalChartPath(chartName string, assetDir string) (string, error) {
	return getLocalChartPath(testutils.GitRootDirectory(), chartName, assetDir)
}

func getLocalChartPath(rootDir string, chartName string, assetDir string) (string, error) {
	dir := assetDir
	if dir == "" {
		dir = defaultTestAssetDir
	}
	testAssetDir := filepath.Join(rootDir, dir)

	if _, err := os.Stat(filepath.Join(testAssetDir, HelmRepoIndexFileName)); err == nil {
		chartPath, err := getPackagedChartPath(testAssetDir, chartName)
		if err == nil {
			return chartPath, nil
		}
		logger.Info("falling back to unpackaged Helm chart", "chart", chartName, "assetDir", testAssetDir, "error", err)
	}

	chartDir := filepath.Join(rootDir, defaultHelmChartDir, chartName)
	if fsutils.IsDirectory(chartDir) {
		return chartDir, nil
	}

	return "", fmt.Errorf("could not find packaged or unpackaged Helm chart %q", chartName)
}

func getPackagedChartPath(testAssetDir string, chartName string) (string, error) {
	version, err := getChartVersion(testAssetDir, chartName)
	if err != nil {
		return "", fmt.Errorf("getting Helm chart version: %w", err)
	}
	chartPath := filepath.Join(testAssetDir, fmt.Sprintf("%s-%s.tgz", chartName, version))
	info, err := os.Stat(chartPath)
	if err != nil {
		return "", fmt.Errorf("stat packaged chart: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("packaged chart path is a directory: %s", chartPath)
	}
	return chartPath, nil
}

// LocalChartImageTagArgs returns helm args that pin image.tag to the build-time
// linker-injected version. This is needed when installing the chart from its
// source directory: Chart.yaml's appVersion does not match the locally-built
// image tags, so the controller pod would otherwise try to pull a nonexistent
// image and stay Pending. Returns nil when no usable version is available.
func LocalChartImageTagArgs() []string {
	v := version.Version
	if v == "" || v == version.UndefinedVersion {
		return nil
	}
	return []string{"--set", "image.tag=" + v}
}

// Parses the Helm index file and returns the version of the chart.
func getChartVersion(testAssetDir string, chartName string) (string, error) {
	// Find helm index file in test asset directory
	helmIndexPath := filepath.Join(testAssetDir, HelmRepoIndexFileName)
	helmIndex, err := repo.LoadIndexFile(helmIndexPath)
	if err != nil {
		return "", fmt.Errorf("parsing Helm index file: %w", err)
	}
	logger.Info("found Helm index file", "path", helmIndexPath)

	// Read and return version from helm index file
	if chartVersions, ok := helmIndex.Entries[chartName]; !ok {
		return "", fmt.Errorf("index file does not contain entry with key: %s", chartName)
	} else if len(chartVersions) == 0 || len(chartVersions) > 1 {
		return "", fmt.Errorf("expected a single entry with name [%s], found: %v", chartName, len(chartVersions))
	} else {
		version := chartVersions[0].Version
		logger.Info("version of Helm chart", "chart", chartName, "version", version)
		return version, nil
	}
}
