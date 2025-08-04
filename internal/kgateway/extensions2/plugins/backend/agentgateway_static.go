package backend

import (
	"fmt"

	"github.com/agentgateway/agentgateway/go/api"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

func processStaticBackendForAgentGateway(be *v1alpha1.Backend) ([]*api.Backend, []*api.Policy, error) {
	if len(be.Spec.Static.Hosts) > 1 {
		// TODO(jmcguire98): as of now agentgateway does not support multiple hosts for static backends
		// if we want to have similar behavior to envoy (load balancing across all hosts provided)
		// we will need to add support for this in agentgateway
		return nil, nil, fmt.Errorf("multiple hosts are currently not supported for static backends in agentgateway")
	}
	if len(be.Spec.Static.Hosts) == 0 {
		return nil, nil, fmt.Errorf("static backends must have at least one host")
	}
	return []*api.Backend{&api.Backend{
		Name: be.Namespace + "/" + be.Name,
		Kind: &api.Backend_Static{
			Static: &api.StaticBackend{
				Host: be.Spec.Static.Hosts[0].Host,
				Port: int32(be.Spec.Static.Hosts[0].Port),
			},
		},
	}}, nil, nil
}
