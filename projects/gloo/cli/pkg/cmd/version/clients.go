package version

import (
	"context"
	"strings"

	"github.com/solo-io/k8s-utils/kubeutils"
	kubev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

//go:generate mockgen -destination ./mocks/mock_watcher.go -source clients.go
type ServerVersion interface {
	Get(ctx context.Context) (*ServerVersionInfo, error)
}

type Container struct {
	Tag        string
	Repository string
	Registry   string
}

type ServerVersionInfo struct {
	Containers []Container
	Namespace  string
}

type kube struct {
	namespace   string
	kubeContext string
}

var (
	GlooEUniqueContainers = []string{"gloo-gateway"}
)

func NewKube(namespace, kubeContext string) *kube {
	return &kube{
		namespace:   namespace,
		kubeContext: kubeContext,
	}
}

func (k *kube) Get(ctx context.Context) (*ServerVersionInfo, error) {
	cfg, err := kubeutils.GetConfig("", "")
	if k.kubeContext != "" {
		cfg, err = kubeutils.GetConfigWithContext("", "", k.kubeContext)
	}

	if err != nil {
		// kubecfg is missing, therefore no cluster is present, only print client version
		return nil, nil
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	deployments, err := client.AppsV1().Deployments(k.namespace).List(ctx, metav1.ListOptions{
		// search only for gloo deployments based on labels
		LabelSelector: "gloo=gateway-v2",
	})
	if err != nil {
		return nil, err
	}

	var kubeContainerList []Container
	for _, v := range deployments.Items {
		for _, container := range v.Spec.Template.Spec.Containers {
			containerInfo := parseContainerString(container)
			kubeContainerList = append(kubeContainerList, Container{
				Tag:        containerInfo.Tag,
				Repository: containerInfo.Repository,
				Registry:   containerInfo.Registry,
			})

		}
	}

	if len(kubeContainerList) == 0 {
		return nil, nil
	}
	serverVersion := &ServerVersionInfo{
		Containers: kubeContainerList,
		Namespace:  k.namespace,
	}
	return serverVersion, nil
}

func parseContainerString(container kubev1.Container) *Container {
	img := &Container{}
	splitImageVersion := strings.Split(container.Image, ":")
	name, tag := "", "latest"
	if len(splitImageVersion) == 2 {
		tag = splitImageVersion[1]
	}
	img.Tag = tag
	name = splitImageVersion[0]
	splitRepoName := strings.Split(name, "/")
	registry := strings.Join(splitRepoName[:len(splitRepoName)-1], "/")
	img.Repository = splitRepoName[len(splitRepoName)-1]
	img.Registry = registry
	return img
}
