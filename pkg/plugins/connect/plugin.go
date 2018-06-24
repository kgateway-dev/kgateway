package connect

import (
	"fmt"
	"time"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoytcpproxy "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/types"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins"
	"github.com/solo-io/gloo/pkg/protoutil"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
)

// this is the key the plugin will search for in the listener config
const (
	pluginName = "connect.gloo.solo.io"
	filterName = "io.solo.filters.network.client_certificate_restriction"
)

var (
	defaultTimeout = time.Second * 30
)

//go:generate protoc -I=./ -I=${GOPATH}/src/github.com/gogo/protobuf/ -I=${GOPATH}/src/github.com/lyft/protoc-gen-validate/ --gogo_out=${GOPATH}/src/ envoy/api/envoy/config/filter/network/client_certificate_restriction/v2/client_certificate_restriction.proto
//go:generate protoc -I=./ -I=${GOPATH}/src/github.com/lyft/protoc-gen-validate/ -I=${GOPATH}/src/github.com/gogo/protobuf/ --gogo_out=${GOPATH}/src/ listener_config.proto

func init() {
	plugins.Register(&Plugin{})
}

type Plugin struct {
	// these clusters are the destination clusters for the tcp proxy on the inbound listener
	// they're just localhost:port; only the local envoy needs to know them
	clustersToGenerate []*envoyapi.Cluster
}

func (p *Plugin) GetDependencies(_ *v1.Config) *plugins.Dependencies {
	return nil
}

func (p *Plugin) ListenerFilters(params *plugins.ListenerFilterPluginParams, in *v1.Listener) ([]plugins.StagedListenerFilter, error) {
	cfg, err := DecodeListenerConfig(in.Config)
	if err != nil {
		return nil, errors.Wrapf(err, "%v: invalid listener config for listener %v", pluginName, in.Name)
	}
	if cfg == nil {
		return nil, nil
	}
	switch listenerType := cfg.Config.(type) {
	case *ListenerConfig_Inbound:
		return p.inboundListenerFilters(params, in, listenerType.Inbound)
	case *ListenerConfig_Outbound:
		return p.outboundListenerFilters(params, in, listenerType.Outbound)
	}
	return nil, errors.Wrapf(err, "%v: unknown config type for listener %v", pluginName, in.Name)
}

func (p *Plugin) inboundListenerFilters(params *plugins.ListenerFilterPluginParams, listener *v1.Listener, cfg *InboundListenerConfig) ([]plugins.StagedListenerFilter, error) {
	if err := validateAuthConfig(cfg.AuthConfig); err != nil {
		return nil, err
	}
	if cfg.LocalServicePort == 0 {
		return nil, errors.Errorf("must define local_service_port")
	}
	if cfg.LocalUpstreamName == "" {
		return nil, errors.Errorf("must define local_upstream_name")
	}
	if err := validateListener(listener, cfg.LocalUpstreamName, params.Config.VirtualServices); err != nil {
		return nil, err
	}
	localServiceCluster := &envoyapi.Cluster{
		Name: fmt.Sprintf("local-service-%v-%v", cfg.LocalUpstreamName, cfg.LocalServicePort),
		Type: envoyapi.Cluster_STRICT_DNS,
		Hosts: []*envoycore.Address{
			{
				Address: &envoycore.Address_SocketAddress{
					SocketAddress: &envoycore.SocketAddress{
						Protocol: envoycore.TCP,
						Address:  "localhost",
						PortSpecifier: &envoycore.SocketAddress_PortValue{
							PortValue: cfg.LocalServicePort,
						},
					},
				},
			},
		},
		DnsLookupFamily: envoyapi.Cluster_V4_ONLY,
	}
	consulAgentCluster := &envoyapi.Cluster{
		Name: fmt.Sprintf("local-consul-agent"),
		Type: envoyapi.Cluster_STRICT_DNS,
		Hosts: []*envoycore.Address{
			{
				Address: &envoycore.Address_SocketAddress{
					SocketAddress: &envoycore.SocketAddress{
						Protocol: envoycore.TCP,
						Address:  cfg.AuthConfig.AuthorizeHostname,
						PortSpecifier: &envoycore.SocketAddress_PortValue{
							PortValue: cfg.AuthConfig.AuthorizePort,
						},
					},
				},
			},
		},
		DnsLookupFamily: envoyapi.Cluster_V4_ONLY,
	}
	generatedClusters := []*envoyapi.Cluster{
		localServiceCluster,
		consulAgentCluster,
	}
	p.clustersToGenerate = append(p.clustersToGenerate, generatedClusters...)
	inboundTcpProxy, err := protoutil.MarshalStruct(&envoytcpproxy.TcpProxy{
		Cluster: localServiceCluster.Name,
	})
	if err != nil {
		panic("unexpected error marsahlling filter config: " + err.Error())
	}
	tcpProxyFilter := envoylistener.Filter{
		Name:   util.TCPProxy,
		Config: inboundTcpProxy,
	}
	return []plugins.StagedListenerFilter{
		{
			ListenerFilter: createAuthFilter(consulAgentCluster.Name, cfg.AuthConfig),
			Stage:          plugins.InAuth,
		},
		{
			ListenerFilter: tcpProxyFilter,
			Stage:          plugins.PostInAuth,
		},
	}, nil
}

