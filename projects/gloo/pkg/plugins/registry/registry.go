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
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/proxyprotocol"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/ratelimit"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/rest"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/shadowing"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/stats"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/tcp"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/tls_inspector"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/tracing"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/tunneling"
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
		azure.NewPlugin(),
		aws.NewPlugin(&transformationPlugin.RequireEarlyTransformation),
		rest.NewPlugin(),
		hcmPlugin,
		als.NewPlugin(),
		proxyprotocol.NewPlugin(),
		tls_inspector.NewPlugin(),
		pipe.NewPlugin(),
		tcp.NewPlugin(utils.NewSslConfigTranslator()),
		static.NewPlugin(),
		transformationPlugin,
		grpcweb.NewPlugin(),
		grpc.NewPlugin(),
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
		tunneling.NewPlugin(),
	)
	if opts.KubeClient != nil {
		reg.plugins = append(reg.plugins, kubernetes.NewPlugin(opts.KubeClient, opts.KubeCoreCache))
	}
	if opts.Consul.ConsulWatcher != nil {
		reg.plugins = append(reg.plugins, consul.NewPlugin(opts.Consul.ConsulWatcher, &consul.ConsulDnsResolver{DnsAddress: opts.Consul.DnsServer}, opts.Consul.DnsPollingInterval))
	}

	return reg
}

func Plugins(opts bootstrap.Opts) []plugins.Plugin {
	return globalRegistry(opts).plugins
}

var _ plugins.PluginRegistry = new(pluginRegistry)

type pluginRegistry struct {
	plugins                      []plugins.Plugin
	listenerPlugins              []plugins.ListenerPlugin
	tcpFilterChainPlugins        []plugins.TcpFilterChainPlugin
	httpFilterPlugins            []plugins.HttpFilterPlugin
	httpConnectionManagerPlugins []plugins.HttpConnectionManagerPlugin
	virtualHostPlugins           []plugins.VirtualHostPlugin
	resourceGeneratorPlugins     []plugins.ResourceGeneratorPlugin
	upstreamPlugins              []plugins.UpstreamPlugin
	endpointPlugins              []plugins.EndpointPlugin
	routePlugins                 []plugins.RoutePlugin
	routeActionPlugins           []plugins.RouteActionPlugin
	weightedDestinationPlugins   []plugins.WeightedDestinationPlugin
}

