//go:build ignore

package actions

import (
	"github.com/kgateway-dev/kgateway/pkg/utils/helmutils"
	"github.com/kgateway-dev/kgateway/pkg/utils/kubeutils/kubectl"
	"github.com/kgateway-dev/kgateway/projects/gloo/cli/pkg/testutils"
	"github.com/kgateway-dev/kgateway/test/kubernetes/testutils/cluster"
	"github.com/kgateway-dev/kgateway/test/kubernetes/testutils/kgateway"
)

// Provider is the entity that creates actions.
// These actions are executed against a running installation of Gloo Gateway, within a Kubernetes Cluster.
// This provider is just a wrapper around sub-providers, so it exposes methods to access those providers
type Provider struct {
	kubeCli *kubectl.Cli
	glooCli *testutils.GlooCli
	helmCli *helmutils.Client

	glooGatewayContext *kgateway.Context
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
func (p *Provider) WithGlooGatewayContext(ggContext *kgateway.Context) *Provider {
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
