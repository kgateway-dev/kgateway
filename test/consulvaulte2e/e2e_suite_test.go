package consulvaulte2e_test

import (
	"os"
	"testing"

	"github.com/solo-io/gloo/projects/gloo/constants"

	"github.com/solo-io/gloo/test/services/envoy"

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
	envoyFactory  envoy.Factory
	consulFactory *services.ConsulFactory
	vaultFactory  *services.VaultFactory
)

var _ = BeforeSuite(func() {
	testhelpers.ValidateRequirementsAndNotifyGinkgo(
		testhelpers.Consul(),
		testhelpers.Vault(),
	)

	var err error
	envoyFactory = envoy.NewFactory()

	consulFactory, err = services.NewConsulFactory()
	Expect(err).NotTo(HaveOccurred())
	vaultFactory, err = services.NewVaultFactory()
	Expect(err).NotTo(HaveOccurred())

	// The consulvaulte2e test suite is not run against a k8s cluster, so we must disable the features that require a k8s cluster
	err = os.Setenv(constants.GlooGatewayEnableK8sGwControllerEnv, "false")
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	envoyFactory.Clean()
	_ = consulFactory.Clean()
	_ = vaultFactory.Clean()

	err := os.Unsetenv(constants.GlooGatewayEnableK8sGwControllerEnv)
	Expect(err).NotTo(HaveOccurred())
})
