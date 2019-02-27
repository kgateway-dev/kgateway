package kube2e

import (
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/go-utils/errors"
	"k8s.io/helm/pkg/repo"
	"path/filepath"
	"runtime"
	"strings"
)

// Returns the version identifier for the current build
func GetTestVersion() (string, error) {

	// Find helm index file in test asset directory
	helmIndex, err := repo.LoadIndexFile(filepath.Join(helpers.GlooDir(), "_test", "index.yaml"))
	if err != nil {
		return "", err
	}

	// Read and return version from helm index file
	if chartVersions, ok := helmIndex.Entries["gloo"]; !ok {
		return "", errors.Errorf("gloo chart not found")
	} else if len(chartVersions) == 0 || len(chartVersions) > 1 {
		return "", errors.Errorf("Expected one chart archive, found: %v", len(chartVersions))
	} else {
		return chartVersions[0].Version, nil
	}
}

func GlooctlInstall(namespace, version string) error {
	return helpers.RunCommand(true,
		"_output/glooctl-"+runtime.GOOS+"-amd64",
		"install",
		"gateway",
		"-n", namespace,
		"-f", strings.Join([]string{"_test/gloo-", version, ".tgz"}, ""),
		"--release", version, // TODO: will not be needed anymore
	)
}

func GlooctlUninstall(namespace string) error {
	return helpers.RunCommand(true,
		"_output/glooctl-"+runtime.GOOS+"-amd64",
		"uninstall",
		"-n", namespace,
	)
}
