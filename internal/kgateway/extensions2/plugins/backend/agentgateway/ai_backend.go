package agentgatewaybackend

import (
	"errors"
	"fmt"

	"github.com/agentgateway/agentgateway/go/api"
)

func ProcessAIBackendForAgentGateway(be *AgentGatewayBackendIr) ([]*api.Backend, []*api.Policy, error) {
	if len(be.Errors) > 0 {
		return nil, nil, fmt.Errorf("errors occurred while processing ai backend for agent gateway: %w", errors.Join(be.Errors...))
	}
	if be.AIIr == nil {
		return nil, nil, fmt.Errorf("ai backend ir must not be nil for AI backend type")
	}

	apiBackend := &api.Backend{
		Name: be.AIIr.Name,
		Kind: &api.Backend_Ai{
			Ai: be.AIIr.Backend,
		},
	}
	authPolicy := &api.Policy{
		Name: fmt.Sprintf("auth-%s", apiBackend.Name),
		Target: &api.PolicyTarget{Kind: &api.PolicyTarget_Backend{
			Backend: apiBackend.Name,
		}},
		Spec: &api.PolicySpec{Kind: &api.PolicySpec_Auth{
			Auth: be.AIIr.AuthPolicy,
		}},
	}
	return []*api.Backend{apiBackend}, []*api.Policy{authPolicy}, nil
}
