package syncer

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	"github.com/solo-io/solo-kit/test/helpers"
)

var (
	namespace = "syncer-test-ns"

	_ = BeforeSuite(func() {
		err := os.Setenv("POD_NAMESPACE", namespace)
		Expect(err).NotTo(HaveOccurred())
	})

	_ = AfterSuite(func() {
		err := os.Unsetenv("POD_NAMESPACE")
		Expect(err).NotTo(HaveOccurred())
	})
)

func TestSyncer(t *testing.T) {
	RegisterFailHandler(Fail)
	helpers.SetupLog()
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "Syncer Suite", []Reporter{junitReporter})
}
