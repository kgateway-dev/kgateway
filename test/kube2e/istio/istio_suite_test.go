package istio_test

import (
	"context"
	"fmt"
	"github.com/avast/retry-go"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/k8s-utils/kubeutils"
	"io/ioutil"
	"os"
	osExec "os/exec"
	"testing"
	"time"

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
	istioNamespace = "istio-system"
	// `constants.IstioIngressNamespace` returns "istio-system", but guides state it should be "istio-ingress"
	// https://istio.io/latest/docs/setup/install/helm/
	ingressNamespace    = "istio-ingress"
	AppServiceNamespace = "default"
	AppServiceName      = "httpbin"
	AppNamespace        = "httpbin"
	AppPort             = 80
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
	snapshotWriter    helpers.SnapshotWriter
)

var _ = BeforeSuite(func() {
	var err error
	err = os.Setenv(statusutils.PodNamespaceEnvName, namespace)
	Expect(err).NotTo(HaveOccurred())

	testHelper, err = kube2e.GetTestHelper(ctx, namespace)
	Expect(err).NotTo(HaveOccurred())

	skhelpers.RegisterPreFailHandler(helpers.KubeDumpOnFail(GinkgoWriter, testHelper.InstallNamespace))

	// Install Istio
	err = installIstioHelm()
	Expect(err).NotTo(HaveOccurred())
	// enabling istio-injection on default namespace for the httpbin pod
	_ = testutils.Kubectl("label", "namespace", "default", "istio-injection=enabled")

	// Install HTTPBin application
	filename, httpBinCleanup := getHTTPBinApplication()
	defer httpBinCleanup()
	_ = testutils.Kubectl("apply", "-f", filename)
	// todo wait for it?

	// Install Gloo
	// todo - check that istio pod is running on the gateway-proxy
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

	cfg, err := kubeutils.GetConfig("", "")
	Expect(err).NotTo(HaveOccurred())

	resourceClientSet, err = kube2e.NewKubeResourceClientSet(ctx, cfg)
	Expect(err).NotTo(HaveOccurred())

	snapshotWriter = helpers.NewSnapshotWriter(resourceClientSet, []retry.Option{})
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

		uninstallIstio()
		cancel()
	}
})

func installIstioHelm() error {
	runAndCleanCommand("helm", "repo", "add", "istio", "https://istio-release.storage.googleapis.com/charts")
	runAndCleanCommand("helm", "repo", "update")

	_ = testutils.Kubectl("create", "namespace", istioNamespace)
	runAndCleanCommand("helm", "install", "istio-base", "istio/base", "-n", istioNamespace)
	runAndCleanCommand("helm", "install", "istiod", "istio/istiod", "-n", istioNamespace, "--wait")

	createNamespaceWithIstioInjection(ingressNamespace)
	runAndCleanCommand("helm", "install", "istio-ingress", "istio/gateway", "-n", ingressNamespace)
	// manual check that istio-ingress deployment is ready and had an exit status of 0 (success)
	EventuallyWithOffset(1, func() error {
		return testutils.Kubectl("rollout", "status", "deployment/istio-ingress", "-n", ingressNamespace)
	}, "60s", "1s").ShouldNot(HaveOccurred())

	return nil
}

// Only installs the Service Account and Deployment from https://raw.githubusercontent.com/istio/istio/master/samples/httpbin/httpbin.yaml
func getHTTPBinApplication() (filename string, cleanup func()) {
	createNamespaceWithIstioInjection(AppNamespace)
	values, err := ioutil.TempFile("", "*.yaml")
	Expect(err).NotTo(HaveOccurred())
	_, err = values.Write([]byte(fmt.Sprintf(`
apiVersion: v1
kind: ServiceAccount
metadata:
  name: httpbin
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: httpbin
spec:
  replicas: 1
  selector:
    matchLabels:
      app: httpbin
      version: v1
  template:
    metadata:
      labels:
        app: httpbin
        version: v1
    spec:
      serviceAccountName: httpbin
      containers:
      - image: docker.io/kennethreitz/httpbin
        imagePullPolicy: IfNotPresent
        name: httpbin
        ports:
        - containerPort: %d
`, AppPort)))
	Expect(err).NotTo(HaveOccurred())
	err = values.Close()
	Expect(err).NotTo(HaveOccurred())
	return values.Name(), func() {
		_ = os.Remove(values.Name())
	}
}

func createNamespaceWithIstioInjection(namespace string) {
	_ = testutils.Kubectl("create", "namespace", namespace)
	_ = testutils.Kubectl("label", "namespace", namespace, "istio-injection=enabled")
}

// uninstalls istio from the cluster
// todo - delete deployment & make sure this is correct
func uninstallIstio() {
	// delete ingress
	runAndCleanCommand("helm", "delete", "istio-ingress", "-n", ingressNamespace)
	_ = testutils.Kubectl("delete", "namespace", ingressNamespace)
	EventuallyWithOffset(1, func() error {
		return testutils.Kubectl("get", "namespace", ingressNamespace)
	}, "60s", "1s").Should(HaveOccurred())

	// delete istio-system
	runAndCleanCommand("helm", "delete", "istiod", "-n", istioNamespace)
	runAndCleanCommand("helm", "delete", "istio-base", "-n", istioNamespace)
	_ = testutils.Kubectl("delete", "namespace", istioNamespace)
	EventuallyWithOffset(1, func() error {
		return testutils.Kubectl("get", "namespace", istioNamespace)
	}, "60s", "1s").Should(HaveOccurred())
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
`))
	Expect(err).NotTo(HaveOccurred())
	err = values.Close()
	Expect(err).NotTo(HaveOccurred())

	return values.Name(), func() {
		_ = os.Remove(values.Name())
	}
}

func runAndCleanCommand(name string, arg ...string) []byte {
	cmd := osExec.Command(name, arg...)
	b, err := cmd.Output()
	// for debugging in Cloud Build
	if err != nil {
		if v, ok := err.(*osExec.ExitError); ok {
			fmt.Println("ExitError: ", string(v.Stderr))
		}
	}
	Expect(err).To(BeNil())
	_ = cmd.Process.Kill()
	_ = cmd.Process.Release()
	return b
}
