//go:build e2e

package common

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
)

const (
	// controllerMetricsPort is the controller container port that serves /metrics.
	controllerMetricsPort = "9092"
	// scrapeAttemptTimeout bounds each proxied request so one stalled pod cannot use up the
	// caller's whole retry budget.
	scrapeAttemptTimeout = 10 * time.Second
)

// ScrapeControllerMetrics returns the /metrics body of the first ready kgateway controller pod in
// the install namespace. It goes through the API server's pod proxy, so it needs no curl pod and
// no Service in front of the controller, and works from hosts that cannot reach pod or cluster IPs.
// It makes one attempt per ready pod; callers retry.
func ScrapeControllerMetrics(ctx context.Context, installation *e2e.TestInstallation) ([]byte, error) {
	namespace := installation.Metadata.InstallNamespace
	pods := installation.ClusterContext.Clientset.CoreV1().Pods(namespace)
	list, err := pods.List(ctx, metav1.ListOptions{
		LabelSelector: testdefaults.WellKnownAppLabel + "=kgateway",
	})
	if err != nil {
		return nil, fmt.Errorf("list kgateway controller pods in %s: %w", namespace, err)
	}

	var errs []error
	for _, pod := range list.Items {
		if !podReady(&pod) {
			continue
		}
		attemptCtx, cancel := context.WithTimeout(ctx, scrapeAttemptTimeout)
		body, err := pods.ProxyGet("http", pod.Name, controllerMetricsPort, "/metrics", nil).DoRaw(attemptCtx)
		cancel()
		if err == nil {
			return body, nil
		}
		errs = append(errs, fmt.Errorf("scrape controller pod %s/%s: %w", namespace, pod.Name, err))
	}
	if len(errs) == 0 {
		return nil, fmt.Errorf("no ready kgateway controller pods in %s", namespace)
	}
	return nil, errors.Join(errs...)
}

func podReady(pod *corev1.Pod) bool {
	if pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
