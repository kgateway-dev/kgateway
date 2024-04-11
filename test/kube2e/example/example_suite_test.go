package example

import (
	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/gloo/test/testutils/kubeutils"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
)

func TestExampleSuite(t *testing.T) {
	RunSpecs(t, "Example Suite")
}

var (
	clusterContext *kubeutils.ClusterContext
)

var _ = BeforeSuite(func() {
	clusterContext = kubeutils.MustKindClusterContext(os.Getenv(testutils.ClusterName))

})

var _ = AfterSuite(func() {

})
