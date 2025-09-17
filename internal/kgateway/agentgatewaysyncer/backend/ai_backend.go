package agentgatewaybackend

import (
	"errors"
	"fmt"

	"github.com/agentgateway/agentgateway/go/api"
)

func ProcessAIBackendForAgw(be *AgwBackendIr) ([]*api.Backend, []*api.Policy, error) {
	if len(be.Errors) > 0 {
		return nil, nil, fmt.Errorf("errors occurred while processing ai backend for agent gateway: %w", errors.Join(be.Errors...))
	}
	if be.AIIr == nil {
		return nil, nil, fmt.Errorf("ai backend ir must not be nil for AI backend type")
	}

	return []*api.Backend{be.AIIr.Backend}, be.AIIr.Policies, nil
}
