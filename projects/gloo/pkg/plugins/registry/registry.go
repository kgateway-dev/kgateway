// Package registry is responsible for managing
package registry

import (
	"context"
	"os"
	"strings"

	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/als"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/aws"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/aws/ec2"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/azure"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/basicroute"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/buffer"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/connection_limit"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/consul"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/cors"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/csrf"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/deprecated_cipher_passthrough"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/dynamic_forward_proxy"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/enterprise_warning"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/extauth"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/faultinjection"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/grpc"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/grpcjson"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/grpcweb"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/gzip"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/headers"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/healthcheck"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/istio_integration"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/linkerd"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/listener"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/loadbalancer"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/local_ratelimit"
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
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
)

var (
	_ plugins.PluginRegistry = new(pluginRegistry)
)

func Plugins(opts bootstrap.Opts) []plugins.Plugin {
	var glooPlugins []plugins.Plugin

	ec2Plugin, err := ec2.NewPlugin(opts.WatchOpts.Ctx, opts.Secrets)
	if err != nil {
		contextutils.LoggerFrom(opts.WatchOpts.Ctx).Errorf("Failed to create ec2 Plugin %+v", err)
	}

	glooPlugins = append(glooPlugins,
		loadbalancer.NewPlugin(),
		upstreamconn.NewPlugin(),
		azure.NewPlugin(),
		aws.NewPlugin(aws.GenerateAWSLambdaRouteConfig),
		rest.NewPlugin(),
		hcm.NewPlugin(),
		als.NewPlugin(),
		proxyprotocol.NewPlugin(),
		tls_inspector.NewPlugin(),
		pipe.NewPlugin(),
		tcp.NewPlugin(utils.NewSslConfigTranslator()),
		connection_limit.NewPlugin(),
		static.NewPlugin(),
		transformation.NewPlugin(),
		grpcweb.NewPlugin(),
		grpc.NewPlugin(),
		faultinjection.NewPlugin(),
		basicroute.NewPlugin(),
		cors.NewPlugin(),
		linkerd.NewPlugin(),
		stats.NewPlugin(),
		ec2Plugin,
		tracing.NewPlugin(),
		shadowing.NewPlugin(),
		headers.NewPlugin(),
		healthcheck.NewPlugin(),
		extauth.NewPlugin(),
		ratelimit.NewPlugin(),
		gzip.NewPlugin(),
		buffer.NewPlugin(),
		csrf.NewPlugin(),
		listener.NewPlugin(),
		virtualhost.NewPlugin(),
		protocoloptions.NewPlugin(),
		grpcjson.NewPlugin(),
		metadata.NewPlugin(),
		tunneling.NewPlugin(),
		dynamic_forward_proxy.NewPlugin(),
		deprecated_cipher_passthrough.NewPlugin(),
		local_ratelimit.NewPlugin(),
	)

	if opts.KubeClient != nil {
		glooPlugins = append(glooPlugins, kubernetes.NewPlugin(opts.KubeClient, opts.KubeCoreCache))
	}
	if opts.Consul.ConsulWatcher != nil {
		glooPlugins = append(glooPlugins, consul.NewPlugin(opts.Consul.ConsulWatcher, consul.NewConsulDnsResolver(opts.Consul.DnsServer), opts.Consul.DnsPollingInterval))
	}
	lookupResult, found := os.LookupEnv("ENABLE_ISTIO_INTEGRATION")
	istioEnabled := found && strings.ToLower(lookupResult) == "true"
	if istioEnabled {
		istioPlugin := istio_integration.NewPlugin(opts.WatchOpts.Ctx)
		glooPlugins = append(glooPlugins, istioPlugin)
	}
	return glooPlugins
}

func GetPluginRegistryFactory(opts bootstrap.Opts) plugins.PluginRegistryFactory {
	return func(ctx context.Context) plugins.PluginRegistry {
		availablePlugins := Plugins(opts)

		// To improve the UX, load a plugin that warns users if they are attempting to use enterprise configuration
		availablePlugins = append(availablePlugins, enterprise_warning.NewPlugin())
		return NewPluginRegistry(availablePlugins)
	}
}

