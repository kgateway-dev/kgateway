package helm_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/go-utils/testutils/helper"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

func TestHelm(t *testing.T) {
	if testutils.AreTestsDisabled() {
		return
	}
	if os.Getenv("CLUSTER_LOCK_TESTS") == "1" {
		log.Warnf("This test does not require using a cluster lock. Cluster lock is enabled so this test is disabled. " +
			"To enable, unset CLUSTER_LOCK_TESTS in your env.")
		return
	}
	helpers.RegisterGlooDebugLogPrintHandlerAndClearLogs()
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	RunSpecs(t, "Helm Suite")
}

const (
	helmValues = `
global:
  image:
    pullPolicy: IfNotPresent
  glooRbac:
    namespaced: true
    nameSuffix: e2e-test-rbac-suffix
settings:
  singleNamespace: true
  create: true
gloo:
  deployment:
    disableUsageStatistics: true
`
)

var testHelper *helper.SoloTestHelper

var _ = BeforeSuite(StartTestHelper)
var _ = AfterSuite(func() {
	helpers.TearDownTestHelper(testHelper)
})

func StartTestHelper() {
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())

	randomNumber := time.Now().Unix() % 10000
	testHelper, err = helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
		defaults.RootDir = filepath.Join(cwd, "../../..")
		defaults.HelmChartName = "gloo"
		defaults.InstallNamespace = "helm-test-" + fmt.Sprintf("%d-%d", randomNumber, GinkgoParallelNode())
		defaults.Verbose = true
		return defaults
	})
	Expect(err).NotTo(HaveOccurred())

	// Register additional fail handlers
	skhelpers.RegisterPreFailHandler(helpers.KubeDumpOnFail(GinkgoWriter, "knative-serving", testHelper.InstallNamespace))
	valueOverrideFile, cleanupFunc := kube2e.GetHelmValuesOverrideFile(helmValues)
	defer cleanupFunc()

	// install gloo with helm
	runAndCleanCommand("kubectl", "create", "namespace", testHelper.InstallNamespace)
	runAndCleanCommand("helm", "repo", "add", testHelper.HelmChartName, "https://storage.googleapis.com/solo-public-helm")
	runAndCleanCommand("helm", "repo", "update")
	runAndCleanCommand("helm", "install", testHelper.HelmChartName, "gloo/gloo",
		"--namespace", testHelper.InstallNamespace,
		"--values", valueOverrideFile,
		"--version", "v1.3.0")

	// Check that everything is OK
	kube2e.GlooctlCheckEventuallyHealthy(testHelper, "40s")

}

func runAndCleanCommand(name string, arg ...string) []byte {
	cmd := exec.Command(name, arg...)
	b, err := cmd.Output()
	// for debugging in Cloud Build
	if err != nil {
		if v, ok := err.(*exec.ExitError); ok {
			fmt.Println("ExitError: ", string(v.Stderr))
		}
	}
	Expect(err).To(BeNil())
	cmd.Process.Kill()
	cmd.Process.Release()
	return b
}
