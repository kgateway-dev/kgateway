package install_test

import (
	"testing"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	"github.com/solo-io/gloo/test/helpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestInstall(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Install Suite")
}

var _ = BeforeSuite(func() {
	err := testutils.Make(helpers.GlooDir(), "prepare-helm")
	Expect(err).NotTo(HaveOccurred())
})
