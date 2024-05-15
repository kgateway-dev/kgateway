package matchers

import (
	"fmt"

	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
)

// ExpectedPod is a struct that represents the expected pod.
type ExpectedPod struct {
	// ContainerName is the name of the container. Required.
	ContainerName string

	// TODO(npolshak): Add more fields to match on as needed
}

// PodMatches returns a GomegaMatcher that checks whether a pod
func PodMatches(pod ExpectedPod) types.GomegaMatcher {
	return &podMatcher{expectedPod: pod}
}

type podMatcher struct {
	expectedPod ExpectedPod
}

func (pm *podMatcher) Match(actual interface{}) (bool, error) {
	pod, ok := actual.(corev1.Pod)
	if !ok {
		return false, fmt.Errorf("expected a pod, got %T", actual)
	}
	for _, container := range pod.Spec.Containers {
		if container.Name == pm.expectedPod.ContainerName {
			return true, nil
		}
	}
	return false, nil
}

func (pm *podMatcher) FailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected pod to have container '%s', but it was not found", pm.expectedPod.ContainerName)
}

func (pm *podMatcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected pod not to have container '%s', but it was found", pm.expectedPod.ContainerName)
}