func (p *Plugin) outboundListenerFilters(params *plugins.ListenerFilterPluginParams, listener *v1.Listener, cfg *OutboundListenerConfig) ([]plugins.StagedListenerFilter, error) {
	if err := validateProxyConfig(cfg.ProxyConfig); err != nil {
		return nil, err
	}
	if err := validateListener(listener, cfg.ProxyConfig.DestinationUpstream, params.Config.VirtualServices); err != nil {
		return nil, err
	}
	tcpProxyFilterConfig := &envoytcpproxy.TcpProxy{
		Cluster: params.EnvoyNameForUpstream(cfg.ProxyConfig.DestinationUpstream),
	}
	tcpProxyFilterConfigStruct, err := protoutil.MarshalStruct(tcpProxyFilterConfig)
	if err != nil {
		panic("unexpected error marsahlling filter config: " + err.Error())
	}
	tcpProxyFilter := envoylistener.Filter{
		Name:   util.TCPProxy,
		Config: tcpProxyFilterConfigStruct,
	}
	return []plugins.StagedListenerFilter{
		{
			ListenerFilter: tcpProxyFilter,
			Stage:          plugins.PostInAuth,
		},
	}, nil
}

func (p *Plugin) GeneratedClusters(_ *plugins.ClusterGeneratorPluginParams) ([]*envoyapi.Cluster, error) {
	clusters := p.clustersToGenerate
	// flush cache
	p.clustersToGenerate = nil
	return clusters, nil
}

// apply the connect security policy to the listener
// each listener is only allowed to connect to a single destination
func validateListener(listener *v1.Listener, destinationUpstream string, virtualServices []*v1.VirtualService) error {
	var destinationVirtualServices []*v1.VirtualService
	for _, vs := range virtualServices {
		for _, destinationVirtualService := range listener.VirtualServices {
			if vs.Name == destinationVirtualService {
				destinationVirtualServices = append(destinationVirtualServices, vs)
				break
			}
		}
	}
	// no virtualservices for this listener
	if len(destinationVirtualServices) == 0 {
		return nil
	}
	var destinationUpstreams []string
	for _, destinationVirtualService := range destinationVirtualServices {
		destinationUpstreams = append(destinationUpstreams, allDestinationUpstreams(destinationVirtualService)...)
	}
	if len(destinationUpstreams) > 1 || destinationUpstreams[0] != destinationUpstream {
		return errors.Errorf("%v is an invalid virtualservice list for this listener. "+
			"%v is the only valid destination for routes on this listener", listener.VirtualServices, destinationUpstream)
	}
	return nil
}

func allDestinationUpstreams(destinationVirtualService *v1.VirtualService) []string {
	var destinations []string
	for _, route := range destinationVirtualService.Routes {
		destinations = append(destinations, destinationUpstreams(route)...)
	}
	return destinations
}

func destinationUpstreams(route *v1.Route) []string {
	switch {
	case route.SingleDestination != nil:
		return []string{destinationUpstream(route.SingleDestination)}
	case route.MultipleDestinations != nil:
		var destinationUpstreams []string
		for _, dest := range route.MultipleDestinations {
			destinationUpstreams = append(destinationUpstreams, destinationUpstream(dest.Destination))
		}
		return destinationUpstreams
	}
	panic("invalid route")
}

func destinationUpstream(dest *v1.Destination) string {
	switch dest := dest.DestinationType.(type) {
	case *v1.Destination_Upstream:
		return dest.Upstream.Name
	case *v1.Destination_Function:
		return dest.Function.UpstreamName
	}
	panic("invalid destination")
}

func createAuthFilter(authClusterName string, auth *AuthConfig) envoylistener.Filter {
	if auth.RequestTimeout == nil || *auth.RequestTimeout == 0 {
		auth.RequestTimeout = &defaultTimeout
	}
	filterConfig := &ClientCertificateRestriction{
		Target:               auth.Target,
		AuthorizeHostname:    auth.AuthorizeHostname,
		AuthorizeClusterName: authClusterName,
		RequestTimeout:       auth.RequestTimeout,
	}
	filterConfigStruct, err := protoutil.MarshalStruct(filterConfig)
	if err != nil {
		panic("unexpected error marshalling proto to struct: " + err.Error())
	}
	return envoylistener.Filter{
		Name:   filterName,
		Config: filterConfigStruct,
	}
}

func EncodeListenerConfig(config *ListenerConfig) *types.Struct {
	if config == nil {
		return nil
	}
	s, err := protoutil.MarshalStruct(config)
	if err != nil {
		panic("failed to encode listener config: " + err.Error())
	}
	return s
}

func DecodeListenerConfig(config *types.Struct) (*ListenerConfig, error) {
	if config == nil {
		return nil, nil
	}
	pluginConfig, ok := config.Fields[pluginName]
	if !ok {
		return nil, nil
	}
	cfg := new(ListenerConfig)
	if err := protoutil.UnmarshalValue(pluginConfig, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func validateProxyConfig(cfg *TcpProxyConfig) error {
	if cfg.DestinationUpstream == "" {
		return errors.Errorf("destination upstream cannot be empty")
	}
	return nil
}

func validateAuthConfig(cfg *AuthConfig) error {
	if cfg == nil {
		return errors.Errorf("must provide AuthConfig")
	}
	if cfg.Target == "" {
		return errors.Errorf("must provide AuthConfig.Target")
	}
	if cfg.AuthorizePort == 0 {
		return errors.Errorf("must provide AuthConfig.AuthorizePort")
	}
	if cfg.AuthorizeHostname == "" {
		return errors.Errorf("must provide AuthConfig.AuthorizeHostname")
	}
	if cfg.AuthorizePath == "" {
		return errors.Errorf("must provide AuthConfig.AuthorizePath")
	}
	return nil
}
