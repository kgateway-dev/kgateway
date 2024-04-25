package actions

import (
	"context"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
	"time"
)

// Provider is the entity that creates actions.
// These actions are executed against a running installation of Gloo Gateway, within a Kubernetes Cluster.
// This provider is just a wrapper around sub-providers, so it exposes methods to access those providers
type Provider struct {
	kubeCli *kubectl.Cli

	testHelper *helper.SoloTestHelper

	glooGatewayContext *gloogateway.Context
}

// NewActionsProvider returns an Provider
func NewActionsProvider() *Provider {
	return &Provider{
		kubeCli:    nil,
		testHelper: nil,
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

// WithTestHelper sets the SoloTestHelper on the Provider
// NOTE: This is not the long-term solution, as we want to rely on tools that users can execute
// However, it is the current solution that our tests rely on, so we are using it for now
func (p *Provider) WithTestHelper(testHelper *helper.SoloTestHelper) *Provider {
	p.testHelper = testHelper
	return p
}

func (p *Provider) Kubectl() *kubectl.Cli {
	return p.kubeCli
}

func (p *Provider) TestHelper() *helper.SoloTestHelper {
	return p.testHelper
}

func (p *Provider) TestHelperInstall(ctx context.Context) error {
	return p.testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", p.glooGatewayContext.ValuesManifestFile))
}

func (p *Provider) TestHelperUninstall(_ context.Context) error {
	return p.testHelper.UninstallGlooAll()
}
