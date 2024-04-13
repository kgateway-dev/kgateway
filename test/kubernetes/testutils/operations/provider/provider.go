package provider

import (
	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations/glooctl"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations/kubectl"
)

// OperationProvider is the entity that creates operations.
// These operations are executed against a running installation of Gloo Gateway, within a Kubernetes Cluster.
// This provider is just a wrapper around sub-providers, so it exposes methods to access those providers
type OperationProvider struct {
	kubeCtlProvider *kubectl.OperationProvider
	glooCtlProvider glooctl.OperationProvider
}

// NewOperationProvider returns an OperationProvider that will fail because it is not configured with a Kubernetes Cluster
func NewOperationProvider() *OperationProvider {
	return &OperationProvider{
		kubeCtlProvider: kubectl.NewProvider(),
		glooCtlProvider: glooctl.NewProvider(),
	}
}

// WithGlooctlProvider sets the glooctl provider on this OperationProvider
func (p *OperationProvider) WithGlooctlProvider(provider glooctl.OperationProvider) *OperationProvider {
	p.glooCtlProvider = provider
	return p
}

// WithClusterContext sets the provider, and all of it's sub-providers, to point to the provided cluster
func (p *OperationProvider) WithClusterContext(clusterContext *cluster.Context) *OperationProvider {
	p.kubeCtlProvider.WithClusterCli(clusterContext.Cli)
	p.glooCtlProvider.WithClusterContext(clusterContext)
	return p
}

// WithGlooGatewayContext sets the provider, and all of it's sub-providers, to point to the provided installation
func (p *OperationProvider) WithGlooGatewayContext(ggCtx *gloogateway.Context) *OperationProvider {
	p.glooCtlProvider.WithGlooGatewayContext(ggCtx)
	return p
}

func (p *OperationProvider) KubeCtl() *kubectl.OperationProvider {
	return p.kubeCtlProvider
}

func (p *OperationProvider) GlooCtl() glooctl.OperationProvider {
	return p.glooCtlProvider
}
