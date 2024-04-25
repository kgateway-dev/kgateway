package cluster

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"os"
	"testing"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"

	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	kubetestclients "github.com/solo-io/gloo/test/kubernetes/testutils/clients"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MustKindContext returns the Context for a KinD cluster with the given name
func MustKindContext(t *testing.T, clusterName string) *Context {
	r := require.New(t)

	kubeCtx := fmt.Sprintf("kind-%s", clusterName)

	restCfg, err := kubeutils.GetRestConfigWithKubeContext(kubeCtx)
	r.NoError(err)

	clientset, err := kubernetes.NewForConfig(restCfg)
	r.NoError(err)

	clt, err := client.New(restCfg, client.Options{
		Scheme: kubetestclients.MustClientScheme(),
	})
	r.NoError(err)

	return &Context{
		Name:        clusterName,
		KubeContext: kubeCtx,
		RestConfig:  restCfg,
		Cli:         kubectl.NewCli().WithKubeContext(kubeCtx).WithReceiver(os.Stdout),
		Client:      clt,
		Clientset:   clientset,
	}
}
