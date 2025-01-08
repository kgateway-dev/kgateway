package kubeutils

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

// GetService gets the service from the provided name/namespace
func GetService(
	ctx context.Context,
	kubeClient *kubernetes.Clientset,
	serviceName string,
	serviceNamespace string,
) (*corev1.Service, error) {

	service, err := kubeClient.CoreV1().Services(serviceNamespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return service, nil
}
