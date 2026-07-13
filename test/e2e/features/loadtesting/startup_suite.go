//go:build e2e

package loadtesting

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	typedappsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	controllerDeploymentName      = "kgateway"
	startupBenchmarkAnnotation    = "e2e.kgateway.dev/startup-benchmark-at"
	startupBenchmarkPollInterval  = 500 * time.Millisecond
	startupBenchmarkReadyDeadline = 5 * time.Minute
)

type startupBenchmarkResult struct {
	Namespace         string
	Deployment        string
	GatewayAPIVersion string
	GatewayAPIChannel string
	ValidationMode    string
	Image             string
	Generation        int64
	DesiredReplicas   int32
	Duration          time.Duration
	LastStatus        string
	PodSnapshot       string
	RecentEvents      string
}

func (s *LoadTestingSuite) measureControllerRolloutStartup() (*startupBenchmarkResult, error) {
	namespace := s.testInstallation.Metadata.InstallNamespace
	deployments := s.testInstallation.ClusterContext.Clientset.AppsV1().Deployments(namespace)

	deployment, err := deployments.Get(s.ctx, controllerDeploymentName, metav1.GetOptions{})
	if err != nil {
		return &startupBenchmarkResult{
			Namespace:  namespace,
			Deployment: controllerDeploymentName,
		}, fmt.Errorf("get controller deployment: %w", err)
	}

	desiredReplicas := int32(1)
	if deployment.Spec.Replicas != nil {
		desiredReplicas = *deployment.Spec.Replicas
	}
	if desiredReplicas < 1 {
		return &startupBenchmarkResult{
			Namespace:       namespace,
			Deployment:      controllerDeploymentName,
			DesiredReplicas: desiredReplicas,
		}, fmt.Errorf("controller deployment has no desired replicas")
	}

	version, channel := s.detectGatewayAPIMetadata()
	result := &startupBenchmarkResult{
		Namespace:         namespace,
		Deployment:        controllerDeploymentName,
		GatewayAPIVersion: version,
		GatewayAPIChannel: channel,
		ValidationMode:    controllerEnvValue(deployment, "KGW_VALIDATION_MODE"),
		Image:             controllerImage(deployment),
		DesiredReplicas:   desiredReplicas,
	}

	patched, err := rolloutRestartDeployment(s.ctx, deployments)
	if err != nil {
		return result, fmt.Errorf("restart controller deployment: %w", err)
	}

	start := time.Now()
	result.Generation = patched.Generation

	duration, lastStatus, err := s.waitForControllerRollout(patched.Generation, desiredReplicas, start)
	result.Duration = duration
	result.LastStatus = lastStatus
	result.PodSnapshot = s.controllerPodSnapshot(namespace, deployment.Spec.Selector)
	result.RecentEvents = s.recentControllerEvents(namespace, deployment.Spec.Selector)
	if err != nil {
		return result, err
	}

	return result, nil
}

