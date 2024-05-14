package assertions

import (
	"context"
	"time"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/solo-io/gloo/test/gomega/matchers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EventuallyPodsRunning asserts that the pod(s) are in the ready state
func (p *Provider) EventuallyPodsRunning(ctx context.Context, podNamespace string, listOpt metav1.ListOptions) {
	p.EventuallyPodsMatches(ctx, podNamespace, listOpt, matchers.PodMatches(matchers.ExpectedPod{Status: corev1.PodRunning}))
}

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
		WithTimeout(time.Second*120).
		WithPolling(time.Second*5).
		Should(gomega.Succeed(), "Failed to match pod")
}
