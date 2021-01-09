package bootstrap

import (
	"bytes"
	"context"
	"os"
	"os/exec"

	"github.com/golang/protobuf/jsonpb"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"

	"github.com/envoyproxy/go-control-plane/pkg/wellknown"

	"github.com/golang/protobuf/ptypes/any"

	envoy_config_bootstrap_v3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	v34 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_extensions_filters_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/golang/protobuf/proto"
	"github.com/rotisserie/eris"
	"github.com/solo-io/go-utils/contextutils"
)

const defaultEnvoyPath = "/usr/local/bin/envoy"

func getEnvoyPath() string {
	ep := os.Getenv("ENVOY_BINARY_PATH")
	if len(ep) == 0 {
		ep = defaultEnvoyPath
	}
	return ep
}

func ValidateBootstrap(ctx context.Context, bootstrapYaml string) error {
	return nil
	envoyPath := getEnvoyPath()
	validateCmd := exec.Command(envoyPath, "--mode", "validate", "--config-yaml", bootstrapYaml, "-l", "critical", "--log-format", "%v")
	if output, err := validateCmd.CombinedOutput(); err != nil {
		if os.IsNotExist(err) {
			// log a warning and return nil; will allow users to continue to run Gloo locally without
			// relying on the Gloo container with Envoy already published to the expected directory
			contextutils.LoggerFrom(ctx).Warnf("Unable to validate envoy configuration using envoy at %v; "+
				"skipping additional validation of Gloo config.", envoyPath)
			return nil
		}
		return eris.Errorf("envoy validation mode output: %v, error: %v", string(output), err)
	}
	return nil
}

func BuildPerFilterBootstrapYaml(filterName string, msg proto.Message) string {

	typedFilter := utils.MustMessageToAny(msg)
	vhosts := []*envoy_config_route_v3.VirtualHost{
		{
			Name:    "placeholder_host",
			Domains: []string{"*"},
			TypedPerFilterConfig: map[string]*any.Any{
				filterName: &any.Any{
					TypeUrl: typedFilter.GetTypeUrl(),
					Value:   typedFilter.GetValue(),
				},
			},
		},
	}

	rc := &envoy_config_route_v3.RouteConfiguration{VirtualHosts: vhosts}

	hcm := &envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager{
		StatPrefix:     "placeholder",
		RouteSpecifier: &envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager_RouteConfig{RouteConfig: rc},
	}

	hcmAny := utils.MustMessageToAny(hcm)
	bootstrap := &envoy_config_bootstrap_v3.Bootstrap{
		Node: &v3.Node{
			Id:      "imspecial",
			Cluster: "doesntmatter",
		},
		StaticResources: &envoy_config_bootstrap_v3.Bootstrap_StaticResources{
			Listeners: []*v34.Listener{
				{
					Name: "placeholder_listener",
					Address: &v3.Address{
						Address: &v3.Address_SocketAddress{SocketAddress: &v3.SocketAddress{
							Address:       "0.0.0.0",
							PortSpecifier: &v3.SocketAddress_PortValue{PortValue: 8081},
						}},
					},
					FilterChains: []*v34.FilterChain{
						{
							Name: "placeholder_filter_chain",
							Filters: []*v34.Filter{
								{
									ConfigType: &v34.Filter_TypedConfig{
										TypedConfig: hcmAny,
									},
									Name: wellknown.HTTPConnectionManager,
								},
							},
						},
					},
				},
			},
		},
	}

	buf := &bytes.Buffer{}
	marshaler := &jsonpb.Marshaler{
		OrigName: true,
	}
	marshaler.Marshal(buf, bootstrap)
	json := string(buf.Bytes())
	return json // returns a json, but json is valid yaml
}
