package k8sgateway_test

import (
	"context"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
	"github.com/solo-io/gloo/test/kubernetes/testutils/runtime"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"

	"testing"

	. "github.com/onsi/ginkgo/v2"
)

func TestK8sGatewaySuite(t *testing.T) {
	skhelpers.RegisterCommonFailHandlers()

	RunSpecs(t, "K8s Gateway Suite")
}

var (
	testCluster *e2e.TestCluster
)

var _ = BeforeSuite(func(ctx context.Context) {
	runtimeContext := runtime.NewContext()

	// We try to isolate the usage of Ginkgo to only where are tests are invoked
	testingFramework := GinkgoTB()
	testingProgressWriter := GinkgoWriter

	// Construct the cluster.Context for this suite
	clusterContext := cluster.MustKindContext(
		testingFramework,
		testingProgressWriter,
		runtimeContext.ClusterName)

	testCluster = &e2e.TestCluster{
		TestingFramework:      testingFramework,
		TestingProgressWriter: testingProgressWriter,
		RuntimeContext:        runtimeContext,
		ClusterContext:        clusterContext,
	}

	// Register the PreFailHandler from the TestSuite
	skhelpers.RegisterPreFailHandler(testCluster.PreFailHandler)
})
