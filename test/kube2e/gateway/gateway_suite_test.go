package gateway_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kube2e"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

func TestGateway(t *testing.T) {
	if kube2e.AreTestsDisabled() {
		return
	}
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	RunSpecs(t, "Gateway Suite")
}

var namespace string

var _ = BeforeSuite(func() {
	namespace = kube2e.InstallGloo(kube2e.GATEWAY)
})

var _ = AfterSuite(func() {
	err := kube2e.GlooctlUninstall(namespace)
	Expect(err).NotTo(HaveOccurred())

})
