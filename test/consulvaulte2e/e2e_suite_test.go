package consulvaulte2e_test

import (
	"log"
	"os"
	"testing"

	"github.com/onsi/ginkgo/reporters"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/solo-kit/test/helpers"
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
	if os.Getenv("RUN_VAULT_TESTS") != "1" {
		log.Printf("This test downloads and runs vault and is disabled by default. To enable, set RUN_VAULT_TESTS=1 in your env.")
		return
	}
	if os.Getenv("RUN_CONSUL_TESTS") != "1" {
		log.Printf("This test downloads and runs consul and is disabled by default. To enable, set RUN_CONSUL_TESTS=1 in your env.")
		return
	}

	// set KUBECONFIG to a nonexistent cfg.
	// this way we are also testing that Gloo can run without a kubeconfig present
	os.Setenv("KUBECONFIG", ".")

	helpers.RegisterCommonFailHandlers()
	helpers.SetupLog()
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "Consul+Vault E2e Suite", []Reporter{junitReporter})
}
