package kubeutils

import (
	"context"

	"k8s.io/client-go/rest"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Inspired by: https://github.com/solo-io/gloo-mesh-enterprise/blob/main/pkg/utils/kubeutils/pods.go

// GetPodsForDeployment gets all pods backing a deployment
func GetPodsForDeployment(
	ctx context.Context,
	restConfig *rest.Config,
	deploymentName string,
	deploymentNamespace string,
) ([]string, error) {
	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	deployment, err := kubeClient.AppsV1().Deployments(deploymentNamespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	matchLabels := deployment.Spec.Selector.MatchLabels
	listOptions := (&client.ListOptions{
		LabelSelector: labels.SelectorFromSet(matchLabels),
		FieldSelector: fields.Set{"status.phase": "Running"}.AsSelector(),
	}).AsListOptions()

	podList, err := kubeClient.CoreV1().Pods(deploymentNamespace).List(ctx, *listOptions)
	if err != nil {
		return nil, err
	}

	pods := make([]string, len(podList.Items))
	for i := range podList.Items {
		pods[i] = podList.Items[i].Name
	}

	return pods, nil
}

// GetPodsForService gets all pods backing a deployment
func GetPodsForService(
	ctx context.Context,
	restConfig *rest.Config,
	serviceName string,
	serviceNamespace string,
) ([]string, error) {
	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	service, err := kubeClient.CoreV1().Services(serviceNamespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	matchLabels := service.Spec.Selector
	listOptions := (&client.ListOptions{LabelSelector: labels.SelectorFromSet(matchLabels)}).AsListOptions()

	podList, err := kubeClient.CoreV1().Pods(serviceNamespace).List(ctx, *listOptions)
	if err != nil {
		return nil, err
	}

	pods := make([]string, len(podList.Items))
	for i := range podList.Items {
		pods[i] = podList.Items[i].Name
	}

	return pods, nil
}
