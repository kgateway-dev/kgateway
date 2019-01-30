package install_test

import (
	"os"
	"testing"

	"github.com/solo-io/solo-kit/pkg/utils/log"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	"github.com/solo-io/gloo/test/helpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestInstall(t *testing.T) {
	if os.Getenv("RUN_KUBE2E_TESTS") != "1" {
		log.Warnf("This test builds and deploys images to dockerhub and kubernetes, " +
			"and is disabled by default. To enable, set RUN_KUBE2E_TESTS=1 in your env.")
		return
	}
	RegisterFailHandler(Fail)
	RunSpecs(t, "Install Suite")
}

var _ = BeforeSuite(func() {
	err := testutils.Make(helpers.GlooDir(), "prepare-helm")
	Expect(err).NotTo(HaveOccurred())
})
