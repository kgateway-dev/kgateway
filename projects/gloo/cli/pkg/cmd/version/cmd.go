package version

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/gogo/protobuf/proto"
	"github.com/olekukonko/tablewriter"
	"github.com/solo-io/gloo/install/helm/gloo/generate"
	linkedversion "github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/version"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/go-utils/kubeutils"
	"github.com/solo-io/go-utils/protoutils"
	"github.com/spf13/cobra"
	kubev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	undefinedServer = "\nServer: version undefined, could not find any version of gloo running"
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.VERSION_COMMAND.Use,
		Aliases: constants.VERSION_COMMAND.Aliases,
		Short:   constants.VERSION_COMMAND.Short,
		Long:    constants.VERSION_COMMAND.Long,
		RunE: func(cmd *cobra.Command, args []string) error {
			vrs, err := getVersion(cmd, opts)
			if err != nil {
				return err
			}
			return printVersion(opts, vrs)
		},
	}

	pflags := cmd.PersistentFlags()
	flagutils.AddOutputFlag(pflags, &opts.Top.Output)
	flagutils.AddNamespaceFlag(pflags, &opts.Metadata.Namespace)

	return cmd
}

func getVersion(cmd *cobra.Command, opts *options.Options) (*version.Version, error) {
	clientVersion, err := getClientVersion()
	if err != nil {
		return nil, err
	}
	serverVersion, err := getServerVersion(opts)
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
		Version: linkedversion.Version,
	}
	return vrs, nil
}

func getServerVersion(opts *options.Options) (*version.ServerVersion, error) {
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
	for _, v := range deployments.Items {
		for _, container := range v.Spec.Template.Spec.Containers {
			containerInfo := parseContainerString(container)
			kubeContainerList = append(kubeContainerList, &version.Kubernetes_Container{
				Tag:      containerInfo.Tag,
				Name:     containerInfo.Repository,
				Registry: containerInfo.Registry,
			})
		}
	}
	if len(kubeContainerList) == 0 {
		return nil, nil
	}
	serverVersion := &version.ServerVersion{
		VersionType: &version.ServerVersion_Kubernetes{
			Kubernetes: &version.Kubernetes{
				Containers: kubeContainerList,
				Namespace:  opts.Metadata.Namespace,
			},
		},
	}
	return serverVersion, nil
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

func printVersion(opts *options.Options, vrs *version.Version) error {
	switch opts.Top.Output {
	case printers.JSON:
		clientVersionStr := GetJson(vrs.GetClientVersion())
		fmt.Printf("Client: \n%s\n", string(clientVersionStr))
		if vrs.GetServerVersion() == nil {
			fmt.Println(undefinedServer)
			return nil
		}
		serverVersionStr := GetJson(vrs.GetServerVersion())
		fmt.Printf("Server: \n%s\n", string(serverVersionStr))
	case printers.JSON_FMT:
		clientVersionStr := GetJson(vrs.GetClientVersion())
		fmt.Printf("Client: \n%s\n", FormatJson(clientVersionStr))
		if vrs.GetServerVersion() == nil {
			fmt.Println(undefinedServer)
			return nil
		}
		serverVersionStr := GetJson(vrs.GetServerVersion())
		fmt.Printf("Server: \n%s\n", string(FormatJson(serverVersionStr)))
	case printers.YAML:
		clientVersionStr := GetYaml(vrs.GetClientVersion())
		fmt.Printf("Client: \n%s\n", string(clientVersionStr))
		if vrs.GetServerVersion() == nil {
			fmt.Println(undefinedServer)
			return nil
		}
		serverVersionStr := GetYaml(vrs.GetServerVersion())
		fmt.Printf("Server: \n%s\n", string(serverVersionStr))
	default:
		fmt.Printf("Client: version: %s\n", vrs.GetClientVersion().Version)
		if vrs.GetServerVersion() == nil {
			fmt.Println(undefinedServer)
			return nil
		}
		kubeSrvVrs := vrs.GetServerVersion().GetKubernetes()
		if kubeSrvVrs == nil {
			fmt.Println(undefinedServer)
			return nil
		}
		table := tablewriter.NewWriter(os.Stdout)
		headers, content := []string{"Namespace"}, []string{kubeSrvVrs.GetNamespace()}
		for _, container := range kubeSrvVrs.GetContainers() {
			headers = append(headers, container.GetName())
			content = append(content,  container.GetTag())
		}
		table.SetHeader(headers)
		table.Append(content)
		table.SetAlignment(tablewriter.ALIGN_LEFT)
		fmt.Println("Server:")
		table.Render()
	}
	return nil
}

func FormatJson(byt []byte) string {
	var out bytes.Buffer
	if err := json.Indent(&out, byt, "", "\t"); err != nil {
		panic(err)
	}
	return out.String()
}

func GetJson(pb proto.Message) []byte {
	data, err := protoutils.MarshalBytes(pb)
	if err != nil {
		panic(err)
	}
	return data
}

func GetYaml(pb proto.Message) []byte {
	jsn := GetJson(pb)
	data, err := yaml.JSONToYAML(jsn)
	if err != nil {
		panic(err)
	}
	return data
}
