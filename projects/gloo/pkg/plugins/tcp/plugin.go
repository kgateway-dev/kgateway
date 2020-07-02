package tcp

import (
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils/gogoutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"
	usconversion "github.com/solo-io/gloo/projects/gloo/pkg/upstreams"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
)

const (
	DefaultTcpStatPrefix = "tcp"

	SniFilter = "envoy.filters.network.sni_cluster"
)

func NewPlugin() *Plugin {
	return &Plugin{}
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

func (p *Plugin) Init(params plugins.InitParams) error {
	p.sslConfigTranslator = utils.NewSslConfigTranslator()
	return nil
}

func (p *Plugin) ProcessListenerFilterChain(params plugins.Params, in *v1.Listener) ([]*envoylistener.FilterChain, error) {
	logger := contextutils.LoggerFrom(params.Ctx)
	tcpListener := in.GetTcpListener()
	if tcpListener == nil {
		return nil, nil
	}
	var filterChains []*envoylistener.FilterChain
	for _, tcpHost := range tcpListener.TcpHosts {

		var listenerFilters []*envoylistener.Filter
		statPrefix := tcpListener.GetStatPrefix()
		if statPrefix == "" {
			statPrefix = DefaultTcpStatPrefix
		}
		tcpFilters, err := p.tcpProxyFilters(params, tcpHost, tcpListener.GetOptions(), statPrefix)
		if err != nil {
			logger.Errorw("could not compute tcp proxy filter", zap.Error(err), zap.Any("tcpHost", tcpHost))
			continue
		}

		listenerFilters = append(listenerFilters, tcpFilters...)

		filterChain, err := p.computerTcpFilterChain(params.Snapshot, in, listenerFilters, tcpHost)
		if err != nil {
			logger.Errorw("could not compute tcp proxy filter", zap.Error(err), zap.Any("tcpHost", tcpHost))
			continue
		}
		filterChains = append(filterChains, filterChain)
	}
	return filterChains, nil
}

func (p *Plugin) tcpProxyFilters(
	params plugins.Params,
	host *v1.TcpHost,
	plugins *v1.TcpListenerOptions,
	statPrefix string,
) ([]*envoylistener.Filter, error) {

	cfg := &envoytcp.TcpProxy{
		StatPrefix: statPrefix,
	}

	if plugins != nil {
		if tcpSettings := plugins.GetTcpProxySettings(); tcpSettings != nil {
			cfg.MaxConnectAttempts = gogoutils.UInt32GogoToProto(tcpSettings.GetMaxConnectAttempts())
			cfg.IdleTimeout = gogoutils.DurationStdToProto(tcpSettings.GetIdleTimeout())
		}
	}

	if err := translatorutil.ValidateTcpRouteDestinations(params.Snapshot, host.GetDestination()); err != nil {
		return nil, err
	}
	var filters []*envoylistener.Filter
	switch dest := host.GetDestination().GetDestination().(type) {
	case *v1.TcpHost_TcpAction_Single:
		usRef, err := usconversion.DestinationToUpstreamRef(dest.Single)
		if err != nil {
			return nil, err
		}
		cfg.ClusterSpecifier = &envoytcp.TcpProxy_Cluster{
			Cluster: translatorutil.UpstreamToClusterName(*usRef),
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
		upstreamGroup, err := params.Snapshot.UpstreamGroups.Find(upstreamGroupRef.Namespace, upstreamGroupRef.Name)
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
		filters = append(filters, &envoylistener.Filter{
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
	if len(multiDest.Destinations) == 0 {
		return nil, translatorutil.NoDestinationSpecifiedError
	}

	wc := make([]*envoytcp.TcpProxy_WeightedCluster_ClusterWeight, len(multiDest.GetDestinations()))
	for i, weightedDest := range multiDest.GetDestinations() {

		usRef, err := usconversion.DestinationToUpstreamRef(weightedDest.GetDestination())
		if err != nil {
			return nil, err
		}

		wc[i] = &envoytcp.TcpProxy_WeightedCluster_ClusterWeight{
			Name:   translatorutil.UpstreamToClusterName(*usRef),
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
	listenerFilters []*envoylistener.Filter,
	host *v1.TcpHost,
) (*envoylistener.FilterChain, error) {
	sslConfig := host.GetSslConfig()
	if sslConfig == nil {
		return &envoylistener.FilterChain{
			Filters:       listenerFilters,
			UseProxyProto: gogoutils.BoolGogoToProto(listener.GetUseProxyProto()),
		}, nil
	}

	downstreamConfig, err := p.sslConfigTranslator.ResolveDownstreamSslConfig(snap.Secrets, sslConfig)
	if err != nil {
		return nil, InvalidSecretsError(err, listener.GetName())
	}
	return p.newSslFilterChain(downstreamConfig,
		sslConfig.GetSniDomains(),
		listener.GetUseProxyProto(),
		listenerFilters,
	), nil
}

func (p *Plugin) newSslFilterChain(
	downstreamConfig *envoyauth.DownstreamTlsContext,
	sniDomains []string,
	useProxyProto *types.BoolValue,
	listenerFilters []*envoylistener.Filter,
) *envoylistener.FilterChain {

	// copy listenerFilter so we can modify filter chain later without changing the filters on all of them!
	listenerFiltersCopy := make([]*envoylistener.Filter, len(listenerFilters))
	for i, lf := range listenerFilters {
		listenerFiltersCopy[i] = proto.Clone(lf).(*envoylistener.Filter)
	}

	return &envoylistener.FilterChain{
		FilterChainMatch: &envoylistener.FilterChainMatch{
			ServerNames: sniDomains,
		},
		Filters: listenerFiltersCopy,
		TransportSocket: &envoycore.TransportSocket{
			Name:       wellknown.TransportSocketTls,
			ConfigType: &envoycore.TransportSocket_TypedConfig{TypedConfig: utils.MustMessageToAny(downstreamConfig)},
		},
		UseProxyProto: gogoutils.BoolGogoToProto(useProxyProto),
	}
}
