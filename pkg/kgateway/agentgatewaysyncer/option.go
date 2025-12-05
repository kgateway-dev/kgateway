package agentgatewaysyncer

import "github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/translator"

type agentGatewaySyncerConfig struct {
	GatewayTransformationFunc translator.GatewaysTransformationFunction
}

type AgentgatewaySyncerOption func(*agentGatewaySyncerConfig)

func processAgentgatewaySyncerOptions(opts ...AgentgatewaySyncerOption) *agentGatewaySyncerConfig {
	cfg := &agentGatewaySyncerConfig{}
	for _, fn := range opts {
		fn(cfg)
	}
	return cfg
}

func WithGatewayForDeployerTransformationFunc(f translator.GatewaysTransformationFunction) AgentgatewaySyncerOption {
	return func(o *agentGatewaySyncerConfig) {
		o.GatewayTransformationFunc = f
	}
}
