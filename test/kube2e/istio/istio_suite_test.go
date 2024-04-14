package istio_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	kubernetesplugin "github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes"
	testutils2 "github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/go-utils/testutils/exec"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"google.golang.org/protobuf/types/known/wrapperspb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/go-utils/testutils"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

const (
	gatewayProxy     = gatewaydefaults.GatewayProxyName
	glooGatewayProxy = "gloo-proxy-http"
	gatewayPort      = int(80)
	installNamespace = defaults.GlooSystem
	httpbinNamespace = "httpbin-ns"
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

	cwd string

	resourceClientSet *kube2e.KubeResourceClientSet

	useGlooGateway bool
	useAutoMtls    bool
)

var _ = BeforeSuite(func() {
	var err error

	ctx, cancel = context.WithCancel(context.Background())

	cwd, err = os.Getwd()
	Expect(err).NotTo(HaveOccurred(), "working dir could not be retrieved")

	// Check env for setup options
	useGlooGateway = testutils2.IsEnvTruthy(testutils2.GlooGatewaySetup)
	useAutoMtls = testutils2.IsEnvTruthy(testutils2.IstioAutoMtls)

	testHelper, err = kube2e.GetTestHelper(ctx, installNamespace)
	Expect(err).NotTo(HaveOccurred())

	if useGlooGateway {
		// gloo gateway is named differently from the classic edge proxy based on the Gateway resource name
		skhelpers.RegisterPreFailHandler(helpers.StandardGlooDumpOnFail(GinkgoWriter,
			metav1.ObjectMeta{
				Name:      glooGatewayProxy,
				Namespace: testHelper.InstallNamespace,
			}))
	} else {
		skhelpers.RegisterPreFailHandler(helpers.StandardGlooDumpOnFail(GinkgoWriter, metav1.ObjectMeta{Namespace: testHelper.InstallNamespace}))
	}

	if !testutils2.ShouldSkipInstall() {
		// testserver is install in gloo-system
		err = testutils.Kubectl("create", "ns", testHelper.InstallNamespace)
		Expect(err).NotTo(HaveOccurred())

		if useGlooGateway {
			// Gloo Gateway setup always uses auto mtls
			installGlooGateway()
		} else {
			installGloo(useAutoMtls)
		}
	}

	resourceClientSet, err = kube2e.NewDefaultKubeResourceClientSet(ctx)
	Expect(err).NotTo(HaveOccurred(), "can create kube resource client set")

	// Install httpbin app in its own namespace, labeled for Istio injection
	installHttpbin()

	// delete testserver Service, as the tests create and manage their own
	testserverExists := false
	err = testutils.Kubectl("get", "service", helper.TestServerName, "-n", installNamespace)
	if err == nil {
		// namespace exists
		testserverExists = true
	}
	if testserverExists {
		// ignore errors if the service doesn't exist
		// (e.g. if the test is being re-run after a previous failure)
		err = testutils.Kubectl("delete", "service", helper.TestServerName, "-n", installNamespace)
		Expect(err).NotTo(HaveOccurred())
	}
	EventuallyWithOffset(1, func() error {
		return testutils.Kubectl("get", "service", helper.TestServerName, "-n", installNamespace)
	}, "60s", "1s").Should(HaveOccurred(), "testserver service should be deleted")

	if !useGlooGateway {
		// TODO(npolshak): Different check necessary for Gloo Gateway created proxy
		expectIstioInjected()
	}
})

// Installs httpbin app in its own namespace, labeled for Istio injection
func installHttpbin() {
	// Check if the namespace exists
	namespaceExists := false
	err := testutils.Kubectl("get", "ns", httpbinNamespace)
	if err == nil {
		// namespace exists
		namespaceExists = true
	}

	if !namespaceExists {
		// If the namespace doesn't exist, create it
		err = testutils.Kubectl("create", "ns", httpbinNamespace)
		if err != nil {
			// Handle error
			panic(err)
		}
	}

	err = testutils.Kubectl("label", "namespace", httpbinNamespace, "istio-injection=enabled")
	Expect(err).NotTo(HaveOccurred())

	err = testutils.Kubectl("apply", "-n", httpbinNamespace, "-f", filepath.Join(cwd, "artifacts", "httpbin.yaml"))
	Expect(err).NotTo(HaveOccurred())

	// Check discovery component has created upstream for httpbin
	if !useGlooGateway {
		httpbinUpstreamName := kubernetesplugin.UpstreamName(httpbinNamespace, httpbinName, httpbinPort)
		helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
			return resourceClientSet.UpstreamClient().Read(installNamespace, httpbinUpstreamName, clients.ReadOpts{})
		})
	}
}

