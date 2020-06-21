package gateway_mtls_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/go-utils/log"

	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e"

	"github.com/solo-io/go-utils/testutils/helper"

	"github.com/solo-io/go-utils/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

func TestGateway(t *testing.T) {
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
	RunSpecs(t, "Gateway MTLS Suite")
}

var testHelper *helper.SoloTestHelper

var _ = BeforeSuite(StartTestHelper)
var _ = AfterSuite(func() {
	helpers.TearDownTestHelper(testHelper)
})

const (
	helmValues = `
global:
  image:
    pullPolicy: IfNotPresent
  glooRbac:
    namespaced: true
    nameSuffix: e2e-test-rbac-suffix
  glooMtls:
    enabled: true
    sds:
      image:
        registry: quay.io/solo-io
settings:
  singleNamespace: true
  create: true
gloo:
  deployment:
    disableUsageStatistics: true
`
)

func StartTestHelper() {
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())

	randomNumber := time.Now().Unix() % 10000
	testHelper, err = helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
		defaults.RootDir = filepath.Join(cwd, "../../..")
		defaults.HelmChartName = "gloo"
		defaults.InstallNamespace = "gateway-test-" + fmt.Sprintf("%d-%d", randomNumber, GinkgoParallelNode())
		defaults.Verbose = true
		return defaults
	})
	Expect(err).NotTo(HaveOccurred())

	// Register additional fail handlers
	skhelpers.RegisterPreFailHandler(helpers.KubeDumpOnFail(GinkgoWriter, "knative-serving", testHelper.InstallNamespace))
	valueOverrideFile, cleanupFunc := kube2e.GetHelmValuesOverrideFile(helmValues)
	defer cleanupFunc()

	err = testHelper.InstallGloo(helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", valueOverrideFile))
	Expect(err).NotTo(HaveOccurred())

	// Check that everything is OK
	kube2e.GlooctlCheckEventuallyHealthy(testHelper, "40s")

	// TODO(marco): explicitly enable strict validation, this can be removed once we enable validation by default
	// See https://github.com/solo-io/gloo/issues/1374
	helpers.UpdateAlwaysAcceptSetting(testHelper.InstallNamespace, false)

	// Ensure gloo reaches valid state and doesn't continually resync
	// we can consider doing the same for leaking go-routines after resyncs
	helpers.EventuallyReachesConsistentState(testHelper)
}
