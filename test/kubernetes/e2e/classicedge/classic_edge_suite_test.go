package classicedge_test

import (
	"context"
	"testing"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/cluster"
	"github.com/solo-io/gloo/test/kubernetes/testutils/runtime"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"

	. "github.com/onsi/ginkgo/v2"
)

func TestClassicGatewaySuite(t *testing.T) {
	skhelpers.RegisterCommonFailHandlers()

	RunSpecs(t, "Classic Edge Gateway Suite")
}

var (
	testCluster *e2e.TestCluster
)

var _ = BeforeSuite(func(ctx context.Context) {
	runtimeContext := runtime.NewContext()

	// Construct the cluster.Context for this suite
	clusterContext := cluster.MustKindContext(runtimeContext.ClusterName)

	testCluster = &e2e.TestCluster{
		RuntimeContext: runtimeContext,
		ClusterContext: clusterContext,
	}

	// Register the PreFailHandler from the TestSuite
	skhelpers.RegisterPreFailHandler(testCluster.PreFailHandler)
})
