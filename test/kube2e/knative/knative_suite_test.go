package knative_test

import (
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/go-utils/testutils/clusterlock"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/go-utils/testutils/helper"

	"github.com/solo-io/go-utils/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

func TestKnative(t *testing.T) {
	if testutils.AreTestsDisabled() {
		return
	}
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	RunSpecs(t, "Knative Suite")
}

var testHelper *helper.SoloTestHelper
var locker *clusterlock.TestClusterLocker

var _ = BeforeSuite(func() {
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())

	testHelper, err = helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
		defaults.RootDir = filepath.Join(cwd, "../../..")
		defaults.HelmChartName = "gloo"
		return defaults
	})
	Expect(err).NotTo(HaveOccurred())

	locker, err = clusterlock.NewTestClusterLocker(kube2e.MustKubeClient(), "")
	Expect(err).NotTo(HaveOccurred())
	Expect(locker.AcquireLock()).NotTo(HaveOccurred())

	// Install Gloo
	err = testHelper.InstallGloo(helper.KNATIVE, 5*time.Minute)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	defer locker.ReleaseLock()
	err := testHelper.UninstallGloo()
	Expect(err).NotTo(HaveOccurred())

	Eventually(func() error {
		return testutils.Kubectl("get", "namespace", testHelper.InstallNamespace)
	}, "60s", "1s").Should(HaveOccurred())
})