type pluginRegistry struct {
	plugins                      []plugins.Plugin
	listenerPlugins              []plugins.ListenerPlugin
	tcpFilterChainPlugins        []plugins.TcpFilterChainPlugin
	networkFilterPlugins         []plugins.NetworkFilterPlugin
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

// NewPluginRegistry creates a plugin registry and places all registered plugins
// into their appropriate plugin lists. This process is referred to as
// registering the plugins.
func NewPluginRegistry(registeredPlugins []plugins.Plugin) *pluginRegistry {
	var allPlugins []plugins.Plugin
	var listenerPlugins []plugins.ListenerPlugin
	var tcpFilterChainPlugins []plugins.TcpFilterChainPlugin
	var httpFilterPlugins []plugins.HttpFilterPlugin
	var networkFilterPlugins []plugins.NetworkFilterPlugin
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
		if plugin == nil {
			continue
		}
		allPlugins = append(allPlugins, plugin)

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

		networkFilterPlugin, ok := plugin.(plugins.NetworkFilterPlugin)
		if ok {
			networkFilterPlugins = append(networkFilterPlugins, networkFilterPlugin)
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
		plugins:                      allPlugins,
		listenerPlugins:              listenerPlugins,
		tcpFilterChainPlugins:        tcpFilterChainPlugins,
		networkFilterPlugins:         networkFilterPlugins,
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

// GetPlugins returns the plugins that were registered within the registery.
func (p *pluginRegistry) GetPlugins() []plugins.Plugin {
	return p.plugins
}

// GetListenerPlugins returns the plugins that were registered which act on Listener.
func (p *pluginRegistry) GetListenerPlugins() []plugins.ListenerPlugin {
	return p.listenerPlugins
}

// GetTcpFilterChainPlugins returns the plugins that were registered which act on TcpFilterChain.
func (p *pluginRegistry) GetTcpFilterChainPlugins() []plugins.TcpFilterChainPlugin {
	return p.tcpFilterChainPlugins
}

// GetNetworkFilterPlugins returns the plugins that were registered which act on NetworkFilter.
func (p *pluginRegistry) GetNetworkFilterPlugins() []plugins.NetworkFilterPlugin {
	return p.networkFilterPlugins
}

// GetHttpFilterPlugins returns the plugins that were registered which act on HttpFilter.
func (p *pluginRegistry) GetHttpFilterPlugins() []plugins.HttpFilterPlugin {
	return p.httpFilterPlugins
}

// GetHttpConnectionManagerPlugins returns the plugins that were registered which act on HttpConnectionManager.
func (p *pluginRegistry) GetHttpConnectionManagerPlugins() []plugins.HttpConnectionManagerPlugin {
	return p.httpConnectionManagerPlugins
}

// GetVirtualHostPlugins returns the plugins that were registered which act on VirtualHost.
func (p *pluginRegistry) GetVirtualHostPlugins() []plugins.VirtualHostPlugin {
	return p.virtualHostPlugins
}

// GetResourceGeneratorPlugins returns the plugins that were registered which act on ResourceGenerator.
func (p *pluginRegistry) GetResourceGeneratorPlugins() []plugins.ResourceGeneratorPlugin {
	return p.resourceGeneratorPlugins
}

// GetUpstreamPlugins returns the plugins that were registered which act on Upstream.
func (p *pluginRegistry) GetUpstreamPlugins() []plugins.UpstreamPlugin {
	return p.upstreamPlugins
}

// GetEndpointPlugins returns the plugins that were registered which act on Endpoint.
func (p *pluginRegistry) GetEndpointPlugins() []plugins.EndpointPlugin {
	return p.endpointPlugins
}

// GetRoutePlugins returns the plugins that were registered which act on Route.
func (p *pluginRegistry) GetRoutePlugins() []plugins.RoutePlugin {
	return p.routePlugins
}

// GetRouteActionPlugins returns the plugins that were registered which act on RouteAction.
func (p *pluginRegistry) GetRouteActionPlugins() []plugins.RouteActionPlugin {
	return p.routeActionPlugins
}

// GetWeightedDestinationPlugins returns the plugins that were registered which act on WeightedDestination.
func (p *pluginRegistry) GetWeightedDestinationPlugins() []plugins.WeightedDestinationPlugin {
	return p.weightedDestinationPlugins
}
