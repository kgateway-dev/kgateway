//go:build e2e

package assertions

import (
	"context"
	"strings"
	"time"

	"github.com/onsi/gomega"
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

func (p *Provider) EventuallyPodHasImageVersion(ctx context.Context, namespace string, labelSelector string, version string) {
	p.Gomega.Eventually(func(g gomega.Gomega) {
		pods, err := p.clusterContext.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to list %s pods", labelSelector)
		g.Expect(pods.Items).NotTo(gomega.BeEmpty(), "no %s pods found", labelSelector)
		for _, pod := range pods.Items {
			for _, container := range pod.Spec.Containers {
				i := strings.LastIndex(container.Image, ":")
				g.Expect(i).To(gomega.BeNumerically(">", 0), "image %q missing tag", container.Image)
				g.Expect(container.Image[i+1:]).To(gomega.ContainSubstring(version),
					"pod %s container %s image tag should match version", pod.Name, container.Name)
			}
		}
	}).
		WithContext(ctx).
		WithTimeout(time.Second*60).
		WithPolling(time.Second*1).
		Should(gomega.Succeed(), "pods should have image tag %q", version)
}

// EventuallyKgatewayUpgradeSucceeded verifies that the kgateway chart upgrade has succeeded
// and that each kgateway pod's controller container image tag matches the expected version.
func (p *Provider) EventuallyKgatewayUpgradeSucceeded(ctx context.Context, version string) {
	p.expectInstallContextDefined()

	p.EventuallyPodsRunning(ctx, p.installContext.InstallNamespace,
		metav1.ListOptions{
			LabelSelector: KgatewayLabelSelector,
		})

	p.EventuallyPodHasImageVersion(ctx, p.installContext.InstallNamespace, KgatewayLabelSelector, version)
}
