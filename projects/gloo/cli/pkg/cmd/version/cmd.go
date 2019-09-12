package version

import (
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
	"github.com/solo-io/go-utils/errors"
	"github.com/solo-io/go-utils/protoutils"
	"github.com/spf13/cobra"
	kubev1 "k8s.io/api/core/v1"
)

const (
	undefinedServer = "\nServer: version undefined, could not find any version of gloo running"
)

var (
	NoNamespaceAllError = errors.New("single namespace must be specified, cannot be namespace all for version command")
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.VERSION_COMMAND.Use,
		Aliases: constants.VERSION_COMMAND.Aliases,
		Short:   constants.VERSION_COMMAND.Short,
		Long:    constants.VERSION_COMMAND.Long,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.PersistentFlags().Changed(flagutils.OutputFlag) {
				opts.Top.Output = printers.JSON
			}
			if opts.Metadata.Namespace == "" {
				return NoNamespaceAllError
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			vrs, err := getVersion(NewKube(), opts)
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

func getVersion(sv ServerVersion, opts *options.Options) (*version.Version, error) {
	clientVersion, err := getClientVersion()
	if err != nil {
		return nil, err
	}
	serverVersion, err := sv.Get(opts)
	return &version.Version{
		Client: clientVersion,
		Server: serverVersion,
	}, nil
}

func getClientVersion() (*version.ClientVersion, error) {
	vrs := &version.ClientVersion{
		Version: linkedversion.Version,
	}
	return vrs, nil
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
		clientVersionStr := GetJson(vrs.GetClient())
		fmt.Printf("Client: \n%s\n", string(clientVersionStr))
		if vrs.GetServer() == nil {
			fmt.Println(undefinedServer)
			return nil
		}
		serverVersionStr := GetJson(vrs.GetServer())
		fmt.Printf("Server: \n%s\n", string(serverVersionStr))
	case printers.YAML:
		clientVersionStr := GetYaml(vrs.GetClient())
		fmt.Printf("Client: \n%s\n", string(clientVersionStr))
		if vrs.GetServer() == nil {
			fmt.Println(undefinedServer)
			return nil
		}
		serverVersionStr := GetYaml(vrs.GetServer())
		fmt.Printf("Server: \n%s\n", string(serverVersionStr))
	default:
		fmt.Printf("Client: version: %s\n", vrs.GetClient().Version)
		if vrs.GetServer() == nil {
			fmt.Println(undefinedServer)
			return nil
		}
		kubeSrvVrs := vrs.GetServer().GetKubernetes()
		if kubeSrvVrs == nil {
			fmt.Println(undefinedServer)
			return nil
		}
		table := tablewriter.NewWriter(os.Stdout)
		headers := []string{"Namespace", "Deployment-Type", "Containers"}
		content := []string{kubeSrvVrs.GetNamespace(), kubeSrvVrs.GetType().String()}
		var rows [][]string
		for i, container := range kubeSrvVrs.GetContainers() {
			name := fmt.Sprintf("%s: %s", container.GetName(), container.GetTag())
			if i == 0 {
				rows = append(rows, append(content, name))
			} else {
				rows = append(rows, []string{"", "", name})
			}
		}
		table.SetHeader(headers)
		table.AppendBulk(rows)
		table.SetAlignment(tablewriter.ALIGN_LEFT)
		fmt.Println("Server:")
		table.Render()
	}
	return nil
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
