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

var namespace string

var _ = BeforeSuite(func() {
	namespace = kube2e.InstallGloo(kube2e.KNATIVE)
})

var _ = AfterSuite(func() {
	err := kube2e.GlooctlUninstall(namespace)
	Expect(err).NotTo(HaveOccurred())
})
