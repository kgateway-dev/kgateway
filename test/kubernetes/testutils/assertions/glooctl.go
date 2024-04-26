package assertions

import (
	"context"
	"time"

	. "github.com/onsi/gomega"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/check"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
)

// EventuallyCheckResourcesOk asserts that `glooctl check` eventually responds Ok
func (p *Provider) EventuallyCheckResourcesOk(ctx context.Context) {
	p.expectGlooGatewayContextDefined()

	p.Gomega.Eventually(func(innerG Gomega) {
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

func (p *Provider) EventuallyInstallationSucceeded(ctx context.Context) {
	p.expectGlooGatewayContextDefined()

	// Check that everything is OK
	p.EventuallyCheckResourcesOk(ctx)
}

func (p *Provider) EventuallyUninstallationSucceeded(ctx context.Context) {
	p.expectGlooGatewayContextDefined()

	p.ExpectNamespaceNotExist(ctx, p.glooGatewayContext.InstallNamespace)
}
