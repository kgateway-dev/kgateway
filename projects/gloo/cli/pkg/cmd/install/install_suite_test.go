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
	gotestutils.RegisterPreFailHandler(gotestutils.PrintTrimmedStack)
	gotestutils.RegisterCommonFailHandlers()
	RunSpecs(t, "Install Suite")
}

var RootDir string
var file string

// NOTE: This needs to be run from the root of the repo as the working directory
var _ = BeforeSuite(func() {
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())
	RootDir = filepath.Join(cwd, "../../../../../..")

	err = testutils.Make(RootDir, "build-test-chart BUILD_ID=unit-testing")
	Expect(err).NotTo(HaveOccurred())

	file = filepath.Join(RootDir, "_test/gloo-test-unit-testing.tgz")
})

var _ = AfterSuite(func() {
	err := os.Remove(file)
	Expect(err).NotTo(HaveOccurred())
})
