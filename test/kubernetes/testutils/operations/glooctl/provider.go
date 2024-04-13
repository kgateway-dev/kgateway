package glooctl

import (
	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
)

type OperationProvider struct {
	clusterContext *cluster.Context
}

func NewProvider() *OperationProvider {
	return &OperationProvider{
		clusterContext: nil,
	}
}

// WithClusterContext sets the ScenarioProvider to point to the provided cluster
func (p *OperationProvider) WithClusterContext(clusterContext *cluster.Context) *OperationProvider {
	p.clusterContext = clusterContext
	return p
}
