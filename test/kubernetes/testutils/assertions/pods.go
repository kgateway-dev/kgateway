package assertions

import (
	"context"
	"time"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EventuallyPodsMatches asserts that the pod(s) in the given namespace matches the provided matcher
func (p *Provider) EventuallyPodsMatches(ctx context.Context, podNamespace string, listOpt metav1.ListOptions, matcher types.GomegaMatcher) {
	p.Gomega.Eventually(func(g gomega.Gomega) {
		proxyPods, err := p.clusterContext.Clientset.CoreV1().Pods(podNamespace).List(ctx, listOpt)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to list pods")
		g.Expect(proxyPods.Items).NotTo(gomega.BeEmpty(), "No pods found")
		for _, pod := range proxyPods.Items {
			g.Expect(pod).To(matcher)
		}
	}).
		WithTimeout(time.Second*60).
		WithPolling(time.Second*5).
		Should(gomega.Succeed(), "Failed to match pod")
}
