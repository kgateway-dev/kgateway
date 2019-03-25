package install_test

import (
	"testing"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestInstall(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Install Suite")
}

// NOTE: This needs to be run from the root of the repo as the working directory
var _ = BeforeSuite(func() {
	err := testutils.Make("", "build-test-chart BUILD_ID=unit-testing")
	Expect(err).NotTo(HaveOccurred())
})
