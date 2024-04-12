package provider

import (
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations/install"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations/manifest"
	"github.com/solo-io/gloo/test/testutils/kubeutils"
)

// Provider is the entity that creates Operation
// These assertions occur against a running instance of Gloo Gateway, within a Kubernetes Cluster.
// So this provider maintains state about the install/cluster it is using, and then provides
// operations.DiscreteAssertion to match
type Provider struct {
	clusterContext *kubeutils.ClusterContext

	manifestProvider *manifest.OperationProvider
	installProvider  *install.OperationProvider
}

// NewProvider returns a Provider that will fail because it is not configured with a Kubernetes Cluster
func NewProvider() *Provider {
	return &Provider{
		clusterContext:   nil,
		manifestProvider: manifest.NewProvider(),
		installProvider:  install.NewProvider(),
	}
}

// WithClusterContext sets the provider, and all of it's sub-providers, to point to the provided cluster
func (p *Provider) WithClusterContext(clusterContext *kubeutils.ClusterContext) *Provider {
	p.clusterContext = clusterContext

	p.manifestProvider.WithClusterCli(clusterContext.Cli)
	p.installProvider.WithClusterContext(clusterContext)
	return p
}

func (p *Provider) Manifests() *manifest.OperationProvider {
	return p.manifestProvider
}

func (p *Provider) Installs() *install.OperationProvider {
	return p.installProvider
}
