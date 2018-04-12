package consul

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/helpers/local"
)

func TestConsul(t *testing.T) {
	if os.Getenv("RUN_CONSUL_TESTS") != "1" {
		log.Printf("This test downloads and runs consul and is disabled by default. To enable, set RUN_CONSUL_TESTS=1 in your env.")
		return
	}
	RegisterFailHandler(Fail)
	log.DefaultOut = GinkgoWriter
	RunSpecs(t, "Consul Suite")
}

var (
	consulFactory  *localhelpers.ConsulFactory
	consulInstance *localhelpers.ConsulInstance
	err            error
)

var _ = BeforeSuite(func() {
	consulFactory, err = localhelpers.NewConsulFactory()
	helpers.Must(err)
	consulInstance, err = consulFactory.NewConsulInstance()
	helpers.Must(err)
	err = consulInstance.Run()
	helpers.Must(err)
})

var _ = AfterSuite(func() {
	consulInstance.Clean()
	consulFactory.Clean()
})
