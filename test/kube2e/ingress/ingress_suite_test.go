package ingress_test

import (
	"github.com/solo-io/gloo/test/kube2e"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

// TODO(ilackarms): tie testrunner to solo CI test containers and then handle image tagging
const defaultTestRunnerImage = "soloio/testrunner:latest"

func TestIngress(t *testing.T) {
	if kube2e.AreTestsDisabled() {
		return
	}
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	RunSpecs(t, "Ingress Suite")
}

var namespace string
var testRunnerPort int32 = 1234

var _ = BeforeSuite(func() {
	namespace = kube2e.InstallGloo(kube2e.INGRESS)
})

var _ = AfterSuite(func() {
	err := kube2e.GlooctlUninstall(namespace)
	Expect(err).NotTo(HaveOccurred())
})
