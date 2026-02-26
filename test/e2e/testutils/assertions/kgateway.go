//go:build e2e

package assertions

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// KgatewayLabelSelector is the label selector for kgateway pods
	KgatewayLabelSelector = "app.kubernetes.io/name=kgateway"
)

func (p *Provider) EventuallyGatewayInstallSucceeded(ctx context.Context) {
	p.expectInstallContextDefined()

	p.EventuallyPodsRunning(ctx, p.installContext.InstallNamespace,
		metav1.ListOptions{
			LabelSelector: KgatewayLabelSelector,
		})
}

func (p *Provider) EventuallyGatewayUninstallSucceeded(ctx context.Context) {
	p.expectInstallContextDefined()

	p.EventuallyPodsNotExist(ctx, p.installContext.InstallNamespace,
		metav1.ListOptions{
			LabelSelector: KgatewayLabelSelector,
		})
}

func (p *Provider) EventuallyGatewayUpgradeSucceeded(ctx context.Context, version string) {
	p.expectInstallContextDefined()

	p.EventuallyPodsRunning(ctx, p.installContext.InstallNamespace,
		metav1.ListOptions{
			LabelSelector: KgatewayLabelSelector,
		})
}

// EventuallyKgatewayInstallSucceeded verifies that the kgateway chart installation has succeeded.
func (p *Provider) EventuallyKgatewayInstallSucceeded(ctx context.Context) {
	p.expectInstallContextDefined()

	p.EventuallyPodsRunning(ctx, p.installContext.InstallNamespace,
		metav1.ListOptions{
			LabelSelector: KgatewayLabelSelector,
		})
}

// EventuallyKgatewayUninstallSucceeded verifies that the kgateway chart has been uninstalled.
func (p *Provider) EventuallyKgatewayUninstallSucceeded(ctx context.Context) {
	p.expectInstallContextDefined()

	p.EventuallyPodsNotExist(ctx, p.installContext.InstallNamespace,
		metav1.ListOptions{
			LabelSelector: KgatewayLabelSelector,
		})
}

// EventuallyKgatewayUpgradeSucceeded verifies that the kgateway chart upgrade has succeeded.
func (p *Provider) EventuallyKgatewayUpgradeSucceeded(ctx context.Context, version string) {
	p.expectInstallContextDefined()

	p.EventuallyPodsRunning(ctx, p.installContext.InstallNamespace,
		metav1.ListOptions{
			LabelSelector: KgatewayLabelSelector,
		})
}
