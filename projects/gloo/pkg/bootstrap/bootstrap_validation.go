package bootstrap

import (
	"context"
	"os"
	"os/exec"

	"github.com/solo-io/go-utils/contextutils"

	"github.com/rotisserie/eris"
)

const defaultEnvoyPath = "/usr/local/bin/envoy"

func getEnvoyPath() string {
	ep := os.Getenv("ENVOY_BINARY_PATH")
	if len(ep) == 0 {
		ep = defaultEnvoyPath
	}
	return ep
}

func ValidateBootstrap(ctx context.Context, bootstrapYaml string) error {
	envoyPath := getEnvoyPath()
	validateCmd := exec.Command(envoyPath, "--mode", "validate", "--config-yaml", bootstrapYaml)
	if output, err := validateCmd.CombinedOutput(); err != nil {
		if os.IsNotExist(err) {
			// log a warning and return nil; will allow users to continue to run Gloo locally without
			// relying on the Gloo container with Envoy already published to the expected directory
			contextutils.LoggerFrom(ctx).Warnf("Unable to validate envoy configuration using envoy at %v; "+
				"skipping additional validation of Gloo config.", envoyPath)
			return nil
		}
		return eris.Errorf("envoy validation mode output: %v, error: %v", string(output), err)
	}
	return nil
}
