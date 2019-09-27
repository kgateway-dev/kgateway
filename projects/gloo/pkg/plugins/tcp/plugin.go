package tcp

import (
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"
	usconversion "github.com/solo-io/gloo/projects/gloo/pkg/upstreams"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/errors"
	"go.uber.org/zap"
)

const (
	DefaultTcpStatPrefix = "tcp"
)

func NewPlugin() *Plugin {
	return &Plugin{}
}

var (
	_ plugins.Plugin                    = new(Plugin)
	_ plugins.ListenerFilterChainPlugin = new(Plugin)

	NoDestinationTypeError = func(host *v1.TcpHost) error {
		return errors.Errorf("no destination type was specified for tcp host %v", host)
	}

	InvalidSecretsError = func(err error, name string) error {
		return errors.Wrapf(err, "invalid secrets for listener %v", name)
	}
)

type Plugin struct {
	sslConfigTranslator utils.SslConfigTranslator
}

func (p *Plugin) Init(params plugins.InitParams) error {
	p.sslConfigTranslator = utils.NewSslConfigTranslator()
	return nil
}

func (p *Plugin) ProcessListenerFilterChain(params plugins.Params, in *v1.Listener) ([]envoylistener.FilterChain, error) {
	logger := contextutils.LoggerFrom(params.Ctx)
	tcpListener := in.GetTcpListener()
	if tcpListener == nil {
		return nil, nil
	}
	var filterChains []envoylistener.FilterChain
	for _, tcpHost := range tcpListener.TcpHosts {

		var listenerFilters []envoylistener.Filter
		statPrefix := tcpListener.GetStatPrefix()
		if statPrefix == "" {
			statPrefix = DefaultTcpStatPrefix
		}
		tcpFilter, err := tcpProxyFilter(params, tcpHost, tcpListener.GetPlugins(), statPrefix)
		if err != nil {
			logger.Errorw("could not compute tcp proxy filter", zap.Error(err), zap.Any("tcpHost", tcpHost))
			continue
		}

		listenerFilters = append(listenerFilters, *tcpFilter)

		filterChain, err := p.computerTcpFilterChain(params.Snapshot, in, listenerFilters, tcpHost)
		if err != nil {
			logger.Errorw("could not compute tcp proxy filter", zap.Error(err), zap.Any("tcpHost", tcpHost))
			continue
		}
		filterChains = append(filterChains, filterChain)
	}
	return filterChains, nil
}

func tcpProxyFilter(params plugins.Params, host *v1.TcpHost, plugins *v1.TcpListenerPlugins, statPrefix string) (*listener.Filter, error) {

	cfg := &envoytcp.TcpProxy{
		StatPrefix: statPrefix,
	}

	if plugins != nil {
		if tcpSettings := plugins.GetTcpProxySettings(); tcpSettings != nil {
			cfg.MaxConnectAttempts = tcpSettings.MaxConnectAttempts
			cfg.IdleTimeout = tcpSettings.IdleTimeout
		}
	}

	if err := translatorutil.ValidateRouteDestinations(params.Snapshot, host.Destination); err != nil {
		return nil, err
	}
	switch dest := host.GetDestination().GetDestination().(type) {
	case *v1.RouteAction_Single:
		usRef, err := usconversion.DestinationToUpstreamRef(dest.Single)
		if err != nil {
			return nil, err
		}
		cfg.ClusterSpecifier = &envoytcp.TcpProxy_Cluster{
			Cluster: translatorutil.UpstreamToClusterName(*usRef),
		}
	case *v1.RouteAction_Multi:
		wc, err := convertToWeightedCluster(dest.Multi)
		if err != nil {
			return nil, err
		}
		cfg.ClusterSpecifier = &envoytcp.TcpProxy_WeightedClusters{
			WeightedClusters: wc,
		}
	case *v1.RouteAction_UpstreamGroup:
		upstreamGroupRef := dest.UpstreamGroup
		upstreamGroup, err := params.Snapshot.UpstreamGroups.Find(upstreamGroupRef.Namespace, upstreamGroupRef.Name)
		if err != nil {
			return nil, pluginutils.NewUpstreamGroupNotFoundErr(*upstreamGroupRef)
		}
		md := &v1.MultiDestination{
			Destinations: upstreamGroup.Destinations,
		}

		wc, err := convertToWeightedCluster(md)
		if err != nil {
			return nil, err
		}
		cfg.ClusterSpecifier = &envoytcp.TcpProxy_WeightedClusters{
			WeightedClusters: wc,
		}

	default:
		return nil, NoDestinationTypeError(host)
	}
	tcpFilter, err := translatorutil.NewFilterWithConfig(envoyutil.TCPProxy, cfg)
	if err != nil {
		return nil, err
	}
	return &tcpFilter, nil
}

func convertToWeightedCluster(multiDest *v1.MultiDestination) (*envoytcp.TcpProxy_WeightedCluster, error) {
	if len(multiDest.Destinations) == 0 {
		return nil, translatorutil.NoDestinationSpecifiedError
	}

	wc := make([]*envoytcp.TcpProxy_WeightedCluster_ClusterWeight, len(multiDest.Destinations))
	for i, weightedDest := range multiDest.Destinations {

		usRef, err := usconversion.DestinationToUpstreamRef(weightedDest.Destination)
		if err != nil {
			return nil, err
		}

		wc[i] = &envoytcp.TcpProxy_WeightedCluster_ClusterWeight{
			Name:   translatorutil.UpstreamToClusterName(*usRef),
			Weight: weightedDest.Weight,
		}
	}
	return &envoytcp.TcpProxy_WeightedCluster{Clusters: wc}, nil
}

// create a duplicate of the listener filter chain for each ssl cert we want to serve
// if there is no SSL config on the listener, the envoy listener will have one insecure filter chain
func (p *Plugin) computerTcpFilterChain(snap *v1.ApiSnapshot, listener *v1.Listener, listenerFilters []envoylistener.Filter, host *v1.TcpHost) (envoylistener.FilterChain, error) {
	sslConfig := host.GetSslConfig()
	if sslConfig == nil {
		return envoylistener.FilterChain{
			Filters:       listenerFilters,
			UseProxyProto: listener.UseProxyProto,
		}, nil
	}

	downstreamConfig, err := p.sslConfigTranslator.ResolveDownstreamSslConfig(snap.Secrets, sslConfig)
	if err != nil {
		return envoylistener.FilterChain{}, InvalidSecretsError(err, listener.Name)
	}
	return newSslFilterChain(downstreamConfig, sslConfig.SniDomains, listener.UseProxyProto, listenerFilters), nil
}

func newSslFilterChain(downstreamConfig *envoyauth.DownstreamTlsContext, sniDomains []string, useProxyProto *types.BoolValue, listenerFilters []envoylistener.Filter) envoylistener.FilterChain {

	return envoylistener.FilterChain{
		FilterChainMatch: &envoylistener.FilterChainMatch{
			ServerNames: sniDomains,
		},
		Filters:       listenerFilters,
		TlsContext:    downstreamConfig,
		UseProxyProto: useProxyProto,
	}
}
