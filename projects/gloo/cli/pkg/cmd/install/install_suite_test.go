package install_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/testutils"
	gotestutils "github.com/solo-io/go-utils/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestInstall(t *testing.T) {
	RegisterFailHandler(Fail)
	gotestutils.RegisterCommonFailHandlers()
	RunSpecs(t, "Install Suite")
}

var RootDir string
var dir string
var file string

// NOTE: This needs to be run from the root of the repo as the working directory
var _ = BeforeSuite(func() {
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())
	RootDir = filepath.Join(cwd, "../../../../../..")

	// the regression test depend on having only one chart in _test.
	// so run these in a different location.
	dir = filepath.Join(RootDir, "_unit_test/")
	os.Mkdir(dir, 0755)

	err = testutils.Make(RootDir, "build-test-chart TEST_ASSET_DIR=\""+dir+"\" BUILD_ID=unit-testing")
	Expect(err).NotTo(HaveOccurred())

	file = filepath.Join(dir, "gloo-test-unit-testing.tgz")
})

var _ = AfterSuite(func() {
	err := os.Remove(file)
	Expect(err).NotTo(HaveOccurred())
	err = os.RemoveAll(dir)
	Expect(err).NotTo(HaveOccurred())
})
