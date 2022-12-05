package istio_test

import (
	"context"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/k8s-utils/kubeutils"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/utils/statusutils"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/k8s-utils/testutils/helper"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

const (
	gatewayProxy = gatewaydefaults.GatewayProxyName
	gatewayPort  = int(80)
)

func TestIstio(t *testing.T) {
	if os.Getenv("KUBE2E_TESTS") != "istio" {
		log.Warnf("This test is disabled. " +
			"To enable, set KUBE2E_TESTS to 'istio' in your env.")
		return
	}
	helpers.RegisterGlooDebugLogPrintHandlerAndClearLogs()
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	_ = os.Remove(cliutil.GetLogsPath())
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "Istio Suite", []Reporter{junitReporter})
}

var (
	testHelper  *helper.SoloTestHelper
	ctx, cancel = context.WithCancel(context.Background())

	namespace         = defaults.GlooSystem
	resourceClientSet *kube2e.KubeResourceClientSet
)

var _ = BeforeSuite(func() {
	var err error

	// todo - may not need to set the pod namespace, since just using the deafult "gloo-system"
	err = os.Setenv(statusutils.PodNamespaceEnvName, namespace)
	Expect(err).NotTo(HaveOccurred())

	// enabling istio-injection for the test-runner
	createIstioInjectableNamespace(namespace)
	testHelper, err = kube2e.GetTestHelper(ctx, namespace)
	Expect(err).NotTo(HaveOccurred())

	skhelpers.RegisterPreFailHandler(helpers.KubeDumpOnFail(GinkgoWriter, testHelper.InstallNamespace))

	// Install Gloo
	values, cleanup := getHelmOverrides()
	defer cleanup()

	err = testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", values))
	Expect(err).NotTo(HaveOccurred())

	// Check that everything is OK
	kube2e.GlooctlCheckEventuallyHealthy(1, testHelper, "90s")
	// TODO(marco): explicitly enable strict validation, this can be removed once we enable validation by default
	// See https://github.com/solo-io/gloo/issues/1374
	kube2e.UpdateAlwaysAcceptSetting(ctx, false, testHelper.InstallNamespace)

	// Ensure gloo reaches valid state and doesn't continually resync
	// we can consider doing the same for leaking go-routines after resyncs
	kube2e.EventuallyReachesConsistentState(testHelper.InstallNamespace)

	// delete test-runner Service, as the tests create and manage their own
	_ = testutils.Kubectl("delete", "service", helper.TestrunnerName, "-n", namespace)
	EventuallyWithOffset(1, func() error {
		return testutils.Kubectl("get", "service", helper.TestrunnerName, "-n", namespace)
	}, "60s", "1s").Should(HaveOccurred())

	cfg, err := kubeutils.GetConfig("", "")
	Expect(err).NotTo(HaveOccurred())

	resourceClientSet, err = kube2e.NewKubeResourceClientSet(ctx, cfg)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	err := os.Unsetenv(statusutils.PodNamespaceEnvName)
	Expect(err).NotTo(HaveOccurred())

	if os.Getenv("TEAR_DOWN") == "true" {
		err := testHelper.UninstallGlooAll()
		Expect(err).NotTo(HaveOccurred())

		// glooctl should delete the namespace. we do it again just in case it failed
		// ignore errors
		_ = testutils.Kubectl("delete", "namespace", testHelper.InstallNamespace)

		EventuallyWithOffset(1, func() error {
			return testutils.Kubectl("get", "namespace", testHelper.InstallNamespace)
		}, "60s", "1s").Should(HaveOccurred())

		cancel()
	}
})

func createIstioInjectableNamespace(ns string) {
	var err error

	err = testutils.Kubectl("create", "ns", ns)
	Expect(err).NotTo(HaveOccurred())
	err = testutils.Kubectl("label", "namespace", ns, "istio-injection=enabled")
	Expect(err).NotTo(HaveOccurred())
}

func getHelmOverrides() (filename string, cleanup func()) {
	values, err := ioutil.TempFile("", "*.yaml")
	Expect(err).NotTo(HaveOccurred())
	// Set up gloo with istio integration enabled
	_, err = values.Write([]byte(`
global:
  istioIntegration:
    labelInstallNamespace: true
    whitelistDiscovery: true
    enableIstioSidecarOnGateway: true
gatewayProxies:
  gatewayProxy:
    healthyPanicThreshold: 0
    gatewaySettings:
      accessLoggingService:
        accessLog:
        - fileSink:
            path: /dev/stdout
            stringFormat: ""
`))
	Expect(err).NotTo(HaveOccurred())
	err = values.Close()
	Expect(err).NotTo(HaveOccurred())

	return values.Name(), func() {
		_ = os.Remove(values.Name())
	}
}
