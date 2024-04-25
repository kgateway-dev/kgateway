package setup

import (
	"context"

	"golang.org/x/sync/errgroup"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/solo-io/go-utils/contextutils"

	"github.com/solo-io/gloo/projects/gateway2/controller"
	"github.com/solo-io/gloo/projects/gateway2/extensions"
	"github.com/solo-io/gloo/projects/gateway2/proxy_syncer"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/debug"
)

// StartFunc represents a function that will be called with the initialized bootstrap.Opts
// and Extensions. This is invoked each time the setup_syncer is executed
// (which runs whenever the Setting CR is modified)
type StartFunc func(ctx context.Context, opts bootstrap.Opts, extensions Extensions) error

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
				contextutils.LoggerFrom(namedCtx).Infof("starting %s goroutine", name)
				err := startFn(namedCtx, opts, extensions)
				if err != nil {
					contextutils.LoggerFrom(namedCtx).Errorf("%s goroutine failed: %v", name, err)
				}
				return err
			},
		)
	}

	contextutils.LoggerFrom(ctx).Debug("main goroutines successfully started")
}

// K8sGatewayControllerStartFunc returns a StartFunc to run the k8s Gateway controller
func K8sGatewayControllerStartFunc(
	proxyClient v1.ProxyClient,
	k8sGatewayExtensions extensions.K8sGatewayExtensions,
	inputChannels *proxy_syncer.GatewayInputChannels,
	mgr manager.Manager,
	queueStatusForProxies proxy_syncer.QueueStatusForProxiesFn,
) StartFunc {
	return func(ctx context.Context, opts bootstrap.Opts, extensions Extensions) error {
		if opts.ProxyDebugServer.Server != nil {
			// If we have a debug server running, let's register the proxy client used by
			// the k8s gateway translation. This will enable operators to query the debug endpoint
			// and inspect the proxies that are stored in memory
			opts.ProxyDebugServer.Server.RegisterProxyReader(debug.K8sGatewayTranslation, proxyClient)
		}

		return controller.Start(ctx, controller.StartConfig{
			K8sGatewayExtensions:      k8sGatewayExtensions,
			GlooPluginRegistryFactory: extensions.PluginRegistryFactory,
			Opts:                      opts,
			Mgr:                       mgr,
			InputChannels:             inputChannels,

			ProxyClient:           proxyClient,
			QueueStatusForProxies: queueStatusForProxies,

			// Useful for development purposes
			// At the moment, this is not tied to any user-facing API
			Dev: false,
		})
	}
}
