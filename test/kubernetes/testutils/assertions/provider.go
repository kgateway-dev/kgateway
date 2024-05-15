package assertions

import (
	"testing"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
)

// Provider is the entity that provides methods which assert behaviors of a Kubernetes Cluster
// These assertions occur against a running instance of Gloo Gateway, within a Kubernetes Cluster.
type Provider struct {
	Assert  *assert.Assertions
	Require *require.Assertions

	// Gomega is well-used around the codebase, so we also add support here
	// NOTE TO DEVELOPERS: We recommend relying on testify assertions where possible
	Gomega gomega.Gomega

	clusterContext     *cluster.Context
	glooGatewayContext *gloogateway.Context
}

// NewProvider returns a Provider that will provide Assertions that can be executed against an
// installation of Gloo Gateway
func NewProvider(t *testing.T) *Provider {
	gomega.RegisterTestingT(t)
	return &Provider{
		Assert:  assert.New(t),
		Require: require.New(t),
		Gomega:  gomega.NewWithT(t),

		clusterContext:     nil,
		glooGatewayContext: nil,
	}
}

// WithClusterContext sets the provider to point to the provided cluster
func (p *Provider) WithClusterContext(clusterContext *cluster.Context) *Provider {
	p.clusterContext = clusterContext
	return p
}

// WithGlooGatewayContext sets the providers to point to a particular installation of Gloo Gateway
func (p *Provider) WithGlooGatewayContext(ggCtx *gloogateway.Context) *Provider {
	p.glooGatewayContext = ggCtx
	return p
}

// expectGlooGatewayContextDefined is invoked by methods on the Provider that can only be invoked
// if the provider has been configured to point to a Gloo Gateway installation
// There are certain Assertions that can be invoked that do not require that Gloo Gateway be installed for them to be invoked
func (p *Provider) expectGlooGatewayContextDefined() {
	p.Require.NotNil(p.glooGatewayContext, "Provider attempted to create an Assertion that requires a Gloo Gateway installation, but none was configured")
}
