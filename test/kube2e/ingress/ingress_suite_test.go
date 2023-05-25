package ingress_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/solo-io/gloo/test/ginkgo/parallel"

	"github.com/solo-io/gloo/test/kube2e"

	"github.com/solo-io/gloo/test/helpers"

	"github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/k8s-utils/testutils/helper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

func TestIngress(t *testing.T) {
	helpers.RegisterGlooDebugLogPrintHandlerAndClearLogs()
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	RunSpecs(t, "Ingress Suite")
}

var (
	testHelper *helper.SoloTestHelper
	ctx        context.Context
	cancel     context.CancelFunc
)

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.Background())
	var err error
	randomNumber := time.Now().Unix() % 10000
	testHelper, err = kube2e.GetTestHelper(ctx, "ingress-test-"+fmt.Sprintf("%d-%d", randomNumber, parallel.GetParallelProcessCount()))
	Expect(err).NotTo(HaveOccurred())
	skhelpers.RegisterPreFailHandler(helpers.KubeDumpOnFail(GinkgoWriter, testHelper.InstallNamespace))
	testHelper.Verbose = true

	// Define helm overrides
	valuesOverrideFile, cleanupFunc := getHelmValuesOverrideFile()
	defer cleanupFunc()

	// Install Gloo
	err = testHelper.InstallGloo(ctx, helper.INGRESS, 5*time.Minute, helper.ExtraArgs("--values", valuesOverrideFile))
	Expect(err).NotTo(HaveOccurred())
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
