package kubeutils

import (
	"fmt"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	"k8s.io/client-go/kubernetes"
)

// ClusterContext contains the metadata about a Kubernetes cluster
// It also includes useful utilities for interacting with that cluster
type ClusterContext struct {
	// The name of the Kubernetes cluster
	Name string

	// The context of the Kubernetes cluster
	KubeContext string

	// A CLI for interacting with Kubernetes cluster
	Cli *kubectl.Cli

	// A set of clients for interacting with the Kubernetes Cluster
	Clientset *kubernetes.Clientset
}

// MustKindClusterContext returns the ClusterContext for a KinD cluster with the given name
func MustKindClusterContext(clusterName string) *ClusterContext {
	kubeCtx := fmt.Sprintf("kind-%s", clusterName)

	return &ClusterContext{
		Name:        clusterName,
		KubeContext: kubeCtx,
		Cli:         kubectl.NewCli().WithKubeContext(kubeCtx),
		Clientset:   MustClientsetWithContext(kubeCtx),
	}
}
