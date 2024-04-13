package assertions

import (
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
	"testing"

	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
)

// Provider is the entity that creates operations.DiscreteAssertion
// These assertions occur against a running instance of Gloo Gateway, within a Kubernetes Cluster.
// So this provider maintains state about the install/cluster it is using, and then provides
// operations.DiscreteAssertion to match
type Provider struct {
	testingFramework testing.TB

	clusterContext     *cluster.Context
	glooGatewayContext *gloogateway.Context
}

// NewProvider returns a Provider that will fail because it is not configured with a Kubernetes Cluster
func NewProvider() *Provider {
	return &Provider{
		testingFramework:   nil,
		clusterContext:     nil,
		glooGatewayContext: nil,
	}
}

// WithTestingFramework sets the testing framework used by the assertion provider
func (p *Provider) WithTestingFramework(t testing.TB) *Provider {
	p.testingFramework = t
	return p
}

// WithClusterContext sets the provider to point to the provided cluster
func (p *Provider) WithClusterContext(clusterContext *cluster.Context) *Provider {
	p.clusterContext = clusterContext
	return p
}

// WithGlooGatewayContext sets the providers to point to a particualr installation of Gloo Gateway
func (p *Provider) WithGlooGatewayContext(ggCtx *gloogateway.Context) *Provider {
	p.glooGatewayContext = ggCtx
	return p
}
