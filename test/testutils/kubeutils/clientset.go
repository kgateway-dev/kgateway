package kubeutils

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
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
