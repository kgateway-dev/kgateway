package envtestassets

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// GetEnvTestAssetsDir returns the envtest binary assets directory.
// It honors KUBEBUILDER_ASSETS when set, otherwise it uses the repo's
// envtest-path make target to locate or install the matching binaries.
func GetEnvTestAssetsDir() (string, error) {
	assetsDir := os.Getenv("KUBEBUILDER_ASSETS")
	if assetsDir == "" {
		out, err := exec.Command("sh", "-c", "make -s --no-print-directory -C $(dirname $(go env GOMOD)) envtest-path").CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("failed to resolve envtest assets directory: %w: %s", err, strings.TrimSpace(string(out)))
		}
		assetsDir = strings.TrimSpace(string(out))
	}
	if assetsDir == "" {
		return "", fmt.Errorf("envtest assets directory is empty")
	}

	info, err := os.Stat(assetsDir)
	if err != nil {
		return "", fmt.Errorf("envtest assets directory does not exist: %s: %w", assetsDir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("envtest assets path is not a directory: %s", assetsDir)
	}

	return assetsDir, nil
}
