package syncer

import (
	"time"

	"github.com/solo-io/gloo/projects/discovery/pkg/fds"
	"github.com/solo-io/gloo/projects/discovery/pkg/fds/discoveries/aws"
	"github.com/solo-io/gloo/projects/discovery/pkg/fds/discoveries/grpc"
	"github.com/solo-io/gloo/projects/discovery/pkg/fds/discoveries/swagger"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/errutils"
)

func RunFDS(opts bootstrap.Opts) error {
	watchOpts := opts.WatchOpts.WithDefaults()
	watchOpts.Ctx = contextutils.WithLogger(watchOpts.Ctx, "fds")

	upstreamClient, err := v1.NewUpstreamClient(opts.Upstreams)
	if err != nil {
		return err
	}
	if err := upstreamClient.Register(); err != nil {
		return err
	}
	secretClient, err := v1.NewSecretClient(opts.Secrets)
	if err != nil {
		return err
	}
	if err := secretClient.Register(); err != nil {
		return err
	}

	cache := v1.NewDiscoveryEmitter(upstreamClient, secretClient)

	var resolvers fds.Resolvers
	for _, plug := range registry.Plugins(opts) {
		resolver, ok := plug.(fds.Resolver)
		if ok {
			resolvers = append(resolvers, resolver)
		}
	}

	// TODO: unhardcode
	functionalPlugins := []fds.FunctionDiscoveryFactory{
		&aws.AWSLambdaFunctionDiscoveryFactory{
			PollingTime: time.Second,
		},
		&swagger.SwaggerFunctionDiscoveryFactory{
			DetectionTimeout: time.Minute,
			FunctionPollTime: time.Second * 15,
		},
		&grpc.FunctionDiscoveryFactory{
			DetectionTimeout: time.Minute,
			FunctionPollTime: time.Second * 15,
		},
	}

	// TODO(yuval-k): max Concurrency here
	updater := fds.NewUpdater(watchOpts.Ctx, resolvers, upstreamClient, 0, functionalPlugins)
	disc := fds.NewFunctionDiscovery(updater)

	sync := NewDiscoverySyncer(disc)
	eventLoop := v1.NewDiscoveryEventLoop(cache, sync)

	errs := make(chan error)

	eventLoopErrs, err := eventLoop.Run(opts.WatchNamespaces, watchOpts)
	if err != nil {
		return err
	}
	go errutils.AggregateErrs(watchOpts.Ctx, errs, eventLoopErrs, "event_loop.fds")

	logger := contextutils.LoggerFrom(watchOpts.Ctx)

	go func() {

		for {
			select {
			case err := <-errs:
				logger.Errorf("error: %v", err)
			case <-watchOpts.Ctx.Done():
				return
			}
		}
	}()
	return nil
}
