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
	return func(ctx context.Context) {
		ginkgo.GinkgoHelper()

		p.AssertCheckResources(NewGomega(ginkgo.Fail), ctx)
	}
}

// AssertCheckResources asserts that `glooctl check` eventually responds Ok
func (p *Provider) AssertCheckResources(g Gomega, ctx context.Context) {
	p.assertGlooGatewayContextDefined(g)

	g.Eventually(func(innerG Gomega) {
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
		innerG.Expect(err).NotTo(HaveOccurred())
	}).
		WithContext(ctx).
		// These are some basic defaults that we expect to work in most cases
		// We can make these configurable if need be, though most installations
		// Should be able to become healthy within this window
		WithTimeout(time.Second * 90).
		WithPolling(time.Second).
		Should(Succeed())
}

func (p *Provider) AssertInstallationWasSuccessful(g Gomega, ctx context.Context) {
	p.assertGlooGatewayContextDefined(g)

	// Check that everything is OK
	p.AssertCheckResources(g, ctx)

	// Ensure gloo reaches valid state and doesn't continually re-sync
	// we can consider doing the same for leaking go-routines after resyncs
	// This is a time-consuming check, and could be removed from being run on every one of our tests,
	// and instead we could have a single test which performs this assertion
	kube2e.EventuallyReachesConsistentState(p.glooGatewayContext.InstallNamespace)
}

func (p *Provider) InstallationWasSuccessful() ClusterAssertion {
	return func(ctx context.Context) {
		ginkgo.GinkgoHelper()

		p.AssertInstallationWasSuccessful(NewGomega(ginkgo.Fail), ctx)
	}
}

func (p *Provider) AssertUninstallationWasSuccessful(g Gomega, ctx context.Context) {
	p.assertGlooGatewayContextDefined(g)

	p.AssertNamespaceNotExist(g, ctx, p.glooGatewayContext.InstallNamespace)
}

func (p *Provider) UninstallationWasSuccessful() ClusterAssertion {
	return func(ctx context.Context) {
		ginkgo.GinkgoHelper()

		p.AssertUninstallationWasSuccessful(NewGomega(ginkgo.Fail), ctx)
	}
}
