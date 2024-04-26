package clients

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	glooinstancev1 "github.com/solo-io/solo-apis/pkg/api/fed.solo.io/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"

	v1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
	"sigs.k8s.io/gateway-api/apis/v1beta1"

	glookubegateway "github.com/solo-io/gloo/projects/gateway2/pkg/api/gateway.gloo.solo.io/v1alpha1"
)

// MustClientset returns the Kubernetes Clientset, or panics
func MustClientset() *kubernetes.Clientset {
	ginkgo.GinkgoHelper()

	restConfig, err := kubeutils.GetRestConfigWithKubeContext("")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	clientset, err := kubernetes.NewForConfig(restConfig)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return clientset
}

func MustClientScheme() *runtime.Scheme {
	ginkgo.GinkgoHelper()

	clientScheme := runtime.NewScheme()

	// K8s API resources
	err := corev1.AddToScheme(clientScheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = appsv1.AddToScheme(clientScheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// Gloo resources
	err = glooinstancev1.AddToScheme(clientScheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// Kubernetes Gateway API resources
	err = glookubegateway.AddToScheme(clientScheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = v1alpha2.AddToScheme(clientScheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = v1beta1.AddToScheme(clientScheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = v1.AddToScheme(clientScheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return clientScheme
}
