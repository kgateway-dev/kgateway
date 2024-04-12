package install

import (
	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
)

type OperationProvider struct {
	clusterContext *cluster.ClusterContext
}

func NewProvider() *OperationProvider {
	return &OperationProvider{
		clusterContext: nil,
	}
}

// WithClusterContext sets the ScenarioProvider to point to the provided cluster
func (p *OperationProvider) WithClusterContext(clusterContext *cluster.ClusterContext) *OperationProvider {
	p.clusterContext = clusterContext
	return p
}
