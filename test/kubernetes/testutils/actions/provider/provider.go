package provider

import (
	"github.com/solo-io/gloo/test/kubernetes/testutils/actions/glooctl"
	"github.com/solo-io/gloo/test/kubernetes/testutils/actions/kubectl"

	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
)

// ActionsProvider is the entity that creates actions.
// These actions are executed against a running installation of Gloo Gateway, within a Kubernetes Cluster.
// This provider is just a wrapper around sub-providers, so it exposes methods to access those providers
type ActionsProvider struct {
	kubectlProvider *kubectl.Provider
	glooctlProvider glooctl.Provider
}

// NewActionsProvider returns an ActionsProvider
func NewActionsProvider() *ActionsProvider {
	return &ActionsProvider{
		kubectlProvider: kubectl.NewProvider(),
		glooctlProvider: glooctl.NewProvider(),
	}
}

// WithClusterContext sets the provider, and all of it's sub-providers, to point to the provided cluster
func (p *ActionsProvider) WithClusterContext(clusterContext *cluster.Context) *ActionsProvider {
	p.kubectlProvider.WithClusterCli(clusterContext.Cli)
	p.glooctlProvider.WithClusterContext(clusterContext)
	return p
}

// WithGlooGatewayContext sets the provider, and all of it's sub-providers, to point to the provided installation
func (p *ActionsProvider) WithGlooGatewayContext(ggCtx *gloogateway.Context) *ActionsProvider {
	p.glooctlProvider.WithGlooGatewayContext(ggCtx)
	return p
}

// WithGlooctlProvider sets the glooctl provider on this ActionsProvider
func (p *ActionsProvider) WithGlooctlProvider(provider glooctl.Provider) *ActionsProvider {
	p.glooctlProvider = provider
	return p
}

func (p *ActionsProvider) Kubectl() *kubectl.Provider {
	return p.kubectlProvider
}

func (p *ActionsProvider) Glooctl() glooctl.Provider {
	return p.glooctlProvider
}
