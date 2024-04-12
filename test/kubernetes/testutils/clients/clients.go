package clients

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
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
	ginkgo.GinkgoHelper()

	return MustClientsetWithContext("")
}

// MustClientsetWithContext returns the Kubernetes Clientset, or panics
func MustClientsetWithContext(kubeContext string) *kubernetes.Clientset {
	ginkgo.GinkgoHelper()

	restConfig := MustRestConfigWithContext(kubeContext)
	clientset, err := kubernetes.NewForConfig(restConfig)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return clientset
}

func MustClientScheme() *runtime.Scheme {
	ginkgo.GinkgoHelper()

	clientScheme := runtime.NewScheme()

	// k8s resources
	err := corev1.AddToScheme(clientScheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = appsv1.AddToScheme(clientScheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// k8s gateway resources
	err = v1alpha2.AddToScheme(clientScheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = v1beta1.AddToScheme(clientScheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = v1.AddToScheme(clientScheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// gloo resources
	err = glooinstancev1.AddToScheme(clientScheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return clientScheme
}
