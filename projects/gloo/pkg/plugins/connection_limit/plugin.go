package connection_limit

import (
	"fmt"

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_connection_limit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/connection_limit/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/connection_limit"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var (
	_ plugins.Plugin              = new(plugin)
	_ plugins.NetworkFilterPlugin = new(plugin)
)

const (
	ExtensionName = "envoy.extensions.filters.network.connection_limit.v3.ConnectionLimit"
	StatPrefix    = "connection_limit"
)

var (
	// Since this is an L4 filter, it would kick in before any HTTP processing takes place.
	// This also bolsters its main use case which is protect resources.
	pluginStage = plugins.BeforeStage(plugins.RateLimitStage)
)

type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(params plugins.InitParams) {}

func generateNetworkFilter(connectionLimit *connection_limit.ConnectionLimit) ([]plugins.StagedNetworkFilter, error) {
	// Sanity checks
	if connectionLimit.GetMaxActiveConnections() == nil {
		return []plugins.StagedNetworkFilter{}, nil
	}
	if connectionLimit.GetMaxActiveConnections().GetValue() < 1 {
		return nil, fmt.Errorf("MaxActiveConnections must be greater than or equal to 1. Current value : %v", connectionLimit.GetMaxActiveConnections())
	}

	config := &envoy_config_connection_limit_v3.ConnectionLimit{
		StatPrefix: StatPrefix,
		MaxConnections: &wrapperspb.UInt64Value{
			Value: uint64(connectionLimit.GetMaxActiveConnections().GetValue()),
		},
		Delay: connectionLimit.GetDelayBeforeClose(),
	}
	marshalledConf, err := utils.MessageToAny(config)
	if err != nil {
		return nil, err
	}
	return []plugins.StagedNetworkFilter{
		{
			NetworkFilter: &envoy_config_listener_v3.Filter{
				Name: ExtensionName,
				ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
					TypedConfig: marshalledConf,
				},
			},
			Stage: pluginStage,
		},
	}, nil

}

func (p *plugin) NetworkFiltersHTTP(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedNetworkFilter, error) {
	return generateNetworkFilter(listener.GetOptions().GetConnectionLimit())
}

func (p *plugin) NetworkFiltersTCP(params plugins.Params, listener *v1.TcpListener) ([]plugins.StagedNetworkFilter, error) {
	return generateNetworkFilter(listener.GetOptions().GetConnectionLimit())
}