func rolloutRestartDeployment(ctx context.Context, deployments typedappsv1.DeploymentInterface) (*appsv1.Deployment, error) {
	patch, err := json.Marshal(map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"metadata": map[string]any{
					"annotations": map[string]string{
						startupBenchmarkAnnotation: time.Now().UTC().Format(time.RFC3339Nano),
					},
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	return deployments.Patch(ctx, controllerDeploymentName, types.MergePatchType, patch, metav1.PatchOptions{})
}

func (s *LoadTestingSuite) waitForControllerRollout(generation int64, desiredReplicas int32, start time.Time) (time.Duration, string, error) {
	ctx, cancel := context.WithTimeout(s.ctx, startupBenchmarkReadyDeadline)
	defer cancel()

	deployments := s.testInstallation.ClusterContext.Clientset.AppsV1().Deployments(s.testInstallation.Metadata.InstallNamespace)
	ticker := time.NewTicker(startupBenchmarkPollInterval)
	defer ticker.Stop()

	var lastStatus string
	for {
		deployment, err := deployments.Get(ctx, controllerDeploymentName, metav1.GetOptions{})
		if err != nil {
			lastStatus = fmt.Sprintf("get deployment failed: %v", err)
		} else {
			lastStatus = deploymentRolloutStatus(deployment)
			if deploymentRolloutReady(deployment, generation, desiredReplicas) {
				return time.Since(start), lastStatus, nil
			}
		}

		select {
		case <-ctx.Done():
			if lastStatus == "" {
				lastStatus = "deployment status not observed"
			}
			return time.Since(start), lastStatus, fmt.Errorf("controller deployment was not ready within %s", startupBenchmarkReadyDeadline)
		case <-ticker.C:
		}
	}
}

func deploymentRolloutReady(deployment *appsv1.Deployment, generation int64, desiredReplicas int32) bool {
	return deployment.Status.ObservedGeneration >= generation &&
		deployment.Status.Replicas == desiredReplicas &&
		deployment.Status.UpdatedReplicas == desiredReplicas &&
		deployment.Status.ReadyReplicas == desiredReplicas &&
		deployment.Status.AvailableReplicas == desiredReplicas &&
		deployment.Status.UnavailableReplicas == 0
}

func deploymentRolloutStatus(deployment *appsv1.Deployment) string {
	conditions := make([]string, 0, len(deployment.Status.Conditions))
	for _, condition := range deployment.Status.Conditions {
		conditions = append(conditions, fmt.Sprintf("%s=%s/%s", condition.Type, condition.Status, condition.Reason))
	}

	return fmt.Sprintf(
		"generation=%d observed_generation=%d replicas=%d updated=%d ready=%d available=%d unavailable=%d conditions=[%s]",
		deployment.Generation,
		deployment.Status.ObservedGeneration,
		deployment.Status.Replicas,
		deployment.Status.UpdatedReplicas,
		deployment.Status.ReadyReplicas,
		deployment.Status.AvailableReplicas,
		deployment.Status.UnavailableReplicas,
		strings.Join(conditions, ","),
	)
}

func (s *LoadTestingSuite) detectGatewayAPIMetadata() (string, string) {
	crd := &apiextensionsv1.CustomResourceDefinition{}
	err := s.testInstallation.ClusterContext.Client.Get(
		s.ctx,
		client.ObjectKey{Name: "gateways.gateway.networking.k8s.io"},
		crd,
	)
	if err != nil {
		return "unknown", "unknown"
	}

	return annotationOrUnknown(crd.Annotations, "gateway.networking.k8s.io/bundle-version"),
		annotationOrUnknown(crd.Annotations, "gateway.networking.k8s.io/channel")
}

func annotationOrUnknown(annotations map[string]string, key string) string {
	if value := annotations[key]; value != "" {
		return value
	}
	return "unknown"
}

func controllerImage(deployment *appsv1.Deployment) string {
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == "controller" {
			return container.Image
		}
	}
	if len(deployment.Spec.Template.Spec.Containers) > 0 {
		return deployment.Spec.Template.Spec.Containers[0].Image
	}
	return "unknown"
}

func controllerEnvValue(deployment *appsv1.Deployment, name string) string {
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name != "controller" {
			continue
		}
		for _, env := range container.Env {
			if env.Name == name {
				return env.Value
			}
		}
	}
	return "unknown"
}

func (s *LoadTestingSuite) controllerPodSnapshot(namespace string, selector *metav1.LabelSelector) string {
	pods, err := s.controllerPods(namespace, selector)
	if err != nil {
		return fmt.Sprintf("list pods failed: %v", err)
	}
	if len(pods) == 0 {
		return "no controller pods found"
	}

	snapshots := make([]string, 0, len(pods))
	for _, pod := range pods {
		containerStatuses := make([]string, 0, len(pod.Status.ContainerStatuses))
		for _, containerStatus := range pod.Status.ContainerStatuses {
			containerStatuses = append(containerStatuses, fmt.Sprintf(
				"%s ready=%t restarts=%d state=%s",
				containerStatus.Name,
				containerStatus.Ready,
				containerStatus.RestartCount,
				containerState(containerStatus.State),
			))
		}

		snapshots = append(snapshots, fmt.Sprintf(
			"%s phase=%s ready=%s containers=[%s]",
			pod.Name,
			pod.Status.Phase,
			podReadyStatus(pod),
			strings.Join(containerStatuses, ";"),
		))
	}

	sort.Strings(snapshots)
	return strings.Join(snapshots, " | ")
}

