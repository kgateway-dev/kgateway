package consulvaulte2e_test

import (
	"os"
	"testing"

	"github.com/solo-io/gloo/test/ginkgo/labels"

	testhelpers "github.com/solo-io/gloo/test/testutils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/solo-kit/test/helpers"
)

func TestE2e(t *testing.T) {
	// set KUBECONFIG to a nonexistent cfg.
	// this way we are also testing that Gloo can run without a kubeconfig present
	os.Setenv("KUBECONFIG", ".")

	helpers.RegisterCommonFailHandlers()
	helpers.SetupLog()
	RunSpecs(t, "Consul+Vault E2e Suite", Label(labels.E2E))
}

var (
	envoyFactory  *services.EnvoyFactory
	consulFactory *services.ConsulFactory
	vaultFactory  *services.VaultFactory
)

var _ = BeforeSuite(func() {
	testhelpers.ValidateRequirementsAndNotifyGinkgo(
		testhelpers.Consul(),
		testhelpers.Vault(),
	)

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
