package translator

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"

	validationapi "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
)

// The Listener subsystem handles downstream request processing.
// https://www.envoyproxy.io/docs/envoy/latest/intro/life_of_a_request.html?#high-level-architecture
// Gloo sends resources to Envoy via xDS. The components of the Listener subsystem that Gloo configures are:
// 1. Listeners
// 2. RouteConfiguration
// Given that Gloo exposes a variety of ListenerTypes (HttpListener, TcpListener, HybridListener), and each of these types
// affect how resources are generated, we abstract those implementation details behind abstract translators.
// The ListenerSubsystemTranslatorFactory returns a ListenerTranslator and RouteConfigurationTranslator for a given Gloo Listener
type ListenerSubsystemTranslatorFactory struct {
	pluginRegistry      plugins.PluginRegistry
	proxy               *v1.Proxy
	sslConfigTranslator utils.SslConfigTranslator
}

func NewListenerSubsystemTranslatorFactory(
	pluginRegistry plugins.PluginRegistry,
	proxy *v1.Proxy,
	sslConfigTranslator utils.SslConfigTranslator,
) *ListenerSubsystemTranslatorFactory {
	return &ListenerSubsystemTranslatorFactory{
		pluginRegistry:      pluginRegistry,
		proxy:               proxy,
		sslConfigTranslator: sslConfigTranslator,
	}
}

func (l *ListenerSubsystemTranslatorFactory) GetTranslators(ctx context.Context, listener *v1.Listener, listenerReport *validationapi.ListenerReport) (
	ListenerTranslator,
	RouteConfigurationTranslator,
) {
	switch listener.GetListenerType().(type) {
	case *v1.Listener_HttpListener:

		return l.GetHttpListenerTranslators(ctx, listener, listenerReport)

	case *v1.Listener_TcpListener:
		return l.GetTcpListenerTranslators(ctx, listener, listenerReport)

	default:
		// This case should never occur
		return &emptyListenerTranslator{}, &emptyRouteConfigurationTranslator{}
	}
}

func (l *ListenerSubsystemTranslatorFactory) GetHttpListenerTranslators(ctx context.Context, listener *v1.Listener, listenerReport *validationapi.ListenerReport) (
	ListenerTranslator,
	RouteConfigurationTranslator,
) {
	httpListenerReport := listenerReport.GetHttpListenerReport()
	if httpListenerReport == nil {
		contextutils.LoggerFrom(ctx).DPanic("internal error: listener report was not http type")
	}

	routeConfigurationName := routeConfigName(listener)

	listenerTranslator := &listenerTranslatorInstance{
		listener: listener,
		report:   listenerReport,
		plugins:  l.pluginRegistry.GetListenerPlugins(),
		filterChainTranslator: &sslDuplicatedFilterChainTranslator{
			parentReport: listenerReport,
			networkFilterTranslator: &httpNetworkFilterTranslator{
				plugins:         l.pluginRegistry.GetHttpFilterPlugins(),
				listener:        listener.GetHttpListener(),
				report:          httpListenerReport,
				routeConfigName: routeConfigurationName,
			},
			sslConfigTranslator: l.sslConfigTranslator,
			sslConfigurations:   mergeSslConfigs(listener.GetSslConfigurations()),
		},
	}

	routeConfigurationTranslator := &httpRouteConfigurationTranslator{
		plugins:                  l.pluginRegistry.GetPlugins(),
		proxy:                    l.proxy,
		parentListener:           listener,
		listener:                 listener.GetHttpListener(),
		parentReport:             listenerReport,
		report:                   httpListenerReport,
		routeConfigName:          routeConfigurationName,
		requireTlsOnVirtualHosts: len(listener.GetSslConfigurations()) > 0,
	}

	return listenerTranslator, routeConfigurationTranslator
}

func (l *ListenerSubsystemTranslatorFactory) GetTcpListenerTranslators(ctx context.Context, listener *v1.Listener, listenerReport *validationapi.ListenerReport) (
	ListenerTranslator,
	RouteConfigurationTranslator,
) {
	tcpListenerReport := listenerReport.GetTcpListenerReport()
	if tcpListenerReport == nil {
		contextutils.LoggerFrom(ctx).DPanic("internal error: listener report was not tcp type")
	}

	listenerTranslator := &listenerTranslatorInstance{
		listener: listener,
		report:   listenerReport,
		plugins:  l.pluginRegistry.GetListenerPlugins(),
		filterChainTranslator: &tcpFilterChainTranslator{
			plugins:        l.pluginRegistry.GetTcpFilterChainPlugins(),
			parentListener: listener,
			listener:       listener.GetTcpListener(),
			report:         tcpListenerReport,
		},
	}

	// A TcpListener does not produce any RouteConfiguration
	routeConfigurationTranslator := &emptyRouteConfigurationTranslator{}

	return listenerTranslator, routeConfigurationTranslator
}
