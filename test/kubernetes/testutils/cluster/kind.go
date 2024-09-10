package cluster

import (
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	kubetestclients "github.com/solo-io/gloo/test/kubernetes/testutils/clients"
)

// MustKindContext returns the Context for a KinD cluster with the given name
func MustKindContext(clusterName string) *Context {
	return MustKindContextWithScheme(clusterName, kubetestclients.MustClientScheme())
}

// MustKindContextWithScheme returns the Context for a KinD cluster with the given name and scheme
func MustKindContextWithScheme(clusterName string, scheme *runtime.Scheme) *Context {
	if len(clusterName) == 0 {
		// We fall back to the cluster named `kind` if no cluster name was provided
		clusterName = "kind"
	}

	kubeCtx := fmt.Sprintf("kind-%s", clusterName)

	restCfg, err := kubeutils.GetRestConfigWithKubeContext(kubeCtx)
	if err != nil {
		panic(err)
	}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		panic(err)
	}

	// This line prevents controller-runtime from complaining about log.SetLogger never being called
	log.SetLogger(zap.New(zap.WriteTo(os.Stdout), zap.UseDevMode(true)))
	clt, err := client.New(restCfg, client.Options{
		Scheme: scheme,
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
