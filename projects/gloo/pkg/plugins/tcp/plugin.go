package tcp

import (
	"errors"
	"time"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_extensions_filters_network_sni_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/sni_cluster/v3"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/hashicorp/go-multierror"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"

	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	als2 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/als"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/tcp"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/als"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/connection_limit"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"
	usconversion "github.com/solo-io/gloo/projects/gloo/pkg/upstreams"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
)

var (
	_ plugins.Plugin               = new(plugin)
	_ plugins.TcpFilterChainPlugin = new(plugin)
)

const (
	ExtensionName        = "tcp"
	DefaultTcpStatPrefix = "tcp"

	SniFilter = "envoy.filters.network.sni_cluster"
)

var (
	NoDestinationTypeError = func(host *v1.TcpHost) error {
		return eris.Errorf("no destination type was specified for tcp host %v", host)
	}

	InvalidSecretsError = func(err error, name string) error {
		return eris.Wrapf(err, "invalid secrets for host %v", name)
	}
)

type plugin struct {
	sslConfigTranslator utils.SslConfigTranslator
}

func NewPlugin(sslConfigTranslator utils.SslConfigTranslator) *plugin {
	return &plugin{sslConfigTranslator: sslConfigTranslator}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(_ plugins.InitParams) {
}

func (p *plugin) CreateTcpFilterChains(params plugins.Params, parentListener *v1.Listener, in *v1.TcpListener) ([]*envoy_config_listener_v3.FilterChain, error) {
	var filterChains []*envoy_config_listener_v3.FilterChain
	multiErr := multierror.Error{}

	alsSettings := parentListener.GetOptions().GetAccessLoggingService()
	tcpListenerOptions := in.GetOptions()

	for hostNum, tcpHost := range in.GetTcpHosts() {
		var listenerFilters []*envoy_config_listener_v3.Filter
		statPrefix := in.GetStatPrefix()
		if statPrefix == "" {
			statPrefix = DefaultTcpStatPrefix
		}

		tcpFilters, err := p.tcpProxyFilters(params, tcpHost, tcpListenerOptions, statPrefix, alsSettings)
		if err != nil {
			if _, ok := err.(*pluginutils.DestinationNotFoundError); ok {
				// this error will be treated as just a warning; wrap in a
				// special error object, and the caller will handle conversion
				// from error to warning
				// https://github.com/solo-io/solo-projects/issues/5163
				multiErr.Errors = append(multiErr.Errors, &validation.TcpHostWarning{
					Err:      err,
					ErrLevel: validation.ErrorLevels_WARNING,
					Context: validation.ErrorLevelContext{
						HostNum: &hostNum,
					},
				})
			} else {
				multiErr.Errors = append(multiErr.Errors, err)
			}
			continue
		}

		listenerFilters = append(listenerFilters, tcpFilters...)

		filterChain, err := p.computeTcpFilterChain(params.Snapshot, listenerFilters, tcpHost)
		if err != nil {
			multiErr.Errors = append(multiErr.Errors, err)
			continue
		}
		filterChains = append(filterChains, filterChain)
	}
	return filterChains, multiErr.ErrorOrNil()
}

func (p *plugin) generateNetworkFilters(plugins *v1.TcpListenerOptions) ([]*envoy_config_listener_v3.Filter, error) {
	connectionLimitFilter, err := connection_limit.GenerateFilter(plugins.GetConnectionLimit())
	if err != nil {
		return nil, err
	}
	return []*envoy_config_listener_v3.Filter{
		connectionLimitFilter,
	}, nil
}

func (p *plugin) tcpProxyFilters(
	params plugins.Params,
	host *v1.TcpHost,
	plugins *v1.TcpListenerOptions,
	statPrefix string,
	alsSettings *als2.AccessLoggingService,
) ([]*envoy_config_listener_v3.Filter, error) {

	cfg := &envoytcp.TcpProxy{
		StatPrefix: statPrefix,
	}

	if plugins != nil {
		if tcpSettings := plugins.GetTcpProxySettings(); tcpSettings != nil {
			cfg.MaxConnectAttempts = tcpSettings.GetMaxConnectAttempts()
			cfg.IdleTimeout = tcpSettings.GetIdleTimeout()
			cfg.TunnelingConfig = convertToEnvoyTunnelingConfig(tcpSettings.GetTunnelingConfig())
			flush := tcpSettings.GetAccessLogFlushInterval()
			if flush != nil && prototime.DurationFromProto(flush) < 1*time.Millisecond {
				return nil, errors.New("access log flush interval must have minimum of 1ms")
			}
			cfg.AccessLogFlushInterval = flush

		}
	}

	if err := translatorutil.ValidateTcpRouteDestinations(params.Snapshot, host.GetDestination()); err != nil {
		return nil, err
	}
	var filters []*envoy_config_listener_v3.Filter

	// Handle all network filters before proceeding to anything else
	networkFilters, err := p.generateNetworkFilters(plugins)
	if err != nil {
		return nil, err
	}
	filters = append(filters, networkFilters...)

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
		typedConfig, err := utils.MessageToAny(&envoy_extensions_filters_network_sni_cluster_v3.SniCluster{})
		if err != nil {
			return nil, err
		}
		filters = append(filters, &envoy_config_listener_v3.Filter{
			Name: SniFilter,
			ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
				TypedConfig: typedConfig,
			},
		})
	default:
		return nil, NoDestinationTypeError(host)
	}

	tcpAccessLogConfig, err := als.ProcessAccessLogPlugins(alsSettings, cfg.GetAccessLog())
	if err != nil {
		return nil, err
	}
	cfg.AccessLog = tcpAccessLogConfig

	tcpFilter, err := translatorutil.NewFilterWithTypedConfig(wellknown.TCPProxy, cfg)
	if err != nil {
		return nil, err
	}
	filters = append(filters, tcpFilter)

	return filters, nil
}

