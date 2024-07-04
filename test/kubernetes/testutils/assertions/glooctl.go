package assertions

import (
	"context"
	"time"

	. "github.com/onsi/gomega"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/check"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/version"
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
		WithTimeout(time.Second * 120).
		WithPolling(time.Second).
		Should(Succeed())
}

// EventuallyMatchesVersion asserts that `glooctl version` eventually responds with the expected server version
func (p *Provider) EventuallyMatchesVersion(ctx context.Context, serverVersion string) {
	p.expectGlooGatewayContextDefined()

	k := version.NewKube(p.glooGatewayContext.InstallNamespace, p.clusterContext.KubeContext)

	p.Gomega.Eventually(func(innerG Gomega) {
		contextWithCancel, cancel := context.WithCancel(ctx)
		defer cancel()
		csv, err := version.GetClientServerVersions(contextWithCancel, k)
		innerG.Expect(err).NotTo(HaveOccurred(), "can get client server versions with glooctl")
		innerG.Expect(csv.GetServer()).To(HaveLen(1), "has detected gloo deployment")
		kServer := csv.GetServer()[0].GetKubernetes()
		innerG.Expect(kServer.GetContainers()).ToNot(BeEmpty(), "has containers for gloo deployment")
		innerG.Expect(kServer.GetContainers()[0].GetTag()).To(Equal(serverVersion), "has expected tag")
	}).
		WithContext(ctx).
		// These are some basic defaults that we expect to work in most cases
		// We can make these configurable if need be, though most installations
		// Should be able to become healthy within this window
		WithTimeout(time.Second * 120).
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

func (p *Provider) EventuallyUpgradeSucceeded(ctx context.Context, version string) {
	p.expectGlooGatewayContextDefined()

	p.EventuallyMatchesVersion(ctx, version)

	// Check that everything is OK
	p.EventuallyCheckResourcesOk(ctx)
}
