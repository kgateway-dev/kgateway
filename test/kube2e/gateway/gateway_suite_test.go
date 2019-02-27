package gateway_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/solo-kit/pkg/utils/log"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
	"testing"
)

// TODO(ilackarms): tie testrunner to solo CI test containers and then handle image tagging
const defaultTestRunnerImage = "soloio/testrunner:latest"

func TestGateway(t *testing.T) {
	if kube2e.AreTestsDisabled() {
		return
	}
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	RunSpecs(t, "Gateway Suite")
}

var namespace string
var testRunnerPort int32 = 1234

var _ = BeforeSuite(func() {

	version, err := kube2e.GetTestVersion()
	Expect(err).NotTo(HaveOccurred())
	log.Debugf("gloo test version is: %s", version)

	namespace = version

	err = kube2e.GlooctlInstall(namespace, version, kube2e.GATEWAY)
	Expect(err).NotTo(HaveOccurred())

	err = helpers.DeployTestRunner(namespace, defaultTestRunnerImage, testRunnerPort)
	Expect(err).NotTo(HaveOccurred())
	log.Debugf("successfully deployed test runner pod to namespace: %s", namespace)
})

var _ = AfterSuite(func() {
	err := kube2e.GlooctlUninstall(namespace)
	Expect(err).NotTo(HaveOccurred())
})