func (p *plugin) convertToWeightedCluster(multiDest *v1.MultiDestination) (*envoytcp.TcpProxy_WeightedCluster, error) {
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
			Weight: weightedDest.GetWeight().GetValue(),
		}
	}
	return &envoytcp.TcpProxy_WeightedCluster{Clusters: wc}, nil
}

// create a duplicate of the listener filter chain for each ssl cert we want to serve
// if there is no SSL config on the listener, the envoy listener will have one insecure filter chain
func (p *plugin) computeTcpFilterChain(
	snap *v1snap.ApiSnapshot,
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
		return nil, InvalidSecretsError(err, host.GetName())
	}
	return p.newSslFilterChain(downstreamConfig, sslConfig.GetSniDomains(), listenerFilters, sslConfig.GetTransportSocketConnectTimeout())
}

func (p *plugin) newSslFilterChain(
	downstreamConfig *envoyauth.DownstreamTlsContext,
	sniDomains []string,
	listenerFilters []*envoy_config_listener_v3.Filter,
	timeout *duration.Duration,
) (*envoy_config_listener_v3.FilterChain, error) {

	// copy listenerFilter so we can modify filter chain later without changing the filters on all of them!
	listenerFiltersCopy := make([]*envoy_config_listener_v3.Filter, len(listenerFilters))
	for i, lf := range listenerFilters {
		listenerFiltersCopy[i] = proto.Clone(lf).(*envoy_config_listener_v3.Filter)
	}
	typedConfig, err := utils.MessageToAny(downstreamConfig)
	if err != nil {
		return nil, err
	}
	return &envoy_config_listener_v3.FilterChain{
		FilterChainMatch: &envoy_config_listener_v3.FilterChainMatch{
			ServerNames: sniDomains,
		},
		Filters: listenerFiltersCopy,
		TransportSocket: &envoy_config_core_v3.TransportSocket{
			Name:       wellknown.TransportSocketTls,
			ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: typedConfig},
		},
		TransportSocketConnectTimeout: timeout,
	}, nil
}

func convertToEnvoyTunnelingConfig(config *tcp.TcpProxySettings_TunnelingConfig) *envoytcp.TcpProxy_TunnelingConfig {
	if config == nil {
		return nil
	}

	return &envoytcp.TcpProxy_TunnelingConfig{
		Hostname:     config.GetHostname(),
		HeadersToAdd: convertToEnvoyHeaderValueOption(config.GetHeadersToAdd()),
	}
}

func convertToEnvoyHeaderValueOption(hvos []*tcp.HeaderValueOption) []*envoy_config_core_v3.HeaderValueOption {
	if len(hvos) == 0 {
		return nil
	}
	headersToAdd := make([]*envoy_config_core_v3.HeaderValueOption, len(hvos))
	for i, hv := range hvos {
		ehvo := envoy_config_core_v3.HeaderValueOption{}
		ehvo.Append = hv.GetAppend()
		ehvo.Header = &envoy_config_core_v3.HeaderValue{
			Key:   hv.GetHeader().GetKey(),
			Value: hv.GetHeader().GetValue(),
		}
		headersToAdd[i] = &ehvo
	}
	return headersToAdd
}
