package gateway_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/gloo/pkg/cliutil/install"

	"github.com/gogo/protobuf/types"
	clienthelpers "github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	"github.com/pkg/errors"
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
		log.Warnf("This test does not require using a cluster lock. cluster lock is enabled so this test is disabled. " +
			"To enable, unset CLUSTER_LOCK_TESTS in your env.")
		return
	}
	helpers.RegisterGlooDebugLogPrintHandlerAndClearLogs()
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	RunSpecs(t, "Gateway Suite")
}

var (
	testHelper   *helper.SoloTestHelper
	testInstance int
	values       *os.File
	randomNumber = time.Now().Unix() % 10000
)

func StartTestHelper() {

	testInstance += 1
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())

	testHelper, err = helper.NewSoloTestHelper(func(defaults helper.TestConfig) helper.TestConfig {
		defaults.RootDir = filepath.Join(cwd, "../../..")
		defaults.HelmChartName = "gloo"
		// TODO: include build id?
		defaults.InstallNamespace = "gateway-test-" + fmt.Sprintf("%d-%d-%d", randomNumber, GinkgoParallelNode(), testInstance)
		return defaults
	})
	Expect(err).NotTo(HaveOccurred())

	RegisterFailHandler(func(message string, callerSkip ...int) {
		glooLogs, _ := install.KubectlOut(nil, "logs", "-n", testHelper.InstallNamespace, "-l", "gloo=gloo")
		gwLogs, _ := install.KubectlOut(nil, "logs", "-n", testHelper.InstallNamespace, "-l", "gloo=gateway")

		fmt.Fprintf(GinkgoWriter, "\n\n\n\nGLOO LOGS\n\n%s\n\n\n\n", glooLogs)
		fmt.Fprintf(GinkgoWriter, "\n\n\n\nGATEWAY LOGS\n\n%s\n\n\n\n", gwLogs)
	})

	skhelpers.RegisterPreFailHandler(helpers.KubeDumpOnFail(GinkgoWriter, "knative-serving", testHelper.InstallNamespace))
	testHelper.Verbose = true

	values, err = ioutil.TempFile("", "*.yaml")
	Expect(err).NotTo(HaveOccurred())
	values.Write([]byte("global:\n  glooRbac:\n    namespaced: true\nsettings:\n  singleNamespace: true\n  create: true\n"))
	values.Close()

	err = testHelper.InstallGloo(helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", values.Name()))
	Expect(err).NotTo(HaveOccurred())
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
	}, "40s", "4s").Should(BeNil())

	// enable strict validation
	// this can be removed once we enable validation by default
	// set projects/gateway/pkg/syncer.AcceptAllResourcesByDefault is set to false
	settingsClient := clienthelpers.MustSettingsClient()
	settings, err := settingsClient.Read(testHelper.InstallNamespace, "default", clients.ReadOpts{})
	Expect(err).NotTo(HaveOccurred())

	Expect(settings.Gateway).NotTo(BeNil())
	Expect(settings.Gateway.Validation).NotTo(BeNil())
	settings.Gateway.Validation.AlwaysAccept = &types.BoolValue{Value: false}

	_, err = settingsClient.Write(settings, clients.WriteOpts{OverwriteExisting: true})
	Expect(err).NotTo(HaveOccurred())
}

func TearDownTestHelper() {
	if values != nil {
		os.Remove(values.Name())
	}
	if testHelper != nil {
		err := testHelper.UninstallGloo()
		Expect(err).NotTo(HaveOccurred())
		_ = testutils.Kubectl("delete", "--wait=false", "namespace", testHelper.InstallNamespace)
	}
}
