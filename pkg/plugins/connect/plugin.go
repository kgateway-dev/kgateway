package connect

import (
	"github.com/solo-io/gloo/pkg/plugins"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/protoutil"
	"github.com/gogo/protobuf/types"
	"github.com/pkg/errors"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"time"
	envoytcpproxy "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/tcp_proxy/v2"
	"github.com/envoyproxy/go-control-plane/pkg/util"
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

type Plugin struct{}

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
		//return p.outboundListenerFilters(params, in, listenerType.Outbound)
	}
	return nil, errors.Wrapf(err, "%v: unknown config type for listener %v", pluginName, in.Name)

	return []plugins.StagedListenerFilter{
		{
			ListenerFilter: createNetworkFilter(cfg),
			Stage:          plugins.InAuth,
		},
	}, nil
}

func (p *Plugin) inboundListenerFilters(params *plugins.ListenerFilterPluginParams, listener *v1.Listener, cfg *InboundListenerConfig) ([]plugins.StagedListenerFilter, error) {

	tcpProxyFilterConfig := &envoytcpproxy.TcpProxy{
		Cluster: params.EnvoyNameForUpstream(cfg.ProxyConfig.DestinationUpstream),
	}
	tcpProxyFilter := envoylistener.Filter{
		Name: util.TCPProxy,
		Config: tcpProxyFilterConfig,
	}
	return []plugins.StagedListenerFilter{
		{
			ListenerFilter: createNetworkFilter(cfg),
			Stage:          plugins.InAuth,
		},
		{
			ListenerFilter: v2.TcpProxy{},
			Stage:          plugins.PostInAuth,
		},
	}, nil
}

// apply the connect security policy to the listener
// each listener is only allowed to connect to a single destination
func validateListener(listener *v1.Listener, virtualServices []*v1.VirtualService) error {
	var destinationVirtualServices []*v1.VirtualService
	for _, vs := range virtualServices {
		for _, destinationVirtualService := range listener.VirtualServices {
			if vs.Name == destinationVirtualService {
				destinationVirtualServices = append(destinationVirtualServices, vs)
				break
			}
		}
	}

}

func createNetworkFilter(cfg *ListenerConfig) envoylistener.Filter {
	requestTimeout := cfg.AuthorizationConfig.ConnectionTimeout
	if requestTimeout == 0 {
		requestTimeout = defaultTimeout
	}
	filterConfig := &ClientCertificateRestriction{
		Target:               cfg.AuthorizationConfig.Target,
		AuthorizeHostname:    cfg.AuthorizationConfig.AuthorizeHostname,
		AuthorizeClusterName: cfg.AuthorizationConfig.AuthorizeClusterName,
		RequestTimeout:       &requestTimeout,
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
	return cfg, validate(cfg.Config)
}

func validateProxyConfig(cfg *TcpProxyConfig) error {
	if cfg.DestinationUpstream == "" {
		return errors.Errorf("destination upstream cannot be empty")
	}
	return nil
}

func validateAuthConfig(cfg *AuthConfig) error {
	if cfg.AuthorizeHostname != nil {
		return errors.Errorf("must provide AuthorizationConfig")
	}
	if cfg.AuthorizationConfig.Target == "" {
		return errors.Errorf("must provide AuthorizationConfig.Target")
	}
	if cfg.AuthorizationConfig.AuthorizeClusterName == "" {
		return errors.Errorf("must provide AuthorizationConfig.AuthorizeClusterName")
	}
	if cfg.AuthorizationConfig.AuthorizeHostname == "" {
		return errors.Errorf("must provide AuthorizationConfig.AuthorizeHostname")
	}
	return nil
}
