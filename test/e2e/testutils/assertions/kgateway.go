//go:build e2e

package assertions

import (
	"context"
	"slices"
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

func (p *Provider) EventuallyPodsHaveImageVersion(ctx context.Context, namespace string, labelSelector string, version string, skipContainers ...string) {
	p.Gomega.Eventually(func(g gomega.Gomega) {
		pods, err := p.clusterContext.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to list %s pods", labelSelector)
		g.Expect(pods.Items).NotTo(gomega.BeEmpty(), "no %s pods found", labelSelector)
		for _, pod := range pods.Items {
			for _, container := range pod.Spec.Containers {
				if slices.Contains(skipContainers, container.Name) {
					continue
				}
				// Strip digest (e.g. @sha256:...) before extracting the tag.
				image := container.Image
				if idx := strings.Index(image, "@"); idx != -1 {
					image = image[:idx]
				}
				i := strings.LastIndex(image, ":")
				g.Expect(i).To(gomega.BeNumerically(">", 0), "image %q missing tag", container.Image)
				g.Expect(container.Image[i+1:]).To(gomega.Equal(version),
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
// It also verifies the controller Deployment finished rolling out (the previous-version
// ReplicaSet is fully scaled down, so no old controller artifacts remain) and that no
// controller pod crash-looped during the upgrade.
func (p *Provider) EventuallyKgatewayUpgradeSucceeded(ctx context.Context, version string) {
	p.expectInstallContextDefined()

	ns := p.installContext.InstallNamespace
	p.EventuallyPodsRunning(ctx, ns, metav1.ListOptions{
		LabelSelector: KgatewayLabelSelector,
	})
	p.EventuallyDeploymentsRolledOut(ctx, ns, KgatewayLabelSelector)
	p.EventuallyPodsHaveImageVersion(ctx, ns, KgatewayLabelSelector, version)
}

// EventuallyDeploymentsRolledOut waits until every Deployment matching labelSelector has
// completed its rollout, applying the same criteria as `kubectl rollout status`:
//   - the latest spec has been observed (ObservedGeneration >= Generation),
//   - all replicas are updated to the new pod template (UpdatedReplicas == desired),
//   - no pods from a previous revision remain (Replicas == desired), and
//   - all replicas are available (AvailableReplicas == desired).
//
// The "no pods from a previous revision remain" check is what proves old artifacts were
// replaced rather than left running alongside the new ones after an upgrade.
func (p *Provider) EventuallyDeploymentsRolledOut(ctx context.Context, namespace string, labelSelector string) {
	p.Gomega.Eventually(func(g gomega.Gomega) {
		deployments, err := p.clusterContext.Clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to list %s deployments", labelSelector)
		g.Expect(deployments.Items).NotTo(gomega.BeEmpty(), "no %s deployments found", labelSelector)
		for i := range deployments.Items {
			d := &deployments.Items[i]
			desired := int32(1)
			if d.Spec.Replicas != nil {
				desired = *d.Spec.Replicas
			}
			g.Expect(d.Status.ObservedGeneration).To(gomega.BeNumerically(">=", d.Generation),
				"deployment %s has not observed its latest spec", d.Name)
			g.Expect(d.Status.UpdatedReplicas).To(gomega.Equal(desired),
				"deployment %s has not finished updating to the new revision", d.Name)
			g.Expect(d.Status.Replicas).To(gomega.Equal(desired),
				"deployment %s still has pods from a previous revision", d.Name)
			g.Expect(d.Status.AvailableReplicas).To(gomega.Equal(desired),
				"deployment %s does not have all replicas available", d.Name)
		}
	}).
		WithContext(ctx).
		WithTimeout(time.Second*120).
		WithPolling(time.Second*2).
		Should(gomega.Succeed(), "deployments %q should finish rolling out", labelSelector)
}
