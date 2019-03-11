package gateway_test

import (
	"github.com/solo-io/go-utils/testutils"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/go-utils/testutils/install"
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

var helper *install.SoloTestHelper

var _ = BeforeSuite(func() {
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())

	helper, err = install.NewSoloTestHelper(func(defaults install.TestConfig) install.TestConfig {
		defaults.RootDir = filepath.Join(cwd, "../../..")
		defaults.HelmChartName = "gloo"
		return defaults
	})
	Expect(err).NotTo(HaveOccurred())

	// Install Gloo
	err = helper.InstallGloo(install.GATEWAY, 5*time.Minute)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	err := helper.UninstallGloo()
	Expect(err).NotTo(HaveOccurred())

	EventuallyWithOffset(1, func() error {
		return testutils.Kubectl("get", "namespace", helper.InstallNamespace)
	}, "60s", "1s").Should(HaveOccurred())
})
