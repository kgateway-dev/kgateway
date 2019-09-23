package version

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/gogo/protobuf/proto"
	"github.com/olekukonko/tablewriter"
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
)

const (
	undefinedServer = "Server: version undefined, could not find any version of gloo running"
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
			return printVersion(NewKube(), os.Stdout, opts)
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
	if err != nil {
		return nil, err
	}
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

func printVersion(sv ServerVersion, w io.Writer, opts *options.Options) error {
	vrs, err := getVersion(sv, opts)
	if err != nil {
		return err
	}
	switch opts.Top.Output {
	case printers.JSON:
		clientVersionStr := string(GetJson(vrs.GetClient()))
		clientVersionStr = strings.ReplaceAll(clientVersionStr, "\n", "")
		fmt.Fprintf(w, "Client: %s\n", clientVersionStr)
		if vrs.GetServer() == nil {
			fmt.Fprintln(w, undefinedServer)
			return nil
		}
		fmt.Fprint(w, "Server: ")
		for _, v := range vrs.GetServer() {
			serverVersionStr := GetJson(v)
			fmt.Fprintf(w, "%s\n", string(serverVersionStr))
		}
	case printers.YAML:
		clientVersionStr := string(GetYaml(vrs.GetClient()))
		clientVersionStr = strings.ReplaceAll(clientVersionStr, "\n", "")
		fmt.Fprintf(w, "Client: %s\n", clientVersionStr)
		if vrs.GetServer() == nil {
			fmt.Fprintln(w, undefinedServer)
			return nil
		}
		fmt.Fprintln(w, "Server:")
		for _, v := range vrs.GetServer() {
			serverVersionStr := string(GetYaml(v))
			clientVersionStr = strings.TrimRight(clientVersionStr, "\n")
			fmt.Fprintf(w, "%s\n", serverVersionStr)
		}
	default:
		fmt.Fprintf(w, "Client: version: %s\n", vrs.GetClient().Version)
		if vrs.GetServer() == nil {
			fmt.Fprintln(w, undefinedServer)
			return nil
		}
		srv := vrs.GetServer()
		if srv == nil {
			fmt.Println(undefinedServer)
			return nil
		}

		table := tablewriter.NewWriter(w)
		headers := []string{"Namespace", "Deployment-Type", "Containers"}
		var rows [][]string
		for _, v := range srv {
			kubeSrvVrs := v.GetKubernetes()
			if kubeSrvVrs == nil {
				continue
			}
			content := []string{kubeSrvVrs.GetNamespace(), getDistributionName(v.GetType().String(), v.GetEnterprise())}
			for i, container := range kubeSrvVrs.GetContainers() {
				name := fmt.Sprintf("%s: %s", container.GetName(), container.GetTag())
				if i == 0 {
					rows = append(rows, append(content, name))
				} else {
					rows = append(rows, []string{"", "", name})
				}
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

func getDistributionName(name string, enterprise bool) string {
	if enterprise {
		return name + " Enterprise"
	}
	return name
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
