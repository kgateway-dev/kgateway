package helm_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"github.com/solo-io/go-utils/log"

	"github.com/solo-io/gloo/test/kube2e"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/gloo/test/helpers"

	"github.com/solo-io/k8s-utils/testutils/helper"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	"github.com/solo-io/solo-kit/pkg/utils/statusutils"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

func TestHelm(t *testing.T) {
	if os.Getenv("KUBE2E_TESTS") != "helm" {
		log.Warnf("This test is disabled. To enable, set KUBE2E_TESTS to 'helm' in your env.")
		return
	}
	helpers.RegisterGlooDebugLogPrintHandlerAndClearLogs()
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "Helm Suite", []Reporter{junitReporter})
}

var testHelper *helper.SoloTestHelper
var ctx, cancel = context.WithCancel(context.Background())
var namespace = defaults.GlooSystem
var _ = BeforeSuite(StartTestHelper)
var _ = AfterSuite(TearDownTestHelper)

// now that we run CI on a kube 1.22 cluster, we must ensure that we install versions of gloo with v1 CRDs
// Per https://github.com/solo-io/gloo/issues/4543: CRDs were migrated from v1beta1 -> v1 in Gloo 1.9.0
const earliestVersionWithV1CRDs = "1.9.0"

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

	var valueOverrideFile string
	var cleanupFunc func()
	if os.Getenv("STRICT_VALIDATION") == "true" {
		valueOverrideFile, cleanupFunc = getStrictValidationHelmValuesOverrideFile()
	} else {
		valueOverrideFile, cleanupFunc = kube2e.GetHelmValuesOverrideFile()
	}
	defer cleanupFunc()

	// install gloo with helm
	if os.Getenv("STRICT_VALIDATION") == "true" {
		// in the strict validation tests, we want to install the gloo version from code (not a release version)
		// to make sure it installs successfully
		chartUri := filepath.Join(testHelper.RootDir, testHelper.TestAssetDir, testHelper.HelmChartName+"-"+testHelper.ChartVersion()+".tgz")
		runAndCleanCommand("helm", "install", testHelper.HelmChartName, chartUri,
			"--namespace", testHelper.InstallNamespace,
			"--create-namespace",
			"--values", valueOverrideFile)
	} else {
		// some tests are testing upgrades, so they initially need a release version to be installed before upgrading
		// to the version from code
		runAndCleanCommand("kubectl", "create", "namespace", testHelper.InstallNamespace)
		runAndCleanCommand("helm", "repo", "add", testHelper.HelmChartName, "https://storage.googleapis.com/solo-public-helm")
		runAndCleanCommand("helm", "repo", "update")
		runAndCleanCommand("helm", "install", testHelper.HelmChartName, "gloo/gloo",
			"--namespace", testHelper.InstallNamespace,
			"--values", valueOverrideFile,
			"--version", fmt.Sprintf("v%s", earliestVersionWithV1CRDs))
	}

	// Check that everything is OK
	kube2e.GlooctlCheckEventuallyHealthy(1, testHelper, "90s")
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

func runAndCleanCommand(name string, arg ...string) []byte {
	cmd := exec.Command(name, arg...)
	b, err := cmd.Output()
	// for debugging in Cloud Build
	if err != nil {
		if v, ok := err.(*exec.ExitError); ok {
			fmt.Println("ExitError: ", string(v.Stderr))
		}
	}
	Expect(err).To(BeNil())
	cmd.Process.Kill()
	cmd.Process.Release()
	return b
}

func getStrictValidationHelmValuesOverrideFile() (filename string, cleanup func()) {
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
  validation:
    allowWarnings: false
    alwaysAcceptResources: false
    failurePolicy: Fail
gatewayProxies:
  gatewayProxy:
    healthyPanicThreshold: 0
`))
	Expect(err).NotTo(HaveOccurred())

	err = values.Close()
	Expect(err).NotTo(HaveOccurred())

	return values.Name(), func() { _ = os.Remove(values.Name()) }
}
