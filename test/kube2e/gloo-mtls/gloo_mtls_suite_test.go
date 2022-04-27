package gloo_mtls_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/gloo/test/kube2e"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/go-utils/testutils/exec"
	"github.com/solo-io/k8s-utils/testutils/helper"
	"github.com/solo-io/solo-kit/pkg/utils/statusutils"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

var (
	testHelper       *helper.SoloTestHelper
	installNamespace = defaults.GlooSystem
	ctx              context.Context
	cancel           context.CancelFunc
)

func TestGlooMtls(t *testing.T) {
	if os.Getenv("KUBE2E_TESTS") != "gloomtls" {
		log.Warnf("This test is disabled. " +
			"To enable, set KUBE2E_TESTS to 'gloomtls' in your env.")
		return
	}
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	_ = os.Remove(cliutil.GetLogsPath())
	skhelpers.RegisterPreFailHandler(helpers.KubeDumpOnFail(GinkgoWriter, installNamespace))
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "Gloo mTLS Suite", []Reporter{junitReporter})
}

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.Background())
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())

	err = os.Setenv(statusutils.PodNamespaceEnvName, installNamespace)
	Expect(err).NotTo(HaveOccurred())

	testHelper, err = helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
		defaults.RootDir = filepath.Join(cwd, "../../..")
		defaults.HelmChartName = "gloo"
		defaults.InstallNamespace = installNamespace
		return defaults
	})
	Expect(err).NotTo(HaveOccurred())

	skhelpers.RegisterPreFailHandler(helpers.KubeDumpOnFail(GinkgoWriter, testHelper.InstallNamespace))

	// Install Gloo
	values, cleanup := getHelmOverrides()
	defer cleanup()

	err = testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", values, "-v"))
	Expect(err).NotTo(HaveOccurred())
	kube2e.GlooctlCheckEventuallyHealthy(1, testHelper, "2m")

	// Print out the versions of CLI and server components
	glooctlVersionCommand := []string{
		filepath.Join(testHelper.BuildAssetDir, testHelper.GlooctlExecName),
		"version", "-n", testHelper.InstallNamespace}
	output, err := exec.RunCommandOutput(testHelper.RootDir, true, glooctlVersionCommand...)
	Expect(err).NotTo(HaveOccurred())
	fmt.Println(output)
})

var _ = AfterSuite(func() {
	err := os.Unsetenv(statusutils.PodNamespaceEnvName)
	Expect(err).NotTo(HaveOccurred())
	if os.Getenv("TEAR_DOWN") == "true" {
		err := testHelper.UninstallGloo()
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

func getHelmOverrides() (filename string, cleanup func()) {
	values, err := ioutil.TempFile("", "*.yaml")
	Expect(err).NotTo(HaveOccurred())
	// Set global.glooMtls.enabled = true, and make sure to pull the quay.io/solo-io
	_, err = values.Write([]byte(`
gloo:
  rbac:
    namespaced: true
    nameSuffix: e2e-test-rbac-suffix
gateway:
  persistProxySpec: true
settings:
  singleNamespace: true
  create: true
prometheus:
  podSecurityPolicy:
    enabled: true
grafana:
  testFramework:
    enabled: false
global:
  glooMtls:
    enabled: true
    sds:
      image:
        registry: quay.io/solo-io
`)) // need to override registry because we use gcr and quay confusingly https://github.com/solo-io/solo-projects/issues/1733
	Expect(err).NotTo(HaveOccurred())
	err = values.Close()
	Expect(err).NotTo(HaveOccurred())

	return values.Name(), func() {
		_ = os.Remove(values.Name())
	}
}
