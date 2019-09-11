package version

import (
	"strings"

	"github.com/solo-io/gloo/install/helm/gloo/generate"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/version"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/go-utils/kubeutils"
	"github.com/spf13/cobra"
	kubev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	linkedversion "k8s.io/helm/pkg/version"
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.VERSION_COMMAND.Use,
		Aliases: constants.VERSION_COMMAND.Aliases,
		Short:   constants.VERSION_COMMAND.Short,
		Long:    constants.VERSION_COMMAND.Long,
		RunE: func(cmd *cobra.Command, args []string) error {
			vrs, err := getVersion(cmd)
			if err != nil {
				return err
			}
			printVersion(opts, vrs)
			return nil
		},
	}

	pflags := cmd.PersistentFlags()
	flagutils.AddOutputFlag(pflags, &opts.Top.Output)

	return cmd
}

func getVersion(cmd *cobra.Command) (*version.Version, error) {
	clientVersion, err := getClientVersion()
	if err != nil {
		return nil, err
	}
	serverVersion, err := getServerVersion()
	if err != nil {
		return nil, err
	}
	return &version.Version{
		ClientVersion: clientVersion,
		ServerVersion: serverVersion,
	}, nil
}

func getClientVersion() (*version.ClientVersion, error) {
	vrs := &version.ClientVersion{
		Version: linkedversion.GetVersion(),
	}
	return vrs, nil
}

func getServerVersion() (*version.ServerVersion, error) {
	cfg, err := kubeutils.GetConfig("", "")
	if err != nil {
		// kubecfg is missing, therefore no cluster is present, only print client version
		return nil, nil
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	deployments, err := client.AppsV1().Deployments("").List(metav1.ListOptions{
		// search only for gloo deployments based on labels
		LabelSelector: "app=gloo",
	})
	if err != nil {
		return nil, err
	}

	var kubeContainerList []*version.Kubernetes_Container
	for _, v := range deployments.Items {
		for _, container := range v.Spec.Template.Spec.Containers {
			containerInfo := parseContainerString(container)
			kubeContainerList = append(kubeContainerList, &version.Kubernetes_Container{
				Tag:        containerInfo.Tag,
				Repository: containerInfo.Repository,
				Registry:   containerInfo.Registry,
			})
		}
	}
	serverVersion := &version.ServerVersion{
		VersionType: &version.ServerVersion_Kubernetes{
			Kubernetes: &version.Kubernetes{
				Containers: kubeContainerList,
			},
		},
	}
	return serverVersion, nil
}

func parseContainerString(container kubev1.Container) *generate.Image {
	img := &generate.Image{}
	splitImageVersion := strings.Split(container.Image, ":")
	name, tag := "", "latest"
	img.Tag = tag
	if len(splitImageVersion) == 2 {
		tag = splitImageVersion[1]
	}
	name = splitImageVersion[0]
	splitRepoName := strings.Split(name, "/")
	img.Repository = splitRepoName[len(splitRepoName)-1]
	img.Registry = strings.Join(splitRepoName[:len(splitRepoName)-1], "/")
	return img
}

func printVersion(opts *options.Options, vrs *version.Version) {
	switch opts.Top.Output {
	case printers.JSON:

	case printers.YAML:
	default:

	}
}
