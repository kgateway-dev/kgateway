package kubeutils

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"k8s.io/client-go/rest"
)

// MustRestConfig returns MustRestConfigWithContext with an empty Kubernetes Context
func MustRestConfig() *rest.Config {
	ginkgo.GinkgoHelper()

	return MustRestConfigWithContext("")
}

// MustRestConfigWithContext returns the rest.Config for a Kubernetes Client,
// provided a Kubernetes context, or panics
func MustRestConfigWithContext(kubeContext string) *rest.Config {
	ginkgo.GinkgoHelper()

	restConfig, err := kubeutils.GetRestConfigWithKubeContext(kubeContext)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return restConfig
}
