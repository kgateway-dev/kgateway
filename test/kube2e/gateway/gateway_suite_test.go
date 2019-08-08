package gateway_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/gloo/test/helpers"

	"github.com/avast/retry-go"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/go-utils/testutils/clusterlock"

	"github.com/solo-io/go-utils/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/go-utils/testutils/helper"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

func TestGateway(t *testing.T) {
	if testutils.AreTestsDisabled() {
		return
	}
	if os.Getenv("CLUSTER_LOCK_TESTS") == "1" {
		log.Warnf("This test does not require using a cluster lock. cluster lock is enabled so this test is disabled. " +
			"To enable, unset CLUSTER_LOCK_TESTS in your env.")
		return
	}
	helpers.RegisterGlooDebugLogPrintHandlerAndClearLogs()
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	RunSpecs(t, "Gateway Suite")
}

var testHelper *helper.SoloTestHelper
var locker *clusterlock.TestClusterLocker

var _ = BeforeSuite(func() {
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())

	testHelper, err = helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
		defaults.RootDir = filepath.Join(cwd, "../../..")
		defaults.HelmChartName = "gloo"
		defaults.InstallNamespace = "gateway_test"
		return defaults
	})
	Expect(err).NotTo(HaveOccurred())

	RegisterFailHandler(helpers.KubeDumpOnFail(GinkgoWriter, "knative-serving", testHelper.InstallNamespace))
	testHelper.Verbose = true

	locker, err = clusterlock.NewTestClusterLocker(kube2e.MustKubeClient(), clusterlock.Options{})
	Expect(err).NotTo(HaveOccurred())
	Expect(locker.AcquireLock(retry.Attempts(40))).NotTo(HaveOccurred())

	values, err := ioutil.TempFile("", "*.yaml")
	Expect(err).NotTo(HaveOccurred())
	values.Write([]byte("rbac:\n  namespaced: true\n"))
	values.Close()

	err = testHelper.InstallGloo(helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", values.Name()))
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	if locker != nil {
		defer locker.ReleaseLock()
	}

	if testHelper != nil {
		err := testHelper.UninstallGloo()
		Expect(err).NotTo(HaveOccurred())

		// TODO go-utils should expose `glooctl uninstall --delete-namespace`
		_ = testutils.Kubectl("delete", "namespace", testHelper.InstallNamespace)

		Eventually(func() error {
			return testutils.Kubectl("get", "namespace", testHelper.InstallNamespace)
		}, "60s", "1s").Should(HaveOccurred())
	}
})
