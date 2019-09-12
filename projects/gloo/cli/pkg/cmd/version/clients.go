package version

import (
	"strings"

	"github.com/solo-io/gloo/install/helm/gloo/generate"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/version"
	"github.com/solo-io/go-utils/kubeutils"
	"github.com/solo-io/go-utils/stringutils"
	kubev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

//go:generate mockgen -destination ./mocks/mock_watcher.go -source clients.go

type ServerVersion interface {
	Get(opts *options.Options) ([]*version.ServerVersion, error)
}

type kube struct{}

var (
	KnativeUniqueContainers = []string{"knative-external-proxy", "knative-internal-proxy"}
	IngressUniqueContainers = []string{"ingress"}
	GlooEUniqueContainers   = []string{"gloo-ee"}
)

func NewKube() *kube {
	return &kube{}
}

func (k *kube) Get(opts *options.Options) ([]*version.ServerVersion, error) {
	cfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		// kubecfg is missing, therefore no cluster is present, only print client version
		return nil, nil
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	deployments, err := client.AppsV1().Deployments(opts.Metadata.Namespace).List(metav1.ListOptions{
		// search only for gloo deployments based on labels
		LabelSelector: "app=gloo",
	})
	if err != nil {
		return nil, err
	}

	var kubeContainerList []*version.Kubernetes_Container
	var foundGlooE, foundIngress, foundKnative bool
	for _, v := range deployments.Items {
		for _, container := range v.Spec.Template.Spec.Containers {
			containerInfo := parseContainerString(container)
			kubeContainerList = append(kubeContainerList, &version.Kubernetes_Container{
				Tag:      containerInfo.Tag,
				Name:     containerInfo.Repository,
				Registry: containerInfo.Registry,
			})
			switch {
			case stringutils.ContainsString(containerInfo.Repository, KnativeUniqueContainers):
				foundKnative = true
			case stringutils.ContainsString(containerInfo.Repository, IngressUniqueContainers):
				foundIngress = true
			case stringutils.ContainsString(containerInfo.Repository, GlooEUniqueContainers):
				foundGlooE = true
			}
		}
	}

	var deploymentType version.GlooType
	switch {
	case foundKnative:
		deploymentType = version.GlooType_Knative
	case foundIngress:
		deploymentType = version.GlooType_Knative
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
				Namespace:  opts.Metadata.Namespace,
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
	img.Tag = tag
	name = splitImageVersion[0]
	splitRepoName := strings.Split(name, "/")
	img.Repository = splitRepoName[len(splitRepoName)-1]
	img.Registry = strings.Join(splitRepoName[:len(splitRepoName)-1], "/")
	return img
}
