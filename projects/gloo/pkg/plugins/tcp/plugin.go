package tcp

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoytcp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	envoyutil "github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/tcp"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	translatorutil "github.com/solo-io/gloo/projects/gloo/pkg/translator"
	usconversion "github.com/solo-io/gloo/projects/gloo/pkg/upstreams"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/errors"
)

const (
	// filter info
	pluginStage = plugins.PostInAuth
)

func NewPlugin() *Plugin {
	return &Plugin{}
}

var (
	_ plugins.Plugin                    = new(Plugin)
	_ plugins.ListenerPlugin            = new(Plugin)
	_ plugins.ListenerFilterChainPlugin = new(Plugin)

	NoDestinationTypeError = func(host *v1.TcpHost) error {
		return errors.Errorf("no destination type was specified for tcp host %v", host)
	}
)

type Plugin struct {
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) ProcessListener(params plugins.Params, in *v1.Listener, out *envoyapi.Listener) error {
	tl := in.GetTcpListener()
	if tl == nil {
		return nil
	}

	if err := addTcpProxySettings(tl, out); err != nil {
		return err
	}

	return nil
}

func addTcpProxySettings(tl *v1.TcpListener, out *envoyapi.Listener) error {
	if tl.Plugins == nil {
		return nil
	}
	tcpSettings := tl.Plugins.TcpProxySettings
	if tcpSettings == nil {
		return nil
	}
	for _, f := range out.FilterChains {
		for i, filter := range f.Filters {
			if filter.Name == envoyutil.TCPProxy {
				// get config
				var cfg envoytcp.TcpProxy
				err := translatorutil.ParseConfig(&filter, &cfg)
				// this should never error
				if err != nil {
					return err
				}

				copySettings(&cfg, tcpSettings)

				f.Filters[i], err = translatorutil.NewFilterWithConfig(envoyutil.TCPProxy, &cfg)
				// this should never error
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func copySettings(cfg *envoytcp.TcpProxy, tcpSettings *tcp.TcpProxySettings) {
	cfg.IdleTimeout = tcpSettings.IdleTimeout
	cfg.MaxConnectAttempts = tcpSettings.MaxConnectAttempts
}

func (p *Plugin) ProcessListenerFilterChain(params plugins.Params, in *v1.Listener) ([]envoylistener.FilterChain, error) {
	tcpListener := in.GetTcpListener()
	if tcpListener == nil {
		return nil, nil
	}
	var filterChains []envoylistener.FilterChain
	for _, tcpHost := range tcpListener.TcpHosts {

		var listenerFilters []envoylistener.Filter

		tcpFilter, err := tcpProxyFilter(params, tcpHost)
		if err != nil {
			contextutils.LoggerFrom(params.Ctx).Debug(err, "could not compute tcp proxy filter for %v", tcpHost)
			continue
		}
		listenerFilters = append(listenerFilters, *tcpFilter)

		filterChain, err := computerTcpFilterChain(params.Snapshot, in, listenerFilters, tcpHost)
		if err != nil {
			contextutils.LoggerFrom(params.Ctx).Debug(err, "could not compute tcp filter chain for %v", tcpHost)
			continue
		}
		filterChains = append(filterChains, filterChain)
	}
	return filterChains, nil
}

func tcpProxyFilter(params plugins.Params, host *v1.TcpHost) (*listener.Filter, error) {
	cfg := &envoytcp.TcpProxy{
		StatPrefix: "tcp",
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
			return nil, err
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
func computerTcpFilterChain(snap *v1.ApiSnapshot, listener *v1.Listener, listenerFilters []envoylistener.Filter, host *v1.TcpHost) (envoylistener.FilterChain, error) {
	sslConfig := host.GetSslConfig()
	if sslConfig == nil {
		return envoylistener.FilterChain{
			Filters:       listenerFilters,
			UseProxyProto: listener.UseProxyProto,
		}, nil
	}

	sslCfgTranslator := utils.NewSslConfigTranslator()
	downstreamConfig, err := sslCfgTranslator.ResolveDownstreamSslConfig(snap, sslConfig)
	if err != nil {
		return envoylistener.FilterChain{}, errors.Wrapf(err, "invalid secrets for listener %v", listener.Name)
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
