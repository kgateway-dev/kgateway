package syncer_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
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
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "Gloo Translator Syncer Suite", []Reporter{junitReporter})
}
