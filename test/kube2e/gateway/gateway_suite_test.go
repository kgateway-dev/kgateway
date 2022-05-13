package gateway_test

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/solo-io/go-utils/testutils/exec"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/k8s-utils/testutils/helper"
	"github.com/solo-io/solo-kit/pkg/utils/statusutils"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGateway(t *testing.T) {
	if os.Getenv("KUBE2E_TESTS") != "gateway" {
		log.Warnf("This test is disabled. " +
			"To enable, set KUBE2E_TESTS to 'gateway' in your env.")
		return
	}
	helpers.RegisterGlooDebugLogPrintHandlerAndClearLogs()
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "Gateway Suite", []Reporter{junitReporter})
}

var testHelper *helper.SoloTestHelper
var ctx, cancel = context.WithCancel(context.Background())
var namespace = defaults.GlooSystem

var _ = BeforeSuite(StartTestHelper)
var _ = AfterSuite(TearDownTestHelper)

func StartTestHelper() {
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())

	err = os.Setenv(statusutils.PodNamespaceEnvName, namespace)
	Expect(err).NotTo(HaveOccurred())

	testHelper, err = helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
		defaults.RootDir = filepath.Join(cwd, "../../..")
		defaults.HelmChartName = "gloo"
		defaults.InstallNamespace = namespace
		defaults.Verbose = true
		return defaults
	})
	Expect(err).NotTo(HaveOccurred())

	// install xds-relay if needed
	if os.Getenv("USE_XDS_RELAY") == "true" {
		err = installXdsRelay()
		Expect(err).NotTo(HaveOccurred())
	}

	var valueOverrideFile string
	var cleanupFunc func()

	if os.Getenv("USE_XDS_RELAY") == "true" {
		valueOverrideFile, cleanupFunc = getXdsRelayHelmValuesOverrideFile()
	} else {
		valueOverrideFile, cleanupFunc = kube2e.GetHelmValuesOverrideFile()
	}
	defer cleanupFunc()

	// Allow skipping of install step for running multiple times
	if os.Getenv("SKIP_INSTALL") != "1" {
		err = testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", valueOverrideFile))
		Expect(err).NotTo(HaveOccurred())
	}

	// Check that everything is OK
	kube2e.GlooctlCheckEventuallyHealthy(1, testHelper, "90s")
	// TODO(marco): explicitly enable strict validation, this can be removed once we enable validation by default
	// See https://github.com/solo-io/gloo/issues/1374
	kube2e.UpdateAlwaysAcceptSetting(ctx, false, testHelper.InstallNamespace)

	// Ensure gloo reaches valid state and doesn't continually resync
	// we can consider doing the same for leaking go-routines after resyncs
	kube2e.EventuallyReachesConsistentState(testHelper.InstallNamespace)
}

func installXdsRelay() error {
	helmRepoAddArgs := strings.Split("helm repo add xds-relay https://storage.googleapis.com/xds-relay-helm", " ")
	err := exec.RunCommandInput("", testHelper.RootDir, true, helmRepoAddArgs...)
	if err != nil {
		return err
	}

	helmInstallArgs := strings.Split("helm install xdsrelay xds-relay/xds-relay --version 0.0.3 --set bootstrap.logging.level=DEBUG --set deployment.replicas=1", " ")

	err = exec.RunCommandInput("", testHelper.RootDir, true, helmInstallArgs...)
	if err != nil {
		return err
	}
	return nil
}

func getXdsRelayHelmValuesOverrideFile() (filename string, cleanup func()) {
	values, err := ioutil.TempFile("", "values-*.yaml")
	Expect(err).NotTo(HaveOccurred())

	// disabling usage statistics is not important to the functionality of the tests,
	// but we don't want to report usage in CI since we only care about how our users are actually using Gloo.
	// install to a single namespace so we can run multiple invocations of the regression tests against the
	// same cluster in CI.
	_, err = values.Write([]byte(`
global:
  image:
    pullPolicy: IfNotPresent
  glooRbac:
    namespaced: true
    nameSuffix: e2e-test-rbac-suffix
settings:
  singleNamespace: true
  create: true
  replaceInvalidRoutes: true
gateway:
  persistProxySpec: true
gatewayProxies:
  gatewayProxy:
    healthyPanicThreshold: 0
    xdsServiceAddress: xds-relay.default.svc.cluster.local
    xdsServicePort: 9991
`))
	Expect(err).NotTo(HaveOccurred())

	err = values.Close()
	Expect(err).NotTo(HaveOccurred())

	return values.Name(), func() { _ = os.Remove(values.Name()) }
}

func TearDownTestHelper() {
	err := os.Unsetenv(statusutils.PodNamespaceEnvName)
	Expect(err).NotTo(HaveOccurred())

	if os.Getenv("TEAR_DOWN") == "true" {
		Expect(testHelper).ToNot(BeNil())
		err := testHelper.UninstallGloo()
		Expect(err).NotTo(HaveOccurred())
		_, err = kube2e.MustKubeClient().CoreV1().Namespaces().Get(ctx, testHelper.InstallNamespace, metav1.GetOptions{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
		cancel()
	}
}
