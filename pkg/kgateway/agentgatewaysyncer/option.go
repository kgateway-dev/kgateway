package agentgatewaysyncer

import "github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/translator"

type agentGatewaySyncerConfig struct {
	GatewayTransformationFunc translator.GatewaysTransformationFunction
}

type AgentGatewaySyncerOption func(*agentGatewaySyncerConfig)

func processAgentGatewaySyncerOptions(opts ...AgentGatewaySyncerOption) *agentGatewaySyncerConfig {
	cfg := &agentGatewaySyncerConfig{}
	for _, fn := range opts {
		fn(cfg)
	}
	return cfg
}

func WithGatewayForDeployerTransformationFunc(f translator.GatewaysTransformationFunction) AgentGatewaySyncerOption {
	return func(o *agentGatewaySyncerConfig) {
		o.GatewayTransformationFunc = f
	}
}
