package agentgatewaybackend

import (
	"errors"
	"fmt"

	"github.com/agentgateway/agentgateway/go/api"
)

func ProcessStaticBackendForAgw(be *AgwBackendIr) ([]*api.Backend, []*api.Policy, error) {
	if len(be.Errors) > 0 {
		return nil, nil, fmt.Errorf("errors occurred while processing static backend for agent gateway: %w", errors.Join(be.Errors...))
	}
	return []*api.Backend{be.StaticIr.Backend}, nil, nil
}
