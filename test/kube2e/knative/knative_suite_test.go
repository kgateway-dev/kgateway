package knative_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/go-utils/testutils/clusterlock"
	"github.com/solo-io/go-utils/testutils/exec"
	"github.com/solo-io/go-utils/testutils/helper"

	"github.com/avast/retry-go"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

func TestKnative(t *testing.T) {
	if os.Getenv("KUBE2E_TESTS") != "knative" {
		log.Warnf("This test is disabled. " +
			"To enable, set KUBE2E_TESTS to 'knative' in your env.")
		return
	}
	helpers.RegisterGlooDebugLogPrintHandlerAndClearLogs()
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	RunSpecs(t, "Knative Suite")
}

var testHelper *helper.SoloTestHelper
var locker *clusterlock.TestClusterLocker

var _ = BeforeSuite(func() {
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())

	randomNumber := time.Now().Unix() % 10000
	testHelper, err = helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
		defaults.RootDir = filepath.Join(cwd, "../../..")
		defaults.HelmChartName = "gloo"
		defaults.InstallNamespace = "knative-test-" + fmt.Sprintf("%d-%d", randomNumber, GinkgoParallelNode())
		return defaults
	})
	Expect(err).NotTo(HaveOccurred())

	skhelpers.RegisterPreFailHandler(helpers.KubeDumpOnFail(GinkgoWriter, "knative-serving", testHelper.InstallNamespace))
	testHelper.Verbose = true

	locker, err = clusterlock.NewTestClusterLocker(kube2e.MustKubeClient(), clusterlock.Options{})
	Expect(err).NotTo(HaveOccurred())
	Expect(locker.AcquireLock(retry.Attempts(40))).NotTo(HaveOccurred())

	// Install Gloo
	err = testHelper.InstallGloo(helper.KNATIVE, 5*time.Minute)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	defer locker.ReleaseLock()
	err := testHelper.UninstallGlooAll()
	Expect(err).NotTo(HaveOccurred())

	// TODO go-utils should expose `glooctl uninstall --delete-namespace`
	testutils.Kubectl("delete", "namespace", testHelper.InstallNamespace)

	Eventually(func() error {
		return testutils.Kubectl("get", "namespace", testHelper.InstallNamespace)
	}, "60s", "1s").Should(HaveOccurred())
})

func deployKnativeTestService(filePath string) {
	b, err := ioutil.ReadFile(filePath)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	// The webhook may take a bit of time to initially be responsive
	// See: https://github.com/istio/istio/pull/7743/files
	EventuallyWithOffset(1, func() error {
		return exec.RunCommandInput(string(b), testHelper.RootDir, true, "kubectl", "apply", "-f", "-")
	}, "30s", "5s").Should(BeNil())
}

func deleteKnativeTestService(filePath string) error {
	b, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}
	err = exec.RunCommandInput(string(b), testHelper.RootDir, true, "kubectl", "delete", "-f", "-")
	if err != nil {
		return err
	}
	return nil
}
