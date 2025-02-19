package assertions

import (
	"context"

	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

func (p *Provider) EventuallyKgatewayInstallSucceeded(ctx context.Context) {
	p.expectInstallContextDefined()

	p.EventuallyReadyReplicas(ctx, metav1.ObjectMeta{
		Name:      kubeutils.GlooDeploymentName,
		Namespace: p.installContext.InstallNamespace,
	}, gomega.Equal(1))
}

func (p *Provider) EventuallyKgatewayUninstallSucceeded(ctx context.Context) {
	p.expectInstallContextDefined()

	p.EventuallyPodsNotExist(ctx, p.installContext.InstallNamespace,
		metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=kgateway",
		})
}

func (p *Provider) EventuallyKgatewayUpgradeSucceeded(ctx context.Context, version string) {
	p.expectInstallContextDefined()

	p.EventuallyReadyReplicas(ctx, metav1.ObjectMeta{
		Name:      kubeutils.GlooDeploymentName,
		Namespace: p.installContext.InstallNamespace,
	}, gomega.Equal(1))
}
