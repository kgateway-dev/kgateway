package glooctl

import (
	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kubernetes/testutils/actions"

	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
)

// Provider defines the standard operations that can be executed via glooctl
// In a perfect world, all operations would be isolated to the OSS repository
// Since there are some custom Enterprise operations, we define this as an interface,
// so that Gloo Gateway Enterprise tests can rely on a custom implementation
type Provider interface {
	WithClusterContext(clusterContext *cluster.Context) Provider
	WithGlooGatewayContext(ggCtx *gloogateway.Context) Provider

	NewTestHelperInstallAction() actions.ClusterAction
	NewTestHelperUninstallAction() actions.ClusterAction
	ExportReport() actions.ClusterAction
}

// providerImpl is the implementation of the Provider for Gloo Gateway Open Source
type providerImpl struct {
	clusterContext     *cluster.Context
	glooGatewayContext *gloogateway.Context
}

func NewProvider() Provider {
	return &providerImpl{
		clusterContext:     nil,
		glooGatewayContext: nil,
	}
}

// WithClusterContext sets the Provider to point to the provided cluster
func (p *providerImpl) WithClusterContext(clusterContext *cluster.Context) Provider {
	p.clusterContext = clusterContext
	return p
}

// WithGlooGatewayContext sets the Provider to point to the provided installation of Gloo Gateway
func (p *providerImpl) WithGlooGatewayContext(ggCtx *gloogateway.Context) Provider {
	p.glooGatewayContext = ggCtx
	return p
}

// requiresGlooGatewayContext is invoked by methods on the Provider that can only be invoked
// if the provider has been configured to point to a Gloo Gateway installation
// There are certain actions that can be invoked that do not require that Gloo Gateway be installed for them to be invoked
func (p *providerImpl) requiresGlooGatewayContext() {
	gomega.Expect(p.glooGatewayContext).NotTo(gomega.BeNil(), "Provider attempted to create an action that requires a Gloo Gateway installation, but none was configured")
}
