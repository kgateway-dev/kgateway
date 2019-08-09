package test

import (
	"os/exec"
	"sync"
	"testing"

	"github.com/solo-io/go-utils/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/go-utils/manifesttestutils"
)

func TestHelm(t *testing.T) {
	RegisterFailHandler(Fail)
	testutils.RegisterPreFailHandler(testutils.PrintTrimmedStack)
	testutils.RegisterCommonFailHandlers()
	RunSpecs(t, "Helm Suite")
}

const (
	namespace = "gloo-system"
)

var (
	version      string
	testManifest TestManifest
	// use a mutex to prevent these tests from running in parallel
	makefileSerializer sync.Mutex
)

func MustMake(dir string, args ...string) {
	makeCmd := exec.Command("make", args...)
	makeCmd.Dir = dir

	makeCmd.Stdout = GinkgoWriter
	makeCmd.Stderr = GinkgoWriter
	err := makeCmd.Run()

	Expect(err).NotTo(HaveOccurred())
}
