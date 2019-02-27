package knative_test

import (
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/solo-kit/pkg/utils/log"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

// TODO(ilackarms): tie testrunner to solo CI test containers and then handle image tagging
const defaultTestRunnerImage = "soloio/testrunner:latest"

func TestKnative(t *testing.T) {
	if kube2e.AreTestsDisabled() {
		return
	}
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	RunSpecs(t, "Knative Suite")
}

var namespace, version string
var testRunnerPort int32 = 1234

var _ = BeforeSuite(func() {

	var err error
	version, err = kube2e.GetTestVersion()
	Expect(err).NotTo(HaveOccurred())
	log.Debugf("gloo test version is: %s", version)

	namespace = version

	err = kube2e.GlooctlInstall(namespace, version, kube2e.KNATIVE)
	Expect(err).NotTo(HaveOccurred())

	err = helpers.DeployTestRunner(namespace, defaultTestRunnerImage, testRunnerPort)
	Expect(err).NotTo(HaveOccurred())
	log.Debugf("successfully deployed test runner pod to namespace: %s", namespace)
})

var _ = AfterSuite(func() {
	err := kube2e.GlooctlUninstall(namespace)
	Expect(err).NotTo(HaveOccurred())
})
