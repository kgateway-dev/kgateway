package assertions

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
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

// PodHasContainersMatcher returns a GomegaMatcher that checks whether a pod has expected container with the given name.
func PodHasContainersMatcher(containerName string, image string) types.GomegaMatcher {
	return &podHasContainerMatcher{
		containerName: containerName,
		image:         image,
	}
}

type podHasContainerMatcher struct {
	containerName string
	image         string
}

func (m *podHasContainerMatcher) Match(actual interface{}) (bool, error) {
	pod, ok := actual.(corev1.Pod)
	if !ok {
		return false, fmt.Errorf("expected a pod, got %T", actual)
	}
	if m.containerName == "" {
		return false, fmt.Errorf("expected container name cannot be empty")
	}
	for _, container := range pod.Spec.Containers {
		if container.Name == m.containerName {
			return true, nil
		}
		if m.image != "" {
			if container.Image == m.image {
				return true, nil
			}

		}
	}
	return false, nil
}

func (m *podHasContainerMatcher) FailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected pod to have container '%s', but it was not found", m.containerName)
}

func (m *podHasContainerMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected pod not to have container '%s', but it was found", m.containerName)
}
