//go:build e2e

package assertions

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// getChartLabelSelector returns the appropriate label selector based on the chart type
func (p *Provider) getChartLabelSelector() string {
	chartType := p.installContext.GetChartType()
	if chartType == "agentgateway" {
		return "app.kubernetes.io/name=agentgateway"
	}
	return "app.kubernetes.io/name=kgateway"
}

func (p *Provider) EventuallyGatewayInstallSucceeded(ctx context.Context) {
	p.expectInstallContextDefined()

	p.EventuallyPodsRunning(ctx, p.installContext.InstallNamespace,
		metav1.ListOptions{
			LabelSelector: p.getChartLabelSelector(),
		})
}

func (p *Provider) EventuallyKgatewayUninstallSucceeded(ctx context.Context) {
	p.expectInstallContextDefined()

	p.EventuallyPodsNotExist(ctx, p.installContext.InstallNamespace,
		metav1.ListOptions{
			LabelSelector: p.getChartLabelSelector(),
		})
}

func (p *Provider) EventuallyKgatewayUpgradeSucceeded(ctx context.Context, version string) {
	p.expectInstallContextDefined()

	p.EventuallyPodsRunning(ctx, p.installContext.InstallNamespace,
		metav1.ListOptions{
			LabelSelector: p.getChartLabelSelector(),
		})
}
