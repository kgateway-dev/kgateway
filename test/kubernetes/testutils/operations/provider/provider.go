package provider

import (
	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations/glooctl"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations/kubectl"
)

// OperationProvider is the entity that creates Operation
// These assertions occur against a running instance of Gloo Gateway, within a Kubernetes Cluster.
// So this provider maintains state about the install/cluster it is using, and then provides
// operations.DiscreteAssertion to match
type OperationProvider struct {
	clusterContext *cluster.Context

	kubeCtlProvider *kubectl.OperationProvider
	glooCtlProvider *glooctl.OperationProvider
}

// NewOperationProvider returns an OperationProvider that will fail because it is not configured with a Kubernetes Cluster
func NewOperationProvider() *OperationProvider {
	return &OperationProvider{
		clusterContext: nil,

		kubeCtlProvider: kubectl.NewProvider(),
		glooCtlProvider: glooctl.NewProvider(),
	}
}

// WithClusterContext sets the provider, and all of it's sub-providers, to point to the provided cluster
func (p *OperationProvider) WithClusterContext(clusterContext *cluster.Context) *OperationProvider {
	p.clusterContext = clusterContext

	p.kubeCtlProvider.WithClusterCli(clusterContext.Cli)
	p.glooCtlProvider.WithClusterContext(clusterContext)
	return p
}

func (p *OperationProvider) KubeCtl() *kubectl.OperationProvider {
	return p.kubeCtlProvider
}

func (p *OperationProvider) GlooCtl() *glooctl.OperationProvider {
	return p.glooCtlProvider
}
