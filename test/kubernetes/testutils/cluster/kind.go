package cluster

import (
	"fmt"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	"github.com/solo-io/gloo/test/kubernetes/testutils/clients"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MustKindContext returns the Context for a KinD cluster with the given name
func MustKindContext(testing testing.TB, clusterName string) *Context {
	testing.Helper()

	kubeCtx := fmt.Sprintf("kind-%s", clusterName)

	restCfg := clients.MustRestConfigWithContext(kubeCtx)

	clientset, err := kubernetes.NewForConfig(restCfg)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	clt, err := client.New(restCfg, client.Options{
		Scheme: clients.MustClientScheme(),
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
