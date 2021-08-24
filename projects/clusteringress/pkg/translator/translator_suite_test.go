package translator_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var (
	namespace = "translator-test-ns"

	_ = BeforeSuite(func() {
		err := os.Setenv("POD_NAMESPACE", namespace)
		Expect(err).NotTo(HaveOccurred())
	})

	_ = AfterSuite(func() {
		err := os.Unsetenv("POD_NAMESPACE")
		Expect(err).NotTo(HaveOccurred())
	})
)

func TestTranslator(t *testing.T) {
	RegisterFailHandler(Fail)
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "Translator Suite", []Reporter{junitReporter})
}