func (s *LoadTestingSuite) recentControllerEvents(namespace string, selector *metav1.LabelSelector) string {
	pods, err := s.controllerPods(namespace, selector)
	if err != nil {
		return fmt.Sprintf("list pods failed: %v", err)
	}
	if len(pods) == 0 {
		return "no controller pods found"
	}

	podNames := make(map[string]struct{}, len(pods))
	for _, pod := range pods {
		podNames[pod.Name] = struct{}{}
	}

	events, err := s.testInstallation.ClusterContext.Clientset.CoreV1().Events(namespace).List(s.ctx, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("involvedObject.kind", "Pod").String(),
	})
	if err != nil {
		return fmt.Sprintf("list events failed: %v", err)
	}

	matchingEvents := make([]corev1.Event, 0)
	for _, event := range events.Items {
		if _, ok := podNames[event.InvolvedObject.Name]; ok {
			matchingEvents = append(matchingEvents, event)
		}
	}
	if len(matchingEvents) == 0 {
		return "no recent controller pod events"
	}

	sort.Slice(matchingEvents, func(i, j int) bool {
		return eventTime(matchingEvents[i]).Before(eventTime(matchingEvents[j]))
	})

	const maxEvents = 8
	if len(matchingEvents) > maxEvents {
		matchingEvents = matchingEvents[len(matchingEvents)-maxEvents:]
	}

	summaries := make([]string, 0, len(matchingEvents))
	for _, event := range matchingEvents {
		summaries = append(summaries, fmt.Sprintf(
			"%s %s/%s %s",
			eventTime(event).Format(time.RFC3339),
			event.Type,
			event.Reason,
			strings.TrimSpace(event.Message),
		))
	}
	return strings.Join(summaries, " | ")
}

func (s *LoadTestingSuite) controllerPods(namespace string, selector *metav1.LabelSelector) ([]corev1.Pod, error) {
	listOptions := metav1.ListOptions{}
	if selector != nil {
		listOptions.LabelSelector = metav1.FormatLabelSelector(selector)
	}

	pods, err := s.testInstallation.ClusterContext.Clientset.CoreV1().Pods(namespace).List(s.ctx, listOptions)
	if err != nil {
		return nil, err
	}
	return pods.Items, nil
}

func containerState(state corev1.ContainerState) string {
	switch {
	case state.Running != nil:
		return "running"
	case state.Waiting != nil:
		return "waiting:" + state.Waiting.Reason
	case state.Terminated != nil:
		return "terminated:" + state.Terminated.Reason
	default:
		return "unknown"
	}
}

func podReadyStatus(pod corev1.Pod) string {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return string(condition.Status)
		}
	}
	return "unknown"
}

func eventTime(event corev1.Event) time.Time {
	if !event.EventTime.IsZero() {
		return event.EventTime.Time
	}
	if !event.LastTimestamp.IsZero() {
		return event.LastTimestamp.Time
	}
	return event.CreationTimestamp.Time
}

func (s *LoadTestingSuite) logStartupBenchmarkResult(result *startupBenchmarkResult, success bool) {
	s.T().Logf(
		"startup_benchmark_result success=%t namespace=%s deployment=%s generation=%d desired_replicas=%d gateway_api_version=%s gateway_api_channel=%s validation_mode=%s image=%s duration_ms=%d duration=%s status=%q pods=%q events=%q",
		success,
		result.Namespace,
		result.Deployment,
		result.Generation,
		result.DesiredReplicas,
		result.GatewayAPIVersion,
		result.GatewayAPIChannel,
		result.ValidationMode,
		result.Image,
		result.Duration.Milliseconds(),
		result.Duration,
		result.LastStatus,
		result.PodSnapshot,
		result.RecentEvents,
	)
}
