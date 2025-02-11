//go:build ignore

package e2e_test

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/test/services/envoy"

	"github.com/kgateway-dev/kgateway/v2/test/ginkgo/labels"

	"github.com/kgateway-dev/kgateway/v2/test/e2e"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"

	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/test/helpers"

	"github.com/kgateway-dev/kgateway/v2/test/services"

	"github.com/kgateway-dev/kgateway/v2/internal/gloo/pkg/defaults"
)

func TestE2e(t *testing.T) {
	// https://github.com/kgateway-dev/kgateway/issues/7147
	// We ought to add goroutine leak validation to these tests
	// See the attached issue for context around why this is valuable and previous attempts to incorporate it

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
