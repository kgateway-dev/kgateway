package matchers

import (
	"fmt"
	"log"

	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
)

// ExpectedPod is a struct that represents the expected pod.
type ExpectedPod struct {
	// ContainerName is the name of the container. Optional.
	ContainerName string

	// Status is the pod phase status (e.g. Running, Pending, Succeeded, Failed). Optional.
	Status corev1.PodPhase

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
	if pm.expectedPod.ContainerName != "" {
		foundContainer := false
		for _, container := range pod.Spec.Containers {
			if container.Name == pm.expectedPod.ContainerName {
				foundContainer = true
			}
		}
		if !foundContainer {
			log.Printf("expected pod to have container '%s', but it was not found", pm.expectedPod.ContainerName)
			return false, nil
		}
	}

	if pm.expectedPod.Status != "" {
		if pod.Status.Phase != pm.expectedPod.Status {
			log.Printf("expected pod to have status %s, but it was %s", pm.expectedPod.Status, pod.Status.Phase)
			return false, nil
		}
	}

	return true, nil
}

func (pm *podMatcher) FailureMessage(actual interface{}) string {
	var errorMsg string
	if pm.expectedPod.ContainerName != "" {
		errorMsg += fmt.Sprintf("Expected pod to have container '%s', but it was not found", pm.expectedPod.ContainerName)
	}
	if pm.expectedPod.Status != "" {
		errorMsg += fmt.Sprintf("Expected pod to have status '%s', but it was not found", pm.expectedPod.Status)
	}
	return errorMsg
}

func (pm *podMatcher) NegatedFailureMessage(actual interface{}) string {
	pod := actual.(corev1.Pod)

	var errorMsg string
	if pm.expectedPod.ContainerName != "" {
		containers := ""
		for _, container := range pod.Spec.Containers {
			containers += container.Name + ", "
		}
		errorMsg += fmt.Sprintf("Expected pod to have container '%s', but it found %s", pm.expectedPod.ContainerName, containers)
	}
	if pm.expectedPod.Status != "" {
		errorMsg += fmt.Sprintf("Expected pod to have status '%s', but it found %s", pm.expectedPod.Status, pod.Status.Phase)
	}
	return errorMsg
}