var _ = AfterSuite(func() {
	if testutils2.ShouldTearDown() {
		uninstallGloo()

		err := testutils.Kubectl("delete", "namespace", httpbinNamespace)
		Expect(err).NotTo(HaveOccurred())
	}

	cancel()
})

func installGloo(autoMtls bool) {
	var helmValuesFile string
	if autoMtls {
		helmValuesFile = filepath.Join(cwd, "artifacts", "automtls-helm.yaml")
	} else {
		helmValuesFile = filepath.Join(cwd, "artifacts", "helm.yaml")
	}

	// Install Gloo
	// this helper function also applies the testserver pod and service
	err := testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", helmValuesFile))
	Expect(err).NotTo(HaveOccurred())

	// Check that everything is OK
	kube2e.GlooctlCheckEventuallyHealthy(1, testHelper, "90s")

	// Ensure gloo reaches valid state and doesn't continually resync
	// we can consider doing the same for leaking go-routines after resyncs
	kube2e.EventuallyReachesConsistentState(testHelper.InstallNamespace)

	// Ensure discovery reaches a valid state
	// Note: discovery is only used in the "classic", non-k8s-gateway api setup
	err = testutils.WaitPodsRunning(ctx, time.Second, testHelper.InstallNamespace, "gloo=discovery")
	Expect(err).NotTo(HaveOccurred())

	if autoMtls {
		kube2e.UpdateSettings(ctx, func(settings *gloov1.Settings) {
			Expect(settings.GetGateway().GetCompressedProxySpec()).NotTo(BeNil())
			settings.Gloo.IstioOptions.EnableAutoMtls = &wrapperspb.BoolValue{Value: true}
		}, testHelper.InstallNamespace)
	}
}

func installGlooGateway() {
	helmValuesFile := filepath.Join(cwd, "artifacts", "gloo-gateway-helm.yaml")

	// Install Gloo Gateway with Istio SDS enabled and automtls
	// this helper function also applies the testserver pod and service
	err := testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", helmValuesFile))
	Expect(err).NotTo(HaveOccurred())

	// TODO(npolshak): Add glooctl health check support for Gloo Gateway

	// Ensure gloo reaches valid state and doesn't continually resync
	// we can consider doing the same for leaking go-routines after resyncs
	kube2e.EventuallyReachesConsistentState(testHelper.InstallNamespace)

	// Create Gateway resources
	err = testutils.Kubectl("apply", "-f", filepath.Join(cwd, "artifacts", "gateway.yaml"))
	Expect(err).NotTo(HaveOccurred())

	kube2e.UpdateSettings(ctx, func(settings *gloov1.Settings) {
		Expect(settings.GetGateway().GetCompressedProxySpec()).NotTo(BeNil())
		settings.Gloo.IstioOptions.EnableAutoMtls = &wrapperspb.BoolValue{Value: true}
	}, testHelper.InstallNamespace)
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

// expects gateway-proxy and httpbin to have the istio-proxy sidecar, testserver should not
func expectIstioInjected() {
	// Check for istio-proxy sidecar
	istioContainer, err := exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "pods", "-l", "gloo=gateway-proxy", "-o", `jsonpath='{.items[*].spec.containers[?(@.name == "istio-proxy")].name}'`)
	ExpectWithOffset(1, istioContainer).To(Equal("'istio-proxy'"), "istio-proxy container should be present on gateway-proxy due to IstioSDS being enabled")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	istioContainer, err = exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "pods", helper.TestServerName, "-o", `jsonpath='{.spec.containers[?(@.name == "istio-proxy")].name}'`)
	ExpectWithOffset(1, istioContainer).To(Equal("''"), "istio-proxy container should not be present on the testserver")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	istioContainer, err = exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", httpbinNamespace, "pods", "-l", "app=httpbin", "-o", `jsonpath='{.items[*].spec.containers[?(@.name == "istio-proxy")].name}'`)
	ExpectWithOffset(1, istioContainer).To(Equal("'istio-proxy'"), "istio-proxy container should be present on the httpbin pod after injection")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	// Check for sds container
	sdsContainer, err := exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "pods", "-l", "gloo=gateway-proxy", "-o", `jsonpath='{.items[*].spec.containers[?(@.name == "sds")].name}'`)
	ExpectWithOffset(1, sdsContainer).To(Equal("'sds'"), "sds container should be present on gateway-proxy due to IstioSDS being enabled")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}
