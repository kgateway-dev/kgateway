package knative_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/gloo/test/ginkgo/parallel"

	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/go-utils/testutils/exec"
	"github.com/solo-io/k8s-utils/testutils/helper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

func TestKnative(t *testing.T) {
	// Knative support has been deprecated in Gloo Edge 1.10 (https://github.com/solo-io/gloo/issues/5707)
	// and will be removed in Gloo Edge 1.11.
	// These tests are not run during CI.
	if true {
		log.Warnf("Knative is deprecated and this test is disabled.")
		return
	}

	helpers.RegisterGlooDebugLogPrintHandlerAndClearLogs()
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	RunSpecs(t, "Knative Suite")
}

var (
	testHelper *helper.SoloTestHelper
	ctx        context.Context
	cancel     context.CancelFunc
)

var _ = BeforeSuite(func() {
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())
	ctx, cancel = context.WithCancel(context.Background())

	randomNumber := time.Now().Unix() % 10000
	testHelper, err = helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
		defaults.RootDir = filepath.Join(cwd, "../../..")
		defaults.HelmChartName = "gloo"
		defaults.InstallNamespace = "knative-test-" + fmt.Sprintf("%d-%d", randomNumber, parallel.GetParallelProcessCount())
		return defaults
	})
	Expect(err).NotTo(HaveOccurred())

	skhelpers.RegisterPreFailHandler(helpers.KubeDumpOnFail(GinkgoWriter, "knative-serving", testHelper.InstallNamespace))
	testHelper.Verbose = true

	// Define helm overrides
	valuesOverrideFile, cleanupFunc := getHelmValuesOverrideFile()
	defer cleanupFunc()

	// Install Gloo
	err = testHelper.InstallGloo(ctx, helper.KNATIVE, 5*time.Minute, helper.ExtraArgs("--values", valuesOverrideFile))
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	if os.Getenv("TEAR_DOWN") == "true" {
		err := testHelper.UninstallGlooAll()
		Expect(err).NotTo(HaveOccurred())

		// TODO go-utils should expose `glooctl uninstall --delete-namespace`
		testutils.Kubectl("delete", "namespace", testHelper.InstallNamespace)

		Eventually(func() error {
			return testutils.Kubectl("get", "namespace", testHelper.InstallNamespace)
		}, "60s", "1s").Should(HaveOccurred())
		cancel()
	}
})

func getHelmValuesOverrideFile() (filename string, cleanup func()) {
	values, err := os.CreateTemp("", "values-*.yaml")
	Expect(err).NotTo(HaveOccurred())

	// disabling panic threshold
	// https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/load_balancing/panic_threshold.html
	_, err = values.Write([]byte(`
gatewayProxies:
  gatewayProxy:
    healthyPanicThreshold: 0
`))
	Expect(err).NotTo(HaveOccurred())

	err = values.Close()
	Expect(err).NotTo(HaveOccurred())

	return values.Name(), func() { _ = os.Remove(values.Name()) }
}

func deployKnativeTestService(filePath string) {
	b, err := os.ReadFile(filePath)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	// The webhook may take a bit of time to initially be responsive
	// See: https://github.com/istio/istio/pull/7743/files
	EventuallyWithOffset(1, func() error {
		return exec.RunCommandInput(string(b), testHelper.RootDir, true, "kubectl", "apply", "-f", "-")
	}, "30s", "5s").Should(BeNil())
}

func deleteKnativeTestService(filePath string) error {
	b, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	err = exec.RunCommandInput(string(b), testHelper.RootDir, true, "kubectl", "delete", "-f", "-")
	if err != nil {
		return err
	}
	return nil
}
