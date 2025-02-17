package assertions

import (
	"context"
)

func (p *Provider) EventuallyInstallationSucceeded(ctx context.Context) {
	p.expectInstallContextDefined()

	// TODO check other things here, e.g. expected pods are up
}

func (p *Provider) EventuallyUninstallationSucceeded(ctx context.Context) {
	p.expectInstallContextDefined()

	p.ExpectNamespaceNotExist(ctx, p.installContext.InstallNamespace)
}

func (p *Provider) EventuallyUpgradeSucceeded(ctx context.Context, version string) {
	p.expectInstallContextDefined()

	// TODO check other things here, e.g. expected pods are up
}
