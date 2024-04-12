package provider

import (
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations/install"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations/manifest"
	"github.com/solo-io/gloo/test/testutils/kubeutils"
)

// OperationProvider is the entity that creates Operation
// These assertions occur against a running instance of Gloo Gateway, within a Kubernetes Cluster.
// So this provider maintains state about the install/cluster it is using, and then provides
// operations.DiscreteAssertion to match
type OperationProvider struct {
	clusterContext *kubeutils.ClusterContext

	manifestProvider *manifest.OperationProvider
	installProvider  *install.OperationProvider
}

// NewOperationProvider returns an OperationProvider that will fail because it is not configured with a Kubernetes Cluster
func NewOperationProvider() *OperationProvider {
	return &OperationProvider{
		clusterContext:   nil,
		manifestProvider: manifest.NewProvider(),
		installProvider:  install.NewProvider(),
	}
}

// WithClusterContext sets the provider, and all of it's sub-providers, to point to the provided cluster
func (p *OperationProvider) WithClusterContext(clusterContext *kubeutils.ClusterContext) *OperationProvider {
	p.clusterContext = clusterContext

	p.manifestProvider.WithClusterCli(clusterContext.Cli)
	p.installProvider.WithClusterContext(clusterContext)
	return p
}

func (p *OperationProvider) Manifests() *manifest.OperationProvider {
	return p.manifestProvider
}

func (p *OperationProvider) Installs() *install.OperationProvider {
	return p.installProvider
}
