package cluster

import (
	"fmt"

	"github.com/onsi/ginkgo/v2"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"

	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	kubetestclients "github.com/solo-io/gloo/test/kubernetes/testutils/clients"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MustKindContext returns the Context for a KinD cluster with the given name
func MustKindContext(clusterName string) *Context {
	ginkgo.GinkgoHelper()

	kubeCtx := fmt.Sprintf("kind-%s", clusterName)

	restCfg, err := kubeutils.GetRestConfigWithKubeContext(kubeCtx)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	clientset, err := kubernetes.NewForConfig(restCfg)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	clt, err := client.New(restCfg, client.Options{
		Scheme: kubetestclients.MustClientScheme(),
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return &Context{
		Name:        clusterName,
		KubeContext: kubeCtx,
		RestConfig:  restCfg,
		Cli:         kubectl.NewCli().WithKubeContext(kubeCtx).WithReceiver(ginkgo.GinkgoWriter),
		Client:      clt,
		Clientset:   clientset,
	}
}
