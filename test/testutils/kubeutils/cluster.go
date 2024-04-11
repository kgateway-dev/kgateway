package kubeutils

import (
	"fmt"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterContext contains the metadata about a Kubernetes cluster
// It also includes useful utilities for interacting with that cluster
type ClusterContext struct {
	// The name of the Kubernetes cluster
	Name string

	// The context of the Kubernetes cluster
	KubeContext string

	// RestConfig holds the common attributes that can be passed to a Kubernetes client on initialization
	RestConfig *rest.Config

	// A CLI for interacting with Kubernetes cluster
	Cli *kubectl.Cli

	// A client to perform CRUD operations on the Kubernetes Cluster
	Client client.Client

	// A set of clients for interacting with the Kubernetes Cluster
	Clientset *kubernetes.Clientset
}

// MustKindClusterContext returns the ClusterContext for a KinD cluster with the given name
func MustKindClusterContext(clusterName string) *ClusterContext {
	ginkgo.GinkgoHelper()

	kubeCtx := fmt.Sprintf("kind-%s", clusterName)

	restCfg := MustRestConfigWithContext(kubeCtx)

	clientset, err := kubernetes.NewForConfig(restCfg)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	clt, err := client.New(restCfg, client.Options{
		Scheme: MustClientScheme(),
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return &ClusterContext{
		Name:        clusterName,
		KubeContext: kubeCtx,
		RestConfig:  restCfg,
		Cli:         kubectl.NewCli().WithKubeContext(kubeCtx).WithReceiver(ginkgo.GinkgoWriter),
		Client:      clt,
		Clientset:   clientset,
	}
}
