package eds_test

import (
	"os"
	"testing"

	"github.com/onsi/ginkgo/reporters"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/go-utils/log"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

var (
	namespace = "eds-test-ns"

	_ = BeforeSuite(func() {
		err := os.Setenv("POD_NAMESPACE", namespace)
		Expect(err).NotTo(HaveOccurred())
	})

	_ = AfterSuite(func() {
		err := os.Unsetenv("POD_NAMESPACE")
		Expect(err).NotTo(HaveOccurred())
	})
)

func TestDiscovery(t *testing.T) {
	if os.Getenv("KUBE2E_TESTS") != "eds" {
		log.Warnf("This test is disabled. " +
			"To enable, set KUBE2E_TESTS to 'eds' in your env.")
		return
	}
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	_ = os.Remove(cliutil.GetLogsPath())
	skhelpers.RegisterPreFailHandler(helpers.KubeDumpOnFail(GinkgoWriter, defaults.GlooSystem))
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "Endpoint discovery (EDS) Suite", []Reporter{junitReporter})
}
