package connection_limit

import (
	"fmt"

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_connection_limit_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/connection_limit/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/connection_limit"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
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
	pluginStage = plugins.BeforeStage(plugins.AuthNStage)
)

type plugin struct {
	removeUnused bool
}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(params plugins.InitParams) {
	p.removeUnused = params.Settings.GetGloo().GetRemoveUnusedFilters().GetValue()
}

func GenerateFilter(connectionLimit *connection_limit.ConnectionLimit) (*envoy_config_listener_v3.Filter, error) {
	if connectionLimit == nil {
		return nil, nil
	}
	if connectionLimit.GetMaxActiveConnections() == nil {
		return nil, nil
	}
	if connectionLimit.GetMaxActiveConnections().GetValue() < 1 {
		return nil, fmt.Errorf("MaxActiveConnections must be greater than or equal to 1. Current value : %v", connectionLimit.GetMaxActiveConnections())
	}
	config := &envoy_config_connection_limit_v3.ConnectionLimit{
		StatPrefix:     StatPrefix,
		MaxConnections: connectionLimit.GetMaxActiveConnections(),
		Delay:          connectionLimit.GetDelayBeforeClose(),
	}
	marshalledConf, err := utils.MessageToAny(config)
	if err != nil {
		return nil, err
	}
	return &envoy_config_listener_v3.Filter{
		Name: ExtensionName,
		ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
			TypedConfig: marshalledConf,
		},
	}, nil

}

func (p *plugin) NetworkFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedNetworkFilter, error) {
	connectionLimitFilter, err := GenerateFilter(listener.GetOptions().GetConnectionLimit())
	if err != nil {
		return nil, err
	}
	return []plugins.StagedNetworkFilter{
		{
			NetworkFilter: connectionLimitFilter,
			Stage:         pluginStage,
		},
	}, nil
}
