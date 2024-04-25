package assertions

import (
	"context"
	"time"

	"github.com/onsi/ginkgo/v2"

	"github.com/solo-io/gloo/test/kube2e"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/check"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

// CheckResources returns the ClusterAssertion that performs a `glooctl check`
func (p *Provider) CheckResources() ClusterAssertion {
	p.requiresGlooGatewayContext()

	return func(ctx context.Context) {
		ginkgo.GinkgoHelper()

		Eventually(func(g Gomega) {
			contextWithCancel, cancel := context.WithCancel(ctx)
			defer cancel()
			opts := &options.Options{
				Metadata: core.Metadata{
					Namespace: p.glooGatewayContext.InstallNamespace,
				},
				Top: options.Top{
					Ctx: contextWithCancel,
				},
			}
			err := check.CheckResources(contextWithCancel, printers.P{}, opts)
			g.Expect(err).NotTo(HaveOccurred())
		}).
			WithContext(ctx).
			// These are some basic defaults that we expect to work in most cases
			// We can make these configurable if need be, though most installations
			// Should be able to become healthy within this window
			WithTimeout(time.Second * 90).
			WithPolling(time.Second).
			Should(Succeed())
	}
}

func (p *Provider) InstallationWasSuccessful() ClusterAssertion {
	p.requiresGlooGatewayContext()

	return func(ctx context.Context) {
		ginkgo.GinkgoHelper()

		// Check that everything is OK
		p.CheckResources()(ctx)

		// Ensure gloo reaches valid state and doesn't continually re-sync
		// we can consider doing the same for leaking go-routines after resyncs
		// This is a time-consuming check, and could be removed from being run on every one of our tests,
		// and instead we could have a single test which performs this assertion
		kube2e.EventuallyReachesConsistentState(p.glooGatewayContext.InstallNamespace)
	}
}

func (p *Provider) UninstallationWasSuccessful() ClusterAssertion {
	p.requiresGlooGatewayContext()

	return func(ctx context.Context) {
		ginkgo.GinkgoHelper()

		p.NamespaceNotExist(p.glooGatewayContext.InstallNamespace)(ctx)
	}
}
