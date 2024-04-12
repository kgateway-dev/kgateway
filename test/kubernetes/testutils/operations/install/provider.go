package install

import "github.com/solo-io/gloo/test/testutils/kubeutils"

type OperationProvider struct {
	clusterContext *kubeutils.ClusterContext
}

func NewProvider() *OperationProvider {
	return &OperationProvider{
		clusterContext: nil,
	}
}

// WithClusterContext sets the ScenarioProvider to point to the provided cluster
func (p *OperationProvider) WithClusterContext(clusterContext *kubeutils.ClusterContext) *OperationProvider {
	p.clusterContext = clusterContext
	return p
}
