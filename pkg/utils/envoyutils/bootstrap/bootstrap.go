package bootstrap

import (
	"bytes"

	envoy_config_bootstrap_v3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_extensions_filters_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	anypb "github.com/golang/protobuf/ptypes/any"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

func FromEnvoyResources(resources *EnvoyResources) (string, error) {
	bootstrap := &envoy_config_bootstrap_v3.Bootstrap{
		Node: &envoy_config_core_v3.Node{
			Id:      "validation-node-id",
			Cluster: "validation-cluster",
		},
		StaticResources: &envoy_config_bootstrap_v3.Bootstrap_StaticResources{
			Listeners: resources.Listeners,
			Clusters:  resources.Clusters,
			Secrets:   resources.Secrets,
		},
	}

	buf := &bytes.Buffer{}
	marshaler := &jsonpb.Marshaler{
		OrigName: true,
	}
	marshaler.Marshal(buf, bootstrap)
	json := string(buf.Bytes())
	return json, nil // returns a json, but json is valid yaml
}

// FromFilter accepts a filter name and typed config for that filter,
// contructs a static bootstrap config containing a single vhost with typed
// per-filter config matching the arguments, marshals it to json, and returns
// the stringified json or any error if it occurred.
func FromFilter(filterName string, msg proto.Message) (string, error) {

	typedFilter, err := utils.MessageToAny(msg)
	if err != nil {
		return "", err
	}

	// Construct a vhost that contains our filter config as TypedPerFilterConfig.
	vhosts := []*envoy_config_route_v3.VirtualHost{
		{
			Name:    "placeholder_host",
			Domains: []string{"*"},
			TypedPerFilterConfig: map[string]*anypb.Any{
				filterName: {
					TypeUrl: typedFilter.GetTypeUrl(),
					Value:   typedFilter.GetValue(),
				},
			},
		},
	}

	// Use our vhost with tpfc in an HttpConnectionManager to be placed in a
	// FilterChain on our listener.
	hcm := &envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager{
		StatPrefix: "placeholder",
		RouteSpecifier: &envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager_RouteConfig{
			RouteConfig: &envoy_config_route_v3.RouteConfiguration{
				VirtualHosts: vhosts,
			},
		},
	}

	hcmAny, err := utils.MessageToAny(hcm)
	if err != nil {
		return "", err
	}
	listener := &envoy_config_listener_v3.Listener{
		Name: "placeholder_listener",
		Address: &envoy_config_core_v3.Address{
			Address: &envoy_config_core_v3.Address_SocketAddress{SocketAddress: &envoy_config_core_v3.SocketAddress{
				Address:       "0.0.0.0",
				PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{PortValue: 8081},
			}},
		},
		FilterChains: []*envoy_config_listener_v3.FilterChain{
			{
				Name: "placeholder_filter_chain",
				Filters: []*envoy_config_listener_v3.Filter{
					{
						Name: wellknown.HTTPConnectionManager,
						ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
							TypedConfig: hcmAny,
						},
					},
				},
			},
		},
	}

	return FromEnvoyResources(&EnvoyResources{Listeners: []*envoy_config_listener_v3.Listener{listener}})
}
