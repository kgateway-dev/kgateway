package example_test

import (
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/assertions"
	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"
	"github.com/solo-io/gloo/test/kubernetes/testutils/operations/provider"
	"github.com/solo-io/gloo/test/kubernetes/testutils/runtime"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"

	"testing"

	. "github.com/onsi/ginkgo/v2"
)

func TestExampleSuite(t *testing.T) {
	helpers.RegisterGlooDebugLogPrintHandlerAndClearLogs()
	skhelpers.RegisterCommonFailHandlers()

	RunSpecs(t, "Example Suite")
}

var _ = BeforeSuite(func(ctx SpecContext) {
	runtimeContext := runtime.NewContext()

	// Construct the cluster.Context for this suite
	clusterContext := cluster.MustKindContext(runtimeContext.ClusterName)

	// Create an operator which is responsible for executing operations against the cluster
	operator := operations.NewGinkgoOperator()

	// Create an operations provider, and point it to the running cluster
	operationsProvider := provider.NewOperationProvider().WithClusterContext(clusterContext)

	// Create an assertions provider, and point it to the running cluster
	assertionProvider := assertions.NewProvider().WithClusterContext(clusterContext)

	e2e.Store(ctx, &e2e.SuiteContext{
		Operator:           operator,
		OperationsProvider: operationsProvider,
		AssertionProvider:  assertionProvider,
	})
})
