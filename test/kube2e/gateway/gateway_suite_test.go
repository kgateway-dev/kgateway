package gateway_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/gloo/test/helpers"

	"github.com/solo-io/go-utils/log"
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
		log.Warnf("This test does not require using a cluster lock. cluster lock is enabled so this test is disabled. " +
			"To enable, unset CLUSTER_LOCK_TESTS in your env.")
		return
	}
	helpers.RegisterGlooDebugLogPrintHandlerAndClearLogs()
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	RunSpecs(t, "Gateway Suite")
}

var (
	testHelper   *helper.SoloTestHelper
	testInstance int
	values       *os.File
	randomNumber = time.Now().Unix() % 10000
)

func StartTestHelper() {

	testInstance += 1
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())

	testHelper, err = helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
		defaults.RootDir = filepath.Join(cwd, "../../..")
		defaults.HelmChartName = "gloo"
		// TODO: include build id?
		defaults.InstallNamespace = "gateway-test-" + fmt.Sprintf("%d-%d-%d", randomNumber, GinkgoParallelNode(), testInstance)
		return defaults
	})
	Expect(err).NotTo(HaveOccurred())

	skhelpers.RegisterPreFailHandler(helpers.KubeDumpOnFail(GinkgoWriter, "knative-serving", testHelper.InstallNamespace))
	testHelper.Verbose = true

	values, err = ioutil.TempFile("", "*.yaml")
	Expect(err).NotTo(HaveOccurred())
	values.Write([]byte("global:\n  glooRbac:\n    namespaced: true\nsettings:\n  singleNamespace: true\n  create: true\n"))
	values.Close()

	err = testHelper.InstallGloo(helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", values.Name()))
	Expect(err).NotTo(HaveOccurred())
}

func TearDownTestHelper() {
	if values != nil {
		os.Remove(values.Name())
	}
	if testHelper != nil {
		err := testHelper.UninstallGloo()
		Expect(err).NotTo(HaveOccurred())
		_ = testutils.Kubectl("delete", "--wait=false", "namespace", testHelper.InstallNamespace)
	}
}
