package bootstrap_test

import (
	"testing"

	"github.com/hashicorp/consul/api"
	"github.com/solo-io/gloo/test/services"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBootstrap(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bootstrap Suite")
}

var (
	consulFactory  *services.ConsulFactory
	consulInstance *services.ConsulInstance
	client         *api.Client
)

var _ = BeforeSuite(func() {
	var err error
	consulFactory, err = services.NewConsulFactory()
	Expect(err).NotTo(HaveOccurred())
	client, err = api.NewClient(api.DefaultConfig())
	Expect(err).NotTo(HaveOccurred())

})

var _ = AfterSuite(func() {
	_ = consulFactory.Clean()
})
