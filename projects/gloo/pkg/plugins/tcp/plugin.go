package tcp

import (
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/proto"
	"github.com/hashicorp/go-multierror"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/tcp"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"
	usconversion "github.com/solo-io/gloo/projects/gloo/pkg/upstreams"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

const (
	DefaultTcpStatPrefix = "tcp"

	SniFilter = "envoy.filters.network.sni_cluster"
)

func NewPlugin(sslConfigTranslator utils.SslConfigTranslator) *Plugin {
	return &Plugin{sslConfigTranslator: sslConfigTranslator}
}

var (
	_ plugins.Plugin                    = (*Plugin)(nil)
	_ plugins.ListenerFilterChainPlugin = (*Plugin)(nil)

	NoDestinationTypeError = func(host *v1.TcpHost) error {
		return eris.Errorf("no destination type was specified for tcp host %v", host)
	}

	InvalidSecretsError = func(err error, name string) error {
		return eris.Wrapf(err, "invalid secrets for listener %v", name)
	}
)

type Plugin struct {
	sslConfigTranslator utils.SslConfigTranslator
}

func (p *Plugin) Init(_ plugins.InitParams) error {
	return nil
}

func (p *Plugin) ProcessListenerFilterChain(params plugins.Params, in *v1.Listener) ([]*envoy_config_listener_v3.FilterChain, error) {
	tcpListener := in.GetTcpListener()
	if tcpListener == nil {
		return nil, nil
	}
	var filterChains []*envoy_config_listener_v3.FilterChain
	multiErr := multierror.Error{}
	for _, tcpHost := range tcpListener.GetTcpHosts() {

		var listenerFilters []*envoy_config_listener_v3.Filter
		statPrefix := tcpListener.GetStatPrefix()
		if statPrefix == "" {
			statPrefix = DefaultTcpStatPrefix
		}
		tcpFilters, err := p.tcpProxyFilters(params, tcpHost, tcpListener.GetOptions(), statPrefix)
		if err != nil {
			multiErr.Errors = append(multiErr.Errors, err)
			continue
		}

		listenerFilters = append(listenerFilters, tcpFilters...)

		filterChain, err := p.computerTcpFilterChain(params.Snapshot, in, listenerFilters, tcpHost)
		if err != nil {
			multiErr.Errors = append(multiErr.Errors, err)
			continue
		}
		filterChains = append(filterChains, filterChain)
	}
	return filterChains, multiErr.ErrorOrNil()
}

func (p *Plugin) tcpProxyFilters(
	params plugins.Params,
	host *v1.TcpHost,
	plugins *v1.TcpListenerOptions,
	statPrefix string,
) ([]*envoy_config_listener_v3.Filter, error) {

	cfg := &envoytcp.TcpProxy{
		StatPrefix: statPrefix,
	}

	if plugins != nil {
		if tcpSettings := plugins.GetTcpProxySettings(); tcpSettings != nil {
			cfg.MaxConnectAttempts = tcpSettings.GetMaxConnectAttempts()
			cfg.IdleTimeout = tcpSettings.GetIdleTimeout()
			cfg.TunnelingConfig = convertToEnvoyTunnelingConfig(tcpSettings.GetTunnelingConfig())
		}
	}

	if err := translatorutil.ValidateTcpRouteDestinations(params.Snapshot, host.GetDestination()); err != nil {
		return nil, err
	}
	var filters []*envoy_config_listener_v3.Filter
	switch dest := host.GetDestination().GetDestination().(type) {
	case *v1.TcpHost_TcpAction_Single:
		usRef, err := usconversion.DestinationToUpstreamRef(dest.Single)
		if err != nil {
			return nil, err
		}
		cfg.ClusterSpecifier = &envoytcp.TcpProxy_Cluster{
			Cluster: translatorutil.UpstreamToClusterName(usRef),
		}
	case *v1.TcpHost_TcpAction_Multi:
		wc, err := p.convertToWeightedCluster(dest.Multi)
		if err != nil {
			return nil, err
		}
		cfg.ClusterSpecifier = &envoytcp.TcpProxy_WeightedClusters{
			WeightedClusters: wc,
		}
	case *v1.TcpHost_TcpAction_UpstreamGroup:
		upstreamGroupRef := dest.UpstreamGroup
		upstreamGroup, err := params.Snapshot.UpstreamGroups.Find(upstreamGroupRef.GetNamespace(), upstreamGroupRef.GetName())
		if err != nil {
			return nil, pluginutils.NewUpstreamGroupNotFoundErr(*upstreamGroupRef)
		}
		md := &v1.MultiDestination{
			Destinations: upstreamGroup.GetDestinations(),
		}

		wc, err := p.convertToWeightedCluster(md)
		if err != nil {
			return nil, err
		}
		cfg.ClusterSpecifier = &envoytcp.TcpProxy_WeightedClusters{
			WeightedClusters: wc,
		}
	case *v1.TcpHost_TcpAction_ForwardSniClusterName:
		// Pass an empty cluster as it will be overwritten by SNI Cluster
		cfg.ClusterSpecifier = &envoytcp.TcpProxy_Cluster{
			Cluster: "",
		}
		// append empty sni-forward-filter to pass the SNI name to the cluster field above
		filters = append(filters, &envoy_config_listener_v3.Filter{
			Name: SniFilter,
		})
	default:
		return nil, NoDestinationTypeError(host)
	}

	tcpFilter, err := translatorutil.NewFilterWithTypedConfig(wellknown.TCPProxy, cfg)
	if err != nil {
		return nil, err
	}
	filters = append(filters, tcpFilter)

	return filters, nil
}

func (p *Plugin) convertToWeightedCluster(multiDest *v1.MultiDestination) (*envoytcp.TcpProxy_WeightedCluster, error) {
	if len(multiDest.GetDestinations()) == 0 {
		return nil, translatorutil.NoDestinationSpecifiedError
	}

	wc := make([]*envoytcp.TcpProxy_WeightedCluster_ClusterWeight, len(multiDest.GetDestinations()))
	for i, weightedDest := range multiDest.GetDestinations() {

		usRef, err := usconversion.DestinationToUpstreamRef(weightedDest.GetDestination())
		if err != nil {
			return nil, err
		}

		wc[i] = &envoytcp.TcpProxy_WeightedCluster_ClusterWeight{
			Name:   translatorutil.UpstreamToClusterName(usRef),
			Weight: weightedDest.GetWeight(),
		}
	}
	return &envoytcp.TcpProxy_WeightedCluster{Clusters: wc}, nil
}

// create a duplicate of the listener filter chain for each ssl cert we want to serve
// if there is no SSL config on the listener, the envoy listener will have one insecure filter chain
func (p *Plugin) computerTcpFilterChain(
	snap *v1.ApiSnapshot,
	listener *v1.Listener,
	listenerFilters []*envoy_config_listener_v3.Filter,
	host *v1.TcpHost,
) (*envoy_config_listener_v3.FilterChain, error) {
	sslConfig := host.GetSslConfig()
	if sslConfig == nil {
		return &envoy_config_listener_v3.FilterChain{
			Filters: listenerFilters,
		}, nil
	}

	downstreamConfig, err := p.sslConfigTranslator.ResolveDownstreamSslConfig(snap.Secrets, sslConfig)
	if err != nil {
		return nil, InvalidSecretsError(err, listener.GetName())
	}
	return p.newSslFilterChain(downstreamConfig, sslConfig.GetSniDomains(), listenerFilters), nil
}

func (p *Plugin) newSslFilterChain(
	downstreamConfig *envoyauth.DownstreamTlsContext,
	sniDomains []string,
	listenerFilters []*envoy_config_listener_v3.Filter,
) *envoy_config_listener_v3.FilterChain {

	// copy listenerFilter so we can modify filter chain later without changing the filters on all of them!
	listenerFiltersCopy := make([]*envoy_config_listener_v3.Filter, len(listenerFilters))
	for i, lf := range listenerFilters {
		listenerFiltersCopy[i] = proto.Clone(lf).(*envoy_config_listener_v3.Filter)
	}

	return &envoy_config_listener_v3.FilterChain{
		FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
			ServerNames: sniDomains,
		},
		Filters: listenerFiltersCopy,
		TransportSocket: &envoy_config_core_v3.TransportSocket{
			Name:       wellknown.TransportSocketTls,
			ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: utils.MustMessageToAny(downstreamConfig)},
		},
	}
}

func convertToEnvoyTunnelingConfig(config *tcp.TcpProxySettings_TunnelingConfig) *envoytcp.TcpProxy_TunnelingConfig {
	if config == nil {
		return nil
	}
	return &envoytcp.TcpProxy_TunnelingConfig{
		Hostname: config.GetHostname(),
	}
}
