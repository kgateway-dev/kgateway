//go:build ignore

package assertions

import (
	"context"
)

func (p *Provider) EventuallyInstallationSucceeded(ctx context.Context) {
	p.expectKgatewayContextDefined()

	// TODO check other things here, e.g. expected pods are up
}

func (p *Provider) EventuallyUninstallationSucceeded(ctx context.Context) {
	p.expectKgatewayContextDefined()

	p.ExpectNamespaceNotExist(ctx, p.kgatewayContext.InstallNamespace)
}

func (p *Provider) EventuallyUpgradeSucceeded(ctx context.Context, version string) {
	p.expectKgatewayContextDefined()

	// TODO check other things here, e.g. expected pods are up
}
