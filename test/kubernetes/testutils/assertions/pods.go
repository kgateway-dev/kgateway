package assertions

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/kube2e/helper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EventuallyPodsRunning asserts that the pod(s) are in the ready state
func (p *Provider) EventuallyPodsRunning(
	ctx context.Context,
	podNamespace string,
	listOpt metav1.ListOptions,
	timeout ...time.Duration,
) {
	p.EventuallyPodsMatches(ctx, podNamespace, listOpt, matchers.PodMatches(matchers.ExpectedPod{Status: corev1.PodRunning}), timeout...)
}

// EventuallyPodsMatches asserts that the pod(s) in the given namespace matches the provided matcher
func (p *Provider) EventuallyPodsMatches(
	ctx context.Context,
	podNamespace string,
	listOpt metav1.ListOptions,
	matcher types.GomegaMatcher,
	timeout ...time.Duration,
) {
	currentTimeout, pollingInterval := helper.GetTimeouts(timeout...)

	p.Gomega.Eventually(func(g gomega.Gomega) {
		pods, err := p.clusterContext.Clientset.CoreV1().Pods(podNamespace).List(ctx, listOpt)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to list pods")
		g.Expect(pods.Items).NotTo(gomega.BeEmpty(), "No pods found")
		for _, pod := range pods.Items {
			g.Expect(pod).To(matcher)
		}
	}).
		WithTimeout(currentTimeout).
		WithPolling(pollingInterval).
		Should(gomega.Succeed(), fmt.Sprintf("Failed to match pod in namespace %s", podNamespace))
}
