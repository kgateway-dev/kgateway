package gloomtls_test

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/avast/retry-go"
	"github.com/solo-io/k8s-utils/kubeutils"

	"github.com/solo-io/gloo/test/kube2e"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

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
	namespace   = defaults.GlooSystem
	gatewayPort = int(80)
)

var (
	testHelper        *helper.SoloTestHelper
	resourceClientset *kube2e.KubeResourceClientSet
	snapshotWriter    helpers.SnapshotWriter

	ctx, cancel = context.WithCancel(context.Background())
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
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "Gloo mTLS Suite", []Reporter{junitReporter})
}

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.Background())
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())

	if useVersion := kube2e.GetTestReleasedVersion(ctx, "gloo"); useVersion != "" {
		testHelper, err = helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
			defaults.RootDir = filepath.Join(cwd, "../../..")
			defaults.ReleasedVersion = useVersion
			defaults.InstallNamespace = namespace
			return defaults
		})
		Expect(err).NotTo(HaveOccurred())
	} else {
		testHelper, err = helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
			defaults.RootDir = filepath.Join(cwd, "../../..")
			defaults.HelmChartName = "gloo"
			defaults.InstallNamespace = namespace
			return defaults
		})
		Expect(err).NotTo(HaveOccurred())
	}
	skhelpers.RegisterPreFailHandler(helpers.KubeDumpOnFail(GinkgoWriter, testHelper.InstallNamespace))

	// Install Gloo
	values, cleanup := getHelmOverrides()
	defer cleanup()

	err = testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", values, "-v"))
	Expect(err).NotTo(HaveOccurred())
	kube2e.GlooctlCheckEventuallyHealthy(1, testHelper, "2m")

	// Ensure gloo reaches valid state and doesn't continually resync
	// we can consider doing the same for leaking go-routines after resyncs
	kube2e.EventuallyReachesConsistentState(testHelper.InstallNamespace)

	cfg, err := kubeutils.GetConfig("", "")
	Expect(err).NotTo(HaveOccurred())

	resourceClientset, err = kube2e.NewKubeResourceClientSet(ctx, cfg)
	Expect(err).NotTo(HaveOccurred())

	snapshotWriter = helpers.NewSnapshotWriter(resourceClientset, []retry.Option{})
})

var _ = AfterSuite(func() {
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
	imageRegistry := "quay.io/solo-io"
	if runtime.GOARCH == "arm64" && os.Getenv("RUNNING_REGRESSION_TESTS") == "true" {
		imageRegistry = "\"localhost:5000\""
	}
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
        registry: ` + imageRegistry + `
`)) // need to override registry because we use gcr and quay confusingly https://github.com/solo-io/solo-projects/issues/1733
	Expect(err).NotTo(HaveOccurred())
	err = values.Close()
	Expect(err).NotTo(HaveOccurred())

	return values.Name(), func() {
		_ = os.Remove(values.Name())
	}
}
