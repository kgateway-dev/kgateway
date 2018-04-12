package local_e2e

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/solo-io/gloo/test/helpers/local"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/log"
)

var (
	envoyFactory             *localhelpers.EnvoyFactory
	glooFactory              *localhelpers.GlooFactory
	functionDiscoveryFactory *localhelpers.FunctionDiscoveryFactory
	natsStreamingFactory     *localhelpers.NatsStreamingFactory
)

func TestLocalE2e(t *testing.T) {
	if runtime.GOOS != "linux" {
		fmt.Println("local E2E Suite only runs on Linux")
		return
	}

	RegisterFailHandler(Fail)
	log.DefaultOut = GinkgoWriter
	RunSpecs(t, "LocalE2e Suite")
}

var _ = BeforeSuite(func() {
	var err error
	envoyFactory, err = localhelpers.NewEnvoyFactory()
	Expect(err).NotTo(HaveOccurred())
	glooFactory, err = localhelpers.NewGlooFactory()
	Expect(err).NotTo(HaveOccurred())
	functionDiscoveryFactory, _ = localhelpers.NewFunctionDiscoveryFactory()
	natsStreamingFactory, _ = localhelpers.NewNatsStreamingFactory()

})

var _ = AfterSuite(func() {
	envoyFactory.Clean()
	glooFactory.Clean()
	if functionDiscoveryFactory != nil {
		functionDiscoveryFactory.Clean()
	}
	if natsStreamingFactory != nil {
		natsStreamingFactory.Clean()
	}
})

var (
	envoyInstance             *localhelpers.EnvoyInstance
	glooInstance              *localhelpers.GlooInstance
	functionDiscoveryInstance *localhelpers.FunctionDiscoveryInstance
	natsStreamingInstance     *localhelpers.NatsStreamingInstance
)

var _ = BeforeEach(func() {
	var err error
	envoyInstance, err = envoyFactory.NewEnvoyInstance()
	Expect(err).NotTo(HaveOccurred())
	glooInstance, err = glooFactory.NewGlooInstance()
	Expect(err).NotTo(HaveOccurred())
	if functionDiscoveryFactory != nil {
		functionDiscoveryInstance, _ = functionDiscoveryFactory.NewFunctionDiscoveryInstance()
	}
	if natsStreamingFactory != nil {
		natsStreamingInstance, _ = natsStreamingFactory.NewNatsStreamingInstance()
	}
})

var _ = AfterEach(func() {
	if envoyInstance != nil {
		envoyInstance.Clean()
	}
	if glooInstance != nil {
		glooInstance.Clean()
	}
	if functionDiscoveryInstance != nil {
		functionDiscoveryInstance.Clean()
	}
	if natsStreamingInstance != nil {
		natsStreamingInstance.Clean()
	}
})
