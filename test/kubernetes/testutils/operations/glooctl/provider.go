package glooctl

import (
	"testing"

	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"
)

// OperationProvider defines the standard operations that can be executed via glooctl
// In a perfect world, all operations would be isolated to the OSS repository
// Since there are some custom Enterprise operations, we define this as an interface,
// so that Gloo Gateway Enterprise tests can rely on a custom implementation
type OperationProvider interface {
	WithClusterContext(clusterContext *cluster.Context) OperationProvider
	WithGlooGatewayContext(ggCtx *gloogateway.Context) OperationProvider

	NewTestHelperInstallOperation(provider *assertions.Provider) operations.Operation
	NewTestHelperUninstallOperation() operations.Operation
	ExportReport() operations.Operation
}

// operationProviderImpl is the implementation of the OperationProvider for Gloo Gateway Open Source
type operationProviderImpl struct {
	testingFramework testing.TB

	clusterContext     *cluster.Context
	glooGatewayContext *gloogateway.Context
}

func NewProvider(testingFramework testing.TB) OperationProvider {
	return &operationProviderImpl{
		testingFramework: testingFramework,

		clusterContext:     nil,
		glooGatewayContext: nil,
	}
}

// WithClusterContext sets the OperationProvider to point to the provided cluster
func (p *operationProviderImpl) WithClusterContext(clusterContext *cluster.Context) OperationProvider {
	p.clusterContext = clusterContext
	return p
}

// WithGlooGatewayContext sets the OperationProvider to point to the provided installation of Gloo Gateway
func (p *operationProviderImpl) WithGlooGatewayContext(ggCtx *gloogateway.Context) OperationProvider {
	p.glooGatewayContext = ggCtx
	return p
}

// requiresGlooGatewayContext is invoked by methods on the Provider that can only be invoked
// if the provider has been configured to point to a Gloo Gateway installation
// There are certain Assertions that can be invoked that do not require that Gloo Gateway be installed for them to be invoked
func (p *operationProviderImpl) requiresGlooGatewayContext() {
	if p.glooGatewayContext == nil {
		p.testingFramework.Fatal("Provider attempted to create an Operation that requires a Gloo Gateway installation, but none was configured")
	}
}
