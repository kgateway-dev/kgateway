package e2e_test

import (
	"testing"

	"github.com/solo-io/gloo/test/services/envoy"

	"github.com/solo-io/gloo/test/ginkgo/labels"

	"github.com/solo-io/gloo/test/e2e"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"

	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/test/helpers"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
)

func TestE2e(t *testing.T) {
	// set default port to an unprivileged port for local testing.
	defaults.HttpPort = 8081

	helpers.RegisterCommonFailHandlers()
	helpers.SetupLog()
	contextutils.SetLogLevel(zapcore.DebugLevel)
	RunSpecs(t, "E2E Suite", Label(labels.E2E))
}

var (
	envoyFactory  envoy.Factory
	consulFactory *services.ConsulFactory
	vaultFactory  *services.VaultFactory

	testContextFactory *e2e.TestContextFactory

	writeNamespace = defaults.GlooSystem
)

var _ = BeforeSuite(func() {
	var err error
	envoyFactory = envoy.NewFactory()

	consulFactory, err = services.NewConsulFactory()
	Expect(err).NotTo(HaveOccurred())
	vaultFactory, err = services.NewVaultFactory()
	Expect(err).NotTo(HaveOccurred())

	testContextFactory = &e2e.TestContextFactory{
		EnvoyFactory:  envoyFactory,
		VaultFactory:  vaultFactory,
		ConsulFactory: consulFactory,
	}
})

var _ = AfterSuite(func() {
	envoyFactory.Clean()
	_ = consulFactory.Clean()
	_ = vaultFactory.Clean()
})
