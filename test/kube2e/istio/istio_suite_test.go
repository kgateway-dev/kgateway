package istio_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap/zapcore"

	testutils2 "github.com/solo-io/gloo/test/testutils"

	"github.com/solo-io/go-utils/testutils/exec"

	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/k8s-utils/testutils/helper"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

const (
	gatewayProxy = gatewaydefaults.GatewayProxyName
	gatewayPort  = int(80)
	namespace    = defaults.GlooSystem
)

func TestIstio(t *testing.T) {
	helpers.RegisterGlooDebugLogPrintHandlerAndClearLogs()
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	_ = os.Remove(cliutil.GetLogsPath())
	RunSpecs(t, "Istio Suite")
}

var (
	testHelper *helper.SoloTestHelper
	ctx        context.Context
	cancel     context.CancelFunc

	resourceClientSet *kube2e.KubeResourceClientSet
)

var _ = BeforeSuite(func() {
	var err error

	ctx, cancel = context.WithCancel(context.Background())

	testHelper, err = kube2e.GetTestHelper(ctx, namespace)
	Expect(err).NotTo(HaveOccurred())

	skhelpers.RegisterPreFailHandler(helpers.KubeDumpOnFail(GinkgoWriter, testHelper.InstallNamespace))

	err = testutils.Kubectl("create", "ns", testHelper.InstallNamespace)
	Expect(err).NotTo(HaveOccurred())

	err = testutils.Kubectl("label", "namespace", testHelper.InstallNamespace, "istio-injection=enabled")
	Expect(err).NotTo(HaveOccurred())

	if !testutils2.ShouldSkipInstall() {
		installGloo()
	}

	// delete test-runner Service, as the tests create and manage their own
	err = testutils.Kubectl("delete", "service", helper.TestrunnerName, "-n", namespace)
	Expect(err).NotTo(HaveOccurred())
	EventuallyWithOffset(1, func() error {
		return testutils.Kubectl("get", "service", helper.TestrunnerName, "-n", namespace)
	}, "60s", "1s").Should(HaveOccurred())

	// set istio-inject for the testrunner namespace to setup istio-proxies
	err = testutils.Kubectl("annotate", "pods", helper.TestrunnerName, "-n", testHelper.InstallNamespace, "sidecar.istio.io/inject=true")
	Expect(err).NotTo(HaveOccurred())

	expectIstioInjected()

	resourceClientSet, err = kube2e.NewDefaultKubeResourceClientSet(ctx)
	Expect(err).NotTo(HaveOccurred(), "can create kube resource client set")
})

var _ = AfterSuite(func() {
	if testutils2.ShouldTearDown() {
		uninstallGloo()
	}

	cancel()
})

func installGloo() {
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred(), "working dir could not be retrieved while installing gloo")
	helmValuesFile := filepath.Join(cwd, "artifacts", "helm.yaml")

	// Install Gloo
	err = testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", helmValuesFile))
	Expect(err).NotTo(HaveOccurred())

	// patch only the gateway-proxy to be istio inject-able
	err = testutils.Kubectl("patch", "-n", testHelper.InstallNamespace, "deployment", "gateway-proxy", "--patch", "{\"spec\": {\"template\": {\"metadata\": {\"labels\": {\"sidecar.istio.io/inject\": \"true\"}}}}}")
	Expect(err).NotTo(HaveOccurred())

	// Check that everything is OK
	kube2e.GlooctlCheckEventuallyHealthy(1, testHelper, "90s")

	// Ensure gloo reaches valid state and doesn't continually resync
	// we can consider doing the same for leaking go-routines after resyncs
	kube2e.EventuallyReachesConsistentState(testHelper.InstallNamespace, zapcore.InfoLevel)
}

func uninstallGloo() {
	err := testHelper.UninstallGlooAll()
	Expect(err).NotTo(HaveOccurred())

	// glooctl should delete the namespace. we do it again just in case it failed
	// ignore errors
	_ = testutils.Kubectl("delete", "namespace", testHelper.InstallNamespace)

	EventuallyWithOffset(1, func() error {
		return testutils.Kubectl("get", "namespace", testHelper.InstallNamespace)
	}, "60s", "1s").Should(HaveOccurred())
}

// expects gateway-proxy and testrunner to have the istio-proxy sidecar
func expectIstioInjected() {
	// Check for istio-proxy sidecar
	istioContainer, err := exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "pods", "-l", "gloo=gateway-proxy", "-o", `jsonpath='{.items[*].spec.containers[?(@.name == "istio-proxy")].name}'`)
	ExpectWithOffset(1, istioContainer).To(Equal("'istio-proxy'"), "istio-proxy container should be present on gateway-proxy after injection")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	istioContainer, err = exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "pods", helper.TestrunnerName, "-o", `jsonpath='{.spec.containers[?(@.name == "istio-proxy")].name}'`)
	ExpectWithOffset(1, istioContainer).To(Equal("'istio-proxy'"), "istio-proxy container should be present on the testrunner after injection")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}
