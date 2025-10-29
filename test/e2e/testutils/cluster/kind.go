//go:build e2e

package cluster

import (
	"fmt"
	"os"

	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"

	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/kubectl"
)

// MustKindContext returns the Context for a KinD cluster with the given name
func MustKindContext(clusterName string) *Context {
	return MustKindContextWithScheme(clusterName, schemes.GatewayScheme())
}

// MustKindContextWithScheme returns the Context for a KinD cluster with the given name and scheme
func MustKindContextWithScheme(clusterName string, scheme *runtime.Scheme) *Context {
	if len(clusterName) == 0 {
		// We fall back to the cluster named `kind` if no cluster name was provided
		clusterName = "kind"
	}

	kubeCtx := os.Getenv(testutils.KubeCtx)
	if len(kubeCtx) == 0 {
		kubeCtx = fmt.Sprintf("kind-%s", clusterName)
	}

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
