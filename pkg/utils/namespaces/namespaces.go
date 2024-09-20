package namespaces

import (
	"context"
	"errors"
	"os"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/solo-kit/pkg/api/external/kubernetes/namespace"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"

	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s_kubernetes "k8s.io/client-go/kubernetes"
)

// AllNamespaces returns true if the list of namespaces watched is empty or contains only a blank string
// This implies that all namespaces are to be watched
func AllNamespaces(watchNamespaces []string) bool {
	if len(watchNamespaces) == 0 {
		return true
	}
	if len(watchNamespaces) == 1 && watchNamespaces[0] == "" {
		return true
	}
	return false
}

// ProcessWatchNamespaces appends the writeNamespace to the list of watchNamespaces passed if not already present and returns it
func ProcessWatchNamespaces(watchNamespaces []string, writeNamespace string) []string {
	if AllNamespaces(watchNamespaces) {
		return watchNamespaces
	}

	var writeNamespaceProvided bool
	for _, ns := range watchNamespaces {
		if ns == writeNamespace {
			writeNamespaceProvided = true
			break
		}
	}
	if !writeNamespaceProvided {
		watchNamespaces = append(watchNamespaces, writeNamespace)
	}

	return watchNamespaces
}

// GetPodNamespace returns the value of the env var `POD_NAMESPACE` and defaults to `gloo-system` if unset
func GetPodNamespace() string {
	if podNamespace := os.Getenv("POD_NAMESPACE"); podNamespace != "" {
		return podNamespace
	}
	return "gloo-system"
}

// NewKubeNamespaceClient creates and returns the `namespace.NewNamespaceClient` if it has permissions to list namespaces
// This entails creating a kubeclient, kubecorecache and calling a SelfSubjectAccessReview to verify if it has the appropriate RBAC
func NewKubeNamespaceClient(ctx context.Context) (kubernetes.KubeNamespaceClient, error) {
	kubeClient, err := helpers.KubeClientWithKubecontext("")
	if err != nil {
		return nil, err
	}

	clientset, ok := kubeClient.(*k8s_kubernetes.Clientset)
	if !ok {
		return nil, errors.New("unable to create kube client to list namespaces")
	}

	action := authv1.ResourceAttributes{
		Namespace: "",
		Verb:      "list",
		Resource:  "namespaces",
	}

	selfCheck := authv1.SelfSubjectAccessReview{
		Spec: authv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &action,
		},
	}

	resp, err := clientset.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, &selfCheck, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}

	if resp.Status.Allowed {
		kubeCache, err := cache.NewKubeCoreCache(ctx, kubeClient)
		if err != nil {
			return nil, err
		}

		return namespace.NewNamespaceClient(kubeClient, kubeCache), nil
	}
	return nil, errors.New("the caller does not have permissions to list namespaces")
}
