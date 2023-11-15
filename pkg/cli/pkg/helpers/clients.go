package helpers

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/solo-io/gloo/v2/pkg/cli/pkg/cmd/options/contextoptions"

	"github.com/solo-io/gloo/v2/pkg/listers"
	apiexts "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/k8s-utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
	"github.com/solo-io/solo-kit/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	clientset         *kubernetes.Clientset
	fakeKubeClientset *fake.Clientset
	memResourceClient *factory.MemoryResourceClientFactory
	consulClient      *factory.ConsulResourceClientFactory
	vaultClient       factory.ResourceClientFactory

	lock sync.Mutex
)

func MustKubeClient() kubernetes.Interface {
	return MustKubeClientWithKubecontext("")
}

func MustKubeClientWithKubecontext(kubecontext string) kubernetes.Interface {
	client, err := KubeClientWithKubecontext(kubecontext)
	if err != nil {
		log.Fatalf("failed to create kube client: %v", err)
	}
	return client
}

func KubeClient() (kubernetes.Interface, error) {
	return KubeClientWithKubecontext("")
}

func KubeClientWithKubecontext(kubecontext string) (kubernetes.Interface, error) {
	if fakeKubeClientset != nil {
		return fakeKubeClientset, nil
	}
	if clientset == nil {
		cfg, err := kubeutils.GetConfigWithContext("", os.Getenv("KUBECONFIG"), kubecontext)
		if err != nil {
			return nil, errors.Wrapf(err, "getting kube config")
		}
		client, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			return nil, errors.Wrapf(err, "creating clientset")
		}
		clientset = client
	}

	return clientset, nil
}

func MustGetNamespaces(ctx context.Context) []string {
	ns, err := GetNamespaces(ctx)
	if err != nil {
		log.Fatalf("failed to list namespaces")
	}
	return ns
}

// Note: requires RBAC permission to list namespaces at the cluster level
func GetNamespaces(ctx context.Context) ([]string, error) {
	kubecontext, err := contextoptions.KubecontextFrom(ctx)
	if err != nil {
		return nil, err
	}
	kubeClient, err := GetKubernetesClient(kubecontext)

	if err != nil {
		return nil, errors.Wrapf(err, "getting kube client")
	}
	var namespaces []string
	nsList, err := kubeClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, ns := range nsList.Items {
		namespaces = append(namespaces, ns.Name)
	}
	return namespaces, nil
}

type namespaceLister struct{}

var _ listers.NamespaceLister = namespaceLister{}

func NewNamespaceLister() listers.NamespaceLister {
	return namespaceLister{}
}

// this namespaceLister implementation requires all implementations to have a context input.
func (namespaceLister) List(ctx context.Context) ([]string, error) {
	return GetNamespaces(ctx)
}

type providedNamespaceLister struct {
	namespaces []string
}

func NewProvidedNamespaceLister(namespaces []string) listers.NamespaceLister {
	return providedNamespaceLister{namespaces: namespaces}
}

func (l providedNamespaceLister) List(ctx context.Context) ([]string, error) {
	return l.namespaces, nil
}

func GetKubernetesClient(kubecontext string) (kubernetes.Interface, error) {
	return GetKubernetesClientWithTimeout(0, kubecontext)
}

func GetKubernetesClientWithTimeout(timeout time.Duration, kubecontext string) (kubernetes.Interface, error) {
	if fakeKubeClientset != nil {
		return fakeKubeClientset, nil
	}
	config, err := getKubernetesConfig(timeout, kubecontext)
	if err != nil {
		return nil, err
	}
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return kubeClient, nil
}

func getKubernetesConfig(timeout time.Duration, kubecontext string) (*rest.Config, error) {
	config, err := kubeutils.GetConfigWithContext("", "", kubecontext)
	if err != nil {
		return nil, fmt.Errorf("Error retrieving Kubernetes configuration: %v \n", err)
	}
	config.Timeout = timeout
	// The burst & QPS values are set at a higher number to enable the triggering of up to this many
	// requests so that the KubeCoreCache that is created does not throttle as it get resources for
	// each watched namespace.
	config.QPS = 50
	config.Burst = 100
	return config, nil
}

func MustApiExtsClient() apiexts.Interface {
	client, err := ApiExtsClient()
	if err != nil {
		log.Fatalf("failed to create api exts client: %v", err)
	}
	return client
}

func ApiExtsClient() (apiexts.Interface, error) {
	cfg, err := kubeutils.GetConfig("", os.Getenv("KUBECONFIG"))
	if err != nil {
		return nil, errors.Wrapf(err, "getting kube config")
	}
	return apiexts.NewForConfig(cfg)
}
