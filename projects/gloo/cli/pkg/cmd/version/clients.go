package version

import (
	"context"
	"strings"

	"github.com/solo-io/gloo/install/helm/gloo/generate"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/version"
	"github.com/solo-io/go-utils/stringutils"
	"github.com/solo-io/k8s-utils/kubeutils"
	kubev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

//go:generate mockgen -destination ./mocks/mock_watcher.go -source clients.go

type ServerVersion interface {
	Get(ctx context.Context) ([]*version.ServerVersion, error)
}

type kube struct {
	namespace   string
	kubeContext string
}

var (
	KnativeUniqueContainers = []string{"knative-external-proxy", "knative-internal-proxy"}
	IngressUniqueContainers = []string{"ingress"}
	GlooEUniqueContainers   = []string{"gloo-ee"}
	ossImageAnnotation      = "gloo.solo.io/oss-image-tag"
)

// NewKube creates a new kube client for our cli
// It knows how to see its namespace and potentially its context
// Mainly used to retrieve server versions of gloo owned deployments
func NewKube(namespace, kubeContext string) *kube {
	return &kube{
		namespace:   namespace,
		kubeContext: kubeContext,
	}
}

func (k *kube) Get(ctx context.Context) ([]*version.ServerVersion, error) {
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
		LabelSelector: "app=gloo",
	})
	if err != nil {
		return nil, err
	}

	var kubeContainerList []*version.Kubernetes_Container
	var foundGlooE, foundIngress, foundKnative bool
	for _, v := range deployments.Items {
		ossTag := v.Spec.Template.GetAnnotations()[ossImageAnnotation]
		for _, container := range v.Spec.Template.Spec.Containers {
			containerInfo := parseContainerString(container)
			kubeContainerList = append(kubeContainerList, &version.Kubernetes_Container{
				Tag:      *containerInfo.Tag,
				Name:     *containerInfo.Repository,
				Registry: *containerInfo.Registry,
				OssTag:   ossTag,
			})
			switch {
			case stringutils.ContainsString(*containerInfo.Repository, KnativeUniqueContainers):
				foundKnative = true
			case stringutils.ContainsString(*containerInfo.Repository, IngressUniqueContainers):
				foundIngress = true
			case stringutils.ContainsString(*containerInfo.Repository, GlooEUniqueContainers):
				foundGlooE = true
			}
		}
	}

	var deploymentType version.GlooType
	switch {
	case foundKnative:
		deploymentType = version.GlooType_Knative
	case foundIngress:
		deploymentType = version.GlooType_Ingress
	default:
		deploymentType = version.GlooType_Gateway
	}

	if len(kubeContainerList) == 0 {
		return nil, nil
	}
	serverVersion := &version.ServerVersion{
		Type:       deploymentType,
		Enterprise: foundGlooE,
		VersionType: &version.ServerVersion_Kubernetes{
			Kubernetes: &version.Kubernetes{
				Containers: kubeContainerList,
				Namespace:  k.namespace,
			},
		},
	}
	return []*version.ServerVersion{serverVersion}, nil
}

func parseContainerString(container kubev1.Container) *generate.Image {
	img := &generate.Image{}
	splitImageVersion := strings.Split(container.Image, ":")
	name, tag := "", "latest"
	if len(splitImageVersion) == 2 {
		tag = splitImageVersion[1]
	}
	img.Tag = &tag
	name = splitImageVersion[0]
	splitRepoName := strings.Split(name, "/")
	registry := strings.Join(splitRepoName[:len(splitRepoName)-1], "/")
	img.Repository = &splitRepoName[len(splitRepoName)-1]
	img.Registry = &registry
	return img
}
