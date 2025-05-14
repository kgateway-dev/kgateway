//go:build ignore

package clients

import (
	"k8s.io/client-go/kubernetes"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

// MustClientset returns the Kubernetes Clientset, or panics
func MustClientset() *kubernetes.Clientset {
	restConfig, err := kubeutils.GetRestConfigWithKubeContext("")
	mustNotError(err)

	clientset, err := kubernetes.NewForConfig(restConfig)
	mustNotError(err)

	return clientset
}

func mustNotError(err error) {
	if err != nil {
		panic(err)
	}
}
