//go:build ignore

package clients

import (
	"github.com/kgateway-dev/kgateway/pkg/utils/kubeutils"
	"k8s.io/client-go/kubernetes"
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
