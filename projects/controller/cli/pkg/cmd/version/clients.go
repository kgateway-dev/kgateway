package version

import (
	"context"
	"strings"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"

	"github.com/solo-io/gloo/install/helm/gloo/generate"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/version"
	"github.com/solo-io/go-utils/stringutils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

//go:generate mockgen -destination ./mocks/mock_watcher.go -source clients.go

type ServerVersion interface {
	Get(ctx context.Context) ([]*version.ServerVersion, error)
	GetClusterVersion() (*version.KubernetesClusterVersion, error)
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
	cfg, err := kubeutils.GetRestConfigWithKubeContext(k.kubeContext)
	if err != nil {
		// kubecfg is missing, therefore no cluster is present, only print client version
		return nil, err
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

func (k *kube) GetClusterVersion() (*version.KubernetesClusterVersion, error) {
	cfg, err := kubeutils.GetRestConfigWithKubeContext(k.kubeContext)
	if err != nil {
		// kubecfg is missing, therefore no cluster is present, only print client version
		return nil, err
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	k8ServerVersion, err := client.ServerVersion()
	if err != nil {
		return nil, err
	}
	clusterVersion := &version.KubernetesClusterVersion{
		Minor:      k8ServerVersion.Minor,
		Major:      k8ServerVersion.Major,
		GitVersion: k8ServerVersion.GitVersion,
		BuildDate:  k8ServerVersion.BuildDate,
		Platform:   k8ServerVersion.Platform,
	}
	return clusterVersion, nil
}

// this func parses container image string when the following formats are used
// - repo
// - repo:tag
// - repo@sha256:digest
// - repo:tag@sha256:digest
// - registry/repo
// - registry/repo:tag
// - registry/repo@sha256:digest
// - registry/repo:tag@sha256:digest
// - also variants when registry is in the form <registry:port>
func parseContainerString(container corev1.Container) *generate.Image {
	img := &generate.Image{}
	splitImageVersion := delimitContainerImageStr(container.Image)

	name := splitImageVersion[0]
	tag, digest := "latest", ""
	if len(splitImageVersion) == 2 {
		if strings.HasSuffix(splitImageVersion[0], "@sha256") { // handle <image>@sha256:<digest>
			strs := strings.Split(splitImageVersion[0], "@")
			if len(strs) == 2 {
				name = strs[0]
			}
			digest = splitImageVersion[1]
		} else {
			tag = splitImageVersion[1]
		}
	} else if len(splitImageVersion) >= 3 && strings.HasSuffix(splitImageVersion[1], "@sha256") { // handle <image>:<tag>@sha256:<digest>
		strs := strings.Split(splitImageVersion[1], "@")
		if len(strs) == 2 {
			tag = strs[0]
		}
		digest = splitImageVersion[2]
	}
	img.Tag = &tag
	img.Digest = &digest
	splitRepoName := strings.Split(name, "/")
	registry := strings.Join(splitRepoName[:len(splitRepoName)-1], "/")
	img.Repository = &splitRepoName[len(splitRepoName)-1]
	img.Registry = &registry
	return img
}

// this func delimits container image by ":"
// - if the image format is of the form <registry:port/repo> then <registry:port> will be treated as the registry
// - all other formats are treated normal when delimited by ":"
func delimitContainerImageStr(str string) []string {
	arr := strings.Split(str, ":")
	// check for special case of image string following <registry:port/repo>
	if len(arr) >= 2 && strings.Index(str, "/") > strings.Index(str, ":") {
		// combine the <registry:port> as the registry, we copy into a new array and shift the rest of the
		// elements.
		var copyArr []string
		copyArr = append(copyArr, arr[0]+":"+arr[1])
		if len(arr) > 2 {
			copyArr = append(copyArr, arr[2:]...)
		}
		arr = copyArr
	}
	return arr
}
