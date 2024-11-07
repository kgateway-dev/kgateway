package validation

import (
	"context"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils/envoyutils/bootstrap"
	"github.com/solo-io/gloo/pkg/utils/envutils"
	"github.com/solo-io/gloo/projects/envoyinit/pkg/runner"
	"github.com/solo-io/gloo/projects/gloo/constants"
	"github.com/solo-io/go-utils/contextutils"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
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

// ValidateSnapshot accepts an xDS snapshot, clones it, and does the necessary
// conversions to imitate the same config being provided as static bootsrap config to
// Envoy, then executes Envoy in validate mode to ensure the config is valid.
// This is necessary as some configurations that work in dynamic do not work in static
// and some configurations may require the context of the destination such as mounted files.
func ValidateSnapshot(
	ctx context.Context,
	snap envoycache.Snapshot,
) error {
	// THIS IS CRITICAL SO WE DO NOT INTERFERE WITH THE CONTROL PLANE.
	// The logic for converting xDS to static bootstrap mutates some of
	// the inputs, which is unacceptable when calling from translation.
	snap = snap.Clone()

	bootstrapJson, err := bootstrap.FromSnapshot(ctx, snap)
	if err != nil {
		err = eris.Wrap(err, "error converting xDS snapshot to static bootstrap")
		contextutils.LoggerFrom(ctx).Error(err)
		return err
	}

	return ValidateBootstrap(ctx, bootstrapJson)
}
