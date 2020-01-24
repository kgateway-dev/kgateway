package gateway_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"github.com/gogo/protobuf/types"
	clienthelpers "github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/check"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/solo-io/gloo/test/helpers"

	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/go-utils/testutils/helper"

	"github.com/solo-io/go-utils/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

func TestGateway(t *testing.T) {
	if testutils.AreTestsDisabled() {
		return
	}

	if os.Getenv("CLUSTER_LOCK_TESTS") == "1" {
		log.Warnf("This test does not require using a cluster lock. Cluster lock is enabled so this test is disabled. " +
			"To enable, unset CLUSTER_LOCK_TESTS in your env.")
		return
	}
	helpers.RegisterGlooDebugLogPrintHandlerAndClearLogs()
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	RunSpecs(t, "Gateway Suite")
}

var testHelper *helper.SoloTestHelper

var _ = BeforeSuite(StartTestHelper)
var _ = AfterSuite(TearDownTestHelper)

func StartTestHelper() {
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())

	randomNumber := time.Now().Unix() % 10000
	testHelper, err = helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
		defaults.RootDir = filepath.Join(cwd, "../../..")
		defaults.HelmChartName = "gloo"
		defaults.InstallNamespace = "gateway-test-" + fmt.Sprintf("%d-%d", randomNumber, GinkgoParallelNode())
		defaults.Verbose = true
		return defaults
	})
	Expect(err).NotTo(HaveOccurred())

	// Register additional fail handlers
	skhelpers.RegisterPreFailHandler(helpers.KubeDumpOnFail(GinkgoWriter, "knative-serving", testHelper.InstallNamespace))
	valueOverrideFile, cleanupFunc := getHelmValuesOverrideFile()
	defer cleanupFunc()

	err = testHelper.InstallGloo(helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", valueOverrideFile))
	Expect(err).NotTo(HaveOccurred())

	// Check that everything is OK
	Eventually(func() error {
		opts := &options.Options{
			Metadata: core.Metadata{
				Namespace: testHelper.InstallNamespace,
			},
		}
		ok, err := check.CheckResources(opts)
		if err != nil {
			return errors.Wrap(err, "unable to run glooctl check")
		}
		if ok {
			return nil
		}
		return errors.New("glooctl check detected a problem with the installation")
	}, "40s", "5s").Should(BeNil())

	// TODO(marco): explicitly enable strict validation, this can be removed once we enable validation by default
	// See https://github.com/solo-io/gloo/issues/1374
	UpdateAlwaysAcceptSetting(false)
}

func TearDownTestHelper() {
	if testHelper != nil {
		err := testHelper.UninstallGloo()
		Expect(err).NotTo(HaveOccurred())
		_ = testutils.Kubectl("delete", "--wait=false", "namespace", testHelper.InstallNamespace)
	}
}

// enable/disable strict validation
func UpdateAlwaysAcceptSetting(alwaysAccept bool) {
	UpdateSettings(func(settings *v1.Settings) {
		Expect(settings.Gateway).NotTo(BeNil())
		Expect(settings.Gateway.Validation).NotTo(BeNil())
		settings.Gateway.Validation.AlwaysAccept = &types.BoolValue{Value: alwaysAccept}
	})
}

func UpdateSettings(f func(settings *v1.Settings)) {
	settingsClient := clienthelpers.MustSettingsClient()
	settings, err := settingsClient.Read(testHelper.InstallNamespace, "default", clients.ReadOpts{})
	Expect(err).NotTo(HaveOccurred())

	f(settings)

	_, err = settingsClient.Write(settings, clients.WriteOpts{OverwriteExisting: true})
	Expect(err).NotTo(HaveOccurred())

	// when validation config changes, the validation server restarts -- give time for it to come up again.
	// without the wait, the validation webhook may temporarily fallback to it's failurePolicy, which is not
	// what we want to test.
	time.Sleep(3 * time.Second)
}

func getHelmValuesOverrideFile() (filename string, cleanup func()) {
	values, err := ioutil.TempFile("", "*.yaml")
	Expect(err).NotTo(HaveOccurred())

	_, err = values.Write([]byte(`
global:
  glooRbac:
    namespaced: true
    nameSuffix: e2e-test-rbac-suffix
settings:
  singleNamespace: true
  create: true
`))
	Expect(err).NotTo(HaveOccurred())

	err = values.Close()
	Expect(err).NotTo(HaveOccurred())

	return values.Name(), func() { _ = os.Remove(values.Name()) }
}
