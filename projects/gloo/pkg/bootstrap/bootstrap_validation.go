package bootstrap

import (
	"bytes"
	"context"
	"os"
	"os/exec"

	envoy_config_bootstrap_v3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_extensions_filters_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
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

func ValidateBootstrap(
	ctx context.Context,
	settings *v1.Settings,
	filterName string,
	msg proto.Message,
) error {
	// If the user has disabled transformation validation, then always return nil
	if settings.GetGateway().GetValidation().GetDisableTransformationValidation().GetValue() {
		return nil
	}

	bootstrapYaml, err := buildPerFilterBootstrapYaml(filterName, msg)
	if err != nil {
		return err
	}

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

func buildPerFilterBootstrapYaml(filterName string, msg proto.Message) (string, error) {

	typedFilter, err := utils.MessageToAny(msg)
	if err != nil {
		return "", err
	}
	vhosts := []*envoy_config_route_v3.VirtualHost{
		{
			Name:    "placeholder_host",
			Domains: []string{"*"},
			TypedPerFilterConfig: map[string]*any.Any{
				filterName: {
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

	hcmAny, err := utils.MessageToAny(hcm)
	if err != nil {
		return "", err
	}
	bootstrap := &envoy_config_bootstrap_v3.Bootstrap{
		Node: &envoy_config_core_v3.Node{
			Id:      "imspecial",
			Cluster: "doesntmatter",
		},
		StaticResources: &envoy_config_bootstrap_v3.Bootstrap_StaticResources{
			Listeners: []*envoy_config_listener_v3.Listener{
				{
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
									ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
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
	return json, nil // returns a json, but json is valid yaml
}
