package agentgatewaysyncer

import "github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/translator"

type agentgatewaySyncerConfig struct {
	GatewayTransformationFunc translator.GatewaysTransformationFunction
}

type AgentgatewaySyncerOption func(*agentgatewaySyncerConfig)

func processAgentgatewaySyncerOptions(opts ...AgentgatewaySyncerOption) *agentgatewaySyncerConfig {
	cfg := &agentgatewaySyncerConfig{}
	for _, fn := range opts {
		fn(cfg)
	}
	return cfg
}

func WithGatewayTransformationFunc(f translator.GatewaysTransformationFunction) AgentgatewaySyncerOption {
	return func(o *agentgatewaySyncerConfig) {
		o.GatewayTransformationFunc = f
	}
}
