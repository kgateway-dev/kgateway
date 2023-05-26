package kubernetes_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/test/gomega"
	"github.com/solo-io/solo-kit/pkg/utils/statusutils"
)

var (
	namespace = "kubernetes-test-ns"

	_ = BeforeSuite(func() {
		// necessary for non-default namespace
		err := os.Setenv(statusutils.PodNamespaceEnvName, namespace)
		Expect(err).NotTo(HaveOccurred())
	})

	_ = AfterSuite(func() {
		err := os.Unsetenv(statusutils.PodNamespaceEnvName)
		Expect(err).NotTo(HaveOccurred())
	})
)

func TestKubernetes(t *testing.T) {
	SetAsyncAssertionDefaults(AsyncAssertionDefaults{})
	RegisterFailHandler(Fail)

	RunSpecs(t, "Kubernetes Suite")
}
