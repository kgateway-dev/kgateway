package clients

import (
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	glookubegateway "github.com/solo-io/gloo/projects/gateway2/pkg/api/gateway.gloo.solo.io/v1alpha1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	glooinstancev1 "github.com/solo-io/solo-apis/pkg/api/fed.solo.io/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	v1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
)

// MustClientset returns the Kubernetes Clientset, or panics
func MustClientset() *kubernetes.Clientset {
	restConfig, err := kubeutils.GetRestConfigWithKubeContext("")
	mustNotError(err)

	clientset, err := kubernetes.NewForConfig(restConfig)
	mustNotError(err)

	return clientset
}

func MustClientScheme() *runtime.Scheme {
	clientScheme := runtime.NewScheme()

	// K8s API resources
	err := corev1.AddToScheme(clientScheme)
	mustNotError(err)

	err = appsv1.AddToScheme(clientScheme)
	mustNotError(err)

	// Gloo resources
	err = gloov1.AddToScheme(clientScheme)
	mustNotError(err)
	err = gatewayv1.AddToScheme(clientScheme)
	mustNotError(err)
	err = glooinstancev1.AddToScheme(clientScheme)
	mustNotError(err)

	// Kubernetes Gateway API resources
	err = glookubegateway.AddToScheme(clientScheme)
	mustNotError(err)

	err = v1alpha2.AddToScheme(clientScheme)
	mustNotError(err)

	err = v1beta1.AddToScheme(clientScheme)
	mustNotError(err)

	err = v1.AddToScheme(clientScheme)
	mustNotError(err)

	return clientScheme
}

func mustNotError(err error) {
	if err != nil {
		panic(err)
	}
}
