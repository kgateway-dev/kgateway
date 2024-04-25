package cluster

import (
	"fmt"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"os"

	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	kubetestclients "github.com/solo-io/gloo/test/kubernetes/testutils/clients"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MustKindContext returns the Context for a KinD cluster with the given name
func MustKindContext(clusterName string) *Context {
	kubeCtx := fmt.Sprintf("kind-%s", clusterName)

	restCfg, err := kubeutils.GetRestConfigWithKubeContext(kubeCtx)
	if err != nil {
		panic(err)
	}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		panic(err)
	}

	clt, err := client.New(restCfg, client.Options{
		Scheme: kubetestclients.MustClientScheme(),
	})
	if err != nil {
		panic(err)
	}

	return &Context{
		Name:        clusterName,
		KubeContext: kubeCtx,
		RestConfig:  restCfg,
		Cli:         kubectl.NewCli().WithKubeContext(kubeCtx).WithReceiver(os.Stdout),
		Client:      clt,
		Clientset:   clientset,
	}
}
