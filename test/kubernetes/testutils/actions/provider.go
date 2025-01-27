package actions

import (
	"github.com/solo-io/gloo/pkg/utils/helmutils"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
)

// Provider is the entity that creates actions.
// These actions are executed against a running installation of Gloo Gateway, within a Kubernetes Cluster.
// This provider is just a wrapper around sub-providers, so it exposes methods to access those providers
type Provider struct {
	kubeCli *kubectl.Cli
	glooCli *testutils.GlooCli
	helmCli *helmutils.Client

	glooGatewayContext *gloogateway.Context
}

// NewActionsProvider returns an Provider
func NewActionsProvider() *Provider {
	return &Provider{
		kubeCli:            nil,
		glooCli:            testutils.NewGlooCli(),
		helmCli:            helmutils.NewClient(),
		glooGatewayContext: nil,
	}
}

// WithClusterContext sets the provider to point to the provided cluster
func (p *Provider) WithClusterContext(clusterContext *cluster.Context) *Provider {
	p.kubeCli = clusterContext.Cli
	return p
}

// WithGlooGatewayContext sets the provider to point to the provided Gloo Gateway installation
func (p *Provider) WithGlooGatewayContext(ggContext *gloogateway.Context) *Provider {
	p.glooGatewayContext = ggContext
	return p
}

func (p *Provider) Kubectl() *kubectl.Cli {
	return p.kubeCli
}

func (p *Provider) Glooctl() *testutils.GlooCli {
	return p.glooCli
}

func (p *Provider) Helm() *helmutils.Client {
	return p.helmCli
}
