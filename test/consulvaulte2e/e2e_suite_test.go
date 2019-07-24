package consulvaulte2e_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/solo-kit/test/helpers"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
)

var (
	envoyFactory  *services.EnvoyFactory
	consulFactory *services.ConsulFactory
	vaultFactory  *services.VaultFactory
)

var _ = BeforeSuite(func() {
	var err error
	envoyFactory, err = services.NewEnvoyFactory()
	Expect(err).NotTo(HaveOccurred())
	consulFactory, err = services.NewConsulFactory()
	Expect(err).NotTo(HaveOccurred())
	vaultFactory, err = services.NewVaultFactory()
	Expect(err).NotTo(HaveOccurred())

})

var _ = AfterSuite(func() {
	_ = envoyFactory.Clean()
	_ = consulFactory.Clean()
	_ = vaultFactory.Clean()
})

func TestE2e(t *testing.T) {

	// set default port to an unprivileged port for local testing.
	defaults.HttpPort = 8081

	helpers.RegisterCommonFailHandlers()
	helpers.SetupLog()
	// RegisterFailHandler(Fail)
	RunSpecs(t, "E2e Suite")
}
