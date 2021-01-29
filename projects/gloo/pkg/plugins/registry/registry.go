package registry

import (
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/als"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/aws"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/aws/ec2"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/azure"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/basicroute"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/buffer"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/consul"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/cors"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/csrf"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/extauth"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/faultinjection"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/grpc"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/grpcjson"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/grpcweb"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/gzip"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/headers"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/healthcheck"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/linkerd"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/listener"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/loadbalancer"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/metadata"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pipe"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/protocoloptions"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/ratelimit"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/rest"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/shadowing"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/stats"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/tcp"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/tracing"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/upstreamconn"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/virtualhost"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/wasm"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

type registry struct {
	plugins []plugins.Plugin
}

var globalRegistry = func(opts bootstrap.Opts, pluginExtensions ...func() plugins.Plugin) *registry {
	transformationPlugin := transformation.NewPlugin()
	hcmPlugin := hcm.NewPlugin()
	reg := &registry{}
	// plugins should be added here
	reg.plugins = append(reg.plugins,
		loadbalancer.NewPlugin(),
		upstreamconn.NewPlugin(),
		azure.NewPlugin(&transformationPlugin.RequireTransformationFilter),
		aws.NewPlugin(&transformationPlugin.RequireTransformationFilter),
		rest.NewPlugin(&transformationPlugin.RequireTransformationFilter),
		hcmPlugin,
		als.NewPlugin(),
		pipe.NewPlugin(),
		tcp.NewPlugin(utils.NewSslConfigTranslator()),
		static.NewPlugin(),
		transformationPlugin,
		grpcweb.NewPlugin(),
		grpc.NewPlugin(&transformationPlugin.RequireTransformationFilter),
		faultinjection.NewPlugin(),
		basicroute.NewPlugin(),
		cors.NewPlugin(),
		linkerd.NewPlugin(),
		stats.NewPlugin(),
		ec2.NewPlugin(opts.WatchOpts.Ctx, opts.Secrets),
		tracing.NewPlugin(),
		shadowing.NewPlugin(),
		headers.NewPlugin(),
		healthcheck.NewPlugin(),
		extauth.NewCustomAuthPlugin(),
		ratelimit.NewPlugin(),
		wasm.NewPlugin(),
		gzip.NewPlugin(),
		buffer.NewPlugin(),
		csrf.NewPlugin(),
		listener.NewPlugin(),
		virtualhost.NewPlugin(),
		protocoloptions.NewPlugin(),
		grpcjson.NewPlugin(),
		metadata.NewPlugin(),
	)
	if opts.KubeClient != nil {
		reg.plugins = append(reg.plugins, kubernetes.NewPlugin(opts.KubeClient, opts.KubeCoreCache))
	}
	if opts.Consul.ConsulWatcher != nil {
		reg.plugins = append(reg.plugins, consul.NewPlugin(opts.Consul.ConsulWatcher, &consul.ConsulDnsResolver{DnsAddress: opts.Consul.DnsServer}, opts.Consul.DnsPollingInterval))
	}
	hcmPlugin.RegisterHcmPlugins(reg.plugins)

	return reg
}

func Plugins(opts bootstrap.Opts) []plugins.Plugin {
	return globalRegistry(opts).plugins
}
