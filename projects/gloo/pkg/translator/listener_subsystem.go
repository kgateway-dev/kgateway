package translator

import (
	validationapi "github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

// The Listener subsystem (https://www.envoyproxy.io/docs/envoy/latest/intro/life_of_a_request.html?highlight=listener#high-level-architecture)
// handles downstream request processing.
// Gloo sends resources to Envoy via xDS. The components of the Listener subsystem that Gloo configures are:
// 1. Listeners
// 2. RouteConfiguration
// Given that Gloo exposes a variety of ListenerTypes (HttpListener, TcpListener, HybridListener), and each of these types
// affect how resources are generated, we abstract those implementation details behind abstract translators.
// The ListenerSubsystemTranslatorFactory returns a ListenerTranslator and RouteConfigurationTranslator for a given Gloo Listener
type ListenerSubsystemTranslatorFactory struct {
	plugins             []plugins.Plugin
	proxy               *v1.Proxy
	sslConfigTranslator utils.SslConfigTranslator
}

func NewListenerSubsystemTranslatorFactory(
	plugins []plugins.Plugin,
	proxy *v1.Proxy,
	sslConfigTranslator utils.SslConfigTranslator,
) *ListenerSubsystemTranslatorFactory {
	return &ListenerSubsystemTranslatorFactory{
		plugins:             plugins,
		proxy:               proxy,
		sslConfigTranslator: sslConfigTranslator,
	}
}

func (l *ListenerSubsystemTranslatorFactory) GetListenerSubsystemTranslators(listener *v1.Listener, listenerReport *validationapi.ListenerReport) (
	ListenerTranslator,
	RouteConfigurationTranslator,
) {
	switch listener.GetListenerType().(type) {
	case *v1.Listener_HttpListener:
		listenerTranslator := &listenerTranslatorInstance{
			listener: listener,
			report:   listenerReport,
			plugins:  l.plugins,
			filterChainTranslator: &httpFilterChainTranslator{
				plugins:             l.plugins,
				sslConfigTranslator: l.sslConfigTranslator,
				parentListener:      listener,
				listener:            listener.GetHttpListener(),
				parentReport:        listenerReport,
				report:              listenerReport.GetHttpListenerReport(),
				routeConfigName:     routeConfigName(listener),
			},
		}

		routeConfigurationTranslator := &httpRouteConfigurationTranslator{
			plugins:                  l.plugins,
			proxy:                    l.proxy,
			parentListener:           listener,
			listener:                 listener.GetHttpListener(),
			parentReport:             listenerReport,
			report:                   listenerReport.GetHttpListenerReport(),
			routeConfigName:          routeConfigName(listener),
			requireTlsOnVirtualHosts: len(listener.GetSslConfigurations()) > 0,
		}

		return listenerTranslator, routeConfigurationTranslator

	case *v1.Listener_TcpListener:
		listenerTranslator := &listenerTranslatorInstance{
			listener: listener,
			report:   listenerReport,
			plugins:  l.plugins,
			filterChainTranslator: &tcpFilterChainTranslator{
				plugins:        l.plugins,
				parentListener: listener,
				listener:       listener.GetTcpListener(),
				report:         listenerReport.GetTcpListenerReport(),
			},
		}
		// A TcpListener does not produce any RouteConfiguration
		routeConfigurationTranslator := &emptyRouteConfigurationTranslator{}

		return listenerTranslator, routeConfigurationTranslator

	default:
		// This case should never occur
		return &emptyListenerTranslator{}, &emptyRouteConfigurationTranslator{}
	}
}
