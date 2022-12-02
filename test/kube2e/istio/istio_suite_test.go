package istio_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/avast/retry-go"
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
	AppServiceNamespace = "default"
	AppServiceName      = "httpbin"
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

	// enabling istio-injection on default namespace for the httpbin pod
	_ = testutils.Kubectl("label", "namespace", AppServiceNamespace, "istio-injection=enabled")

	// Install HTTPBin application
	filename, httpBinCleanup := getHTTPBinApplication()
	defer httpBinCleanup()
	_ = testutils.Kubectl("apply", "-f", filename)
	EventuallyWithOffset(1, func() error {
		return testutils.Kubectl("get", "deployment/httpbin", "-n", "default")
	}, "60s", "1s").ShouldNot(HaveOccurred())

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

		uninstallHTTPBin()
		cancel()
	}
})

// Only installs the Service Account and Deployment from https://raw.githubusercontent.com/istio/istio/master/samples/httpbin/httpbin.yaml
func getHTTPBinApplication() (filename string, cleanup func()) {
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

func uninstallHTTPBin() {
	_ = testutils.Kubectl("delete", "deployment/httpbin")
	EventuallyWithOffset(1, func() error {
		return testutils.Kubectl("get", "deployment/httpbin")
	}, "60s", "1s").Should(HaveOccurred())

	_ = testutils.Kubectl("delete", "serviceaccount", "httpbin", "-n", "default")
	EventuallyWithOffset(1, func() error {
		return testutils.Kubectl("get", "serviceaccount", "httpbin", "-n", "default")
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
