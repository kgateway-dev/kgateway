package assertions

import (
	"context"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/check"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"time"
)

func (p *Provider) CheckResources() DiscreteAssertion {
	return func(ctx context.Context) {
		p.testingFramework.Helper()

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
