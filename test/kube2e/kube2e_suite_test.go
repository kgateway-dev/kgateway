package kube2e_test

import (
	"os"
	"sync"
	"testing"
	"time"

	skhelpers "github.com/solo-io/solo-kit/test/helpers"

	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/solo-kit/pkg/utils/log"
	stringutils "github.com/solo-io/solo-kit/test/helpers"
	"github.com/solo-io/solo-kit/test/setup"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	// TODO(ilackarms): tie testrunner to solo CI test containers and then handle image tagging
	defaultTestRunnerImage = "soloio/testrunner:latest"
)

func TestKube2e(t *testing.T) {
	if os.Getenv("RUN_KUBE2E_TESTS") != "1" {
		log.Warnf("This test builds and deploys images to dockerhub and kubernetes, " +
			"and is disabled by default. To enable, set RUN_KUBE2E_TESTS=1 in your env.")
		return
	}

	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	// RegisterFailHandler(Fail)
	RunSpecs(t, "Kube2e Suite")
}

var namespace, version string
var testRunnerPort int32
var _ = BeforeSuite(func() {
	// build and push images for test
	version = helpers.TestVersion()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer GinkgoRecover()
		defer wg.Done()
		err := helpers.BuildPushContainers(version, true, true)
		Expect(err).NotTo(HaveOccurred())
	}()

	// todo (ilackarms): move randstring to stringutils package
	namespace = "a" + stringutils.RandString(8)
	testRunnerPort = 1234
	wg.Add(1)
	go func() {
		defer GinkgoRecover()
		defer wg.Done()
		err := setup.SetupKubeForTest(namespace)
		Expect(err).NotTo(HaveOccurred())
		err = helpers.DeployTestRunner(namespace, defaultTestRunnerImage, testRunnerPort)
		Expect(err).NotTo(HaveOccurred())
	}()
	wg.Wait()

	err := helpers.DeployGlooWithHelm(namespace, version, false, true)
	Expect(err).NotTo(HaveOccurred())
	err = helpers.WaitGlooPods(time.Minute, time.Second)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	setup.TeardownKube(namespace)
})
