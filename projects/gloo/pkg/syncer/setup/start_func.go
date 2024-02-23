package setup

import (
	"context"

	"github.com/solo-io/gloo/projects/gateway2/controller"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/go-utils/contextutils"
	"golang.org/x/sync/errgroup"
)

// StartFunc represents a function that will be called with the initialized bootstrap.Opts
// and Extensions. This is invoked each time the setup_syncer is executed
// (which runs whenever the Setting CR is modified)
type StartFunc func(opts bootstrap.Opts, extensions Extensions) error

// ExecuteAsynchronousStartFuncs accepts a collection of StartFunc inputs, and executes them within an Error Group
func ExecuteAsynchronousStartFuncs(
	ctx context.Context,
	opts bootstrap.Opts,
	extensions Extensions,
	startFuncs map[string]StartFunc,
	errorGroup *errgroup.Group,
) {
	for name, start := range startFuncs {
		startFn := start // pike
		namedCtx := contextutils.WithLogger(ctx, name)

		errorGroup.Go(
			func() error {
				contextutils.LoggerFrom(namedCtx).Debugf("starting main goroutine")
				err := startFn(opts, extensions)
				if err != nil {
					contextutils.LoggerFrom(namedCtx).Errorf("main goroutine failed: %v", err)
				}
				return err
			},
		)
	}
}

// K8sGatewayControllerStartFunc returns a StartFunc to run the k8s Gateway controller
func K8sGatewayControllerStartFunc() StartFunc {
	return func(opts bootstrap.Opts, extensions Extensions) error {
		// Run GG controller
		// TODO: These values are hard-coded, but they should be inherited from the Helm chart
		return controller.Start(controller.ControllerConfig{
			Ctx:                   opts.WatchOpts.Ctx,
			GatewayClassName:      "gloo-gateway",
			GatewayControllerName: "solo.io/gloo-gateway",
			AutoProvision:         true,

			ControlPlane: opts.ControlPlane,
		})
	}
}
