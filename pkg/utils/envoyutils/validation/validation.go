package validation

import (
	"context"

	"github.com/solo-io/gloo/pkg/utils/envutils"
	"github.com/solo-io/gloo/projects/envoyinit/pkg/runner"
	"github.com/solo-io/gloo/projects/gloo/constants"
)

const (
	// defaultEnvoyPath is derived from where our Gloo pod has Envoy. Gloo is built with
	// our Envoy wrapper as a base image, which itself contains the Envoy binary at
	// this path. See the following files for more info on how this is built.
	// projects/gloo/cmd/Dockerfile
	// projects/envoyinit/cmd/Dockerfile.envoyinit
	// https://github.com/solo-io/envoy-gloo/blob/v1.30.4-patch5/ci/Dockerfile
	defaultEnvoyPath = "/usr/local/bin/envoy"
)

func ValidateBootstrap(ctx context.Context, bootstrap string) error {
	return runner.RunEnvoyValidate(ctx, envutils.GetOrDefault(constants.EnvoyBinaryEnv, defaultEnvoyPath, false), bootstrap)
}