func NewPluginRegistry(registeredPlugins []plugins.Plugin) *pluginRegistry {
	var listenerPlugins []plugins.ListenerPlugin
	var tcpFilterChainPlugins []plugins.TcpFilterChainPlugin
	var httpFilterPlugins []plugins.HttpFilterPlugin
	var httpConnectionManagerPlugins []plugins.HttpConnectionManagerPlugin
	var virtualHostPlugins []plugins.VirtualHostPlugin
	var resourceGeneratorPlugins []plugins.ResourceGeneratorPlugin
	var upstreamPlugins []plugins.UpstreamPlugin
	var endpointPlugins []plugins.EndpointPlugin
	var routePlugins []plugins.RoutePlugin
	var routeActionPlugins []plugins.RouteActionPlugin
	var weightedDestinationPlugins []plugins.WeightedDestinationPlugin

	// Process registered plugins once
	for _, plugin := range registeredPlugins {
		listenerPlugin, ok := plugin.(plugins.ListenerPlugin)
		if ok {
			listenerPlugins = append(listenerPlugins, listenerPlugin)
		}

		tcpFilterChainPlugin, ok := plugin.(plugins.TcpFilterChainPlugin)
		if ok {
			tcpFilterChainPlugins = append(tcpFilterChainPlugins, tcpFilterChainPlugin)
		}

		httpFilterPlugin, ok := plugin.(plugins.HttpFilterPlugin)
		if ok {
			httpFilterPlugins = append(httpFilterPlugins, httpFilterPlugin)
		}

		httpConnectionManagerPlugin, ok := plugin.(plugins.HttpConnectionManagerPlugin)
		if ok {
			httpConnectionManagerPlugins = append(httpConnectionManagerPlugins, httpConnectionManagerPlugin)
		}

		virtualHostPlugin, ok := plugin.(plugins.VirtualHostPlugin)
		if ok {
			virtualHostPlugins = append(virtualHostPlugins, virtualHostPlugin)
		}

		resourceGeneratorPlugin, ok := plugin.(plugins.ResourceGeneratorPlugin)
		if ok {
			resourceGeneratorPlugins = append(resourceGeneratorPlugins, resourceGeneratorPlugin)
		}

		upstreamPlugin, ok := plugin.(plugins.UpstreamPlugin)
		if ok {
			upstreamPlugins = append(upstreamPlugins, upstreamPlugin)
		}

		endpointPlugin, ok := plugin.(plugins.EndpointPlugin)
		if ok {
			endpointPlugins = append(endpointPlugins, endpointPlugin)
		}

		routePlugin, ok := plugin.(plugins.RoutePlugin)
		if ok {
			routePlugins = append(routePlugins, routePlugin)
		}

		routeActionPlugin, ok := plugin.(plugins.RouteActionPlugin)
		if ok {
			routeActionPlugins = append(routeActionPlugins, routeActionPlugin)
		}

		weightedDestinationPlugin, ok := plugin.(plugins.WeightedDestinationPlugin)
		if ok {
			weightedDestinationPlugins = append(weightedDestinationPlugins, weightedDestinationPlugin)
		}
	}

	return &pluginRegistry{
		plugins:                      registeredPlugins,
		listenerPlugins:              listenerPlugins,
		tcpFilterChainPlugins:        tcpFilterChainPlugins,
		httpFilterPlugins:            httpFilterPlugins,
		httpConnectionManagerPlugins: httpConnectionManagerPlugins,
		virtualHostPlugins:           virtualHostPlugins,
		resourceGeneratorPlugins:     resourceGeneratorPlugins,
		upstreamPlugins:              upstreamPlugins,
		endpointPlugins:              endpointPlugins,
		routePlugins:                 routePlugins,
		routeActionPlugins:           routeActionPlugins,
		weightedDestinationPlugins:   weightedDestinationPlugins,
	}
}

func (p *pluginRegistry) GetPlugins() []plugins.Plugin {
	return p.plugins
}

func (p *pluginRegistry) GetListenerPlugins() []plugins.ListenerPlugin {
	return p.listenerPlugins
}

func (p *pluginRegistry) GetTcpFilterChainPlugins() []plugins.TcpFilterChainPlugin {
	return p.tcpFilterChainPlugins
}

func (p *pluginRegistry) GetHttpFilterPlugins() []plugins.HttpFilterPlugin {
	return p.httpFilterPlugins
}

func (p *pluginRegistry) GetHttpConnectionManagerPlugins() []plugins.HttpConnectionManagerPlugin {
	return p.httpConnectionManagerPlugins
}

func (p *pluginRegistry) GetVirtualHostPlugins() []plugins.VirtualHostPlugin {
	return p.virtualHostPlugins
}

func (p *pluginRegistry) GetResourceGeneratorPlugins() []plugins.ResourceGeneratorPlugin {
	return p.resourceGeneratorPlugins
}

func (p *pluginRegistry) GetUpstreamPlugins() []plugins.UpstreamPlugin {
	return p.upstreamPlugins
}

func (p *pluginRegistry) GetEndpointPlugins() []plugins.EndpointPlugin {
	return p.endpointPlugins
}

func (p *pluginRegistry) GetRoutePlugins() []plugins.RoutePlugin {
	return p.routePlugins
}

func (p *pluginRegistry) GetRouteActionPlugins() []plugins.RouteActionPlugin {
	return p.routeActionPlugins
}

func (p *pluginRegistry) GetWeightedDestinationPlugins() []plugins.WeightedDestinationPlugin {
	return p.weightedDestinationPlugins
}
