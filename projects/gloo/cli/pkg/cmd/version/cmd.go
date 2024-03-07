package version

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/solo-io/go-utils/contextutils"

	"github.com/ghodss/yaml"
	"github.com/golang/protobuf/proto"
	"github.com/olekukonko/tablewriter"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils/protoutils"
	linkedversion "github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/version"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
	kube1vVersion "k8s.io/apimachinery/pkg/version"
	kubeYaml "sigs.k8s.io/yaml"
)

const (
	undefinedServer = "Server: version undefined, could not find any version of gloo running"
)

// VersionWrapper is a struct for version information
type VersionWrapper struct {
	GlooVersion       json.RawMessage     `json:"glooVersion,omitempty" yaml:"glooVersion,omitempty"`
	KubernetesVersion *kube1vVersion.Info `json:"kubernetesVersion,omitempty" yaml:"kubernetesVersion,omitempty"`
}

var (
	NoNamespaceAllError = eris.New("single namespace must be specified, cannot be namespace all for version command")
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	// Default output for version command is JSON
	versionOutput := printers.JSON

	cmd := &cobra.Command{
		Use:     constants.VERSION_COMMAND.Use,
		Aliases: constants.VERSION_COMMAND.Aliases,
		Short:   constants.VERSION_COMMAND.Short,
		Long:    constants.VERSION_COMMAND.Long,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			opts.Top.Output = versionOutput

			if opts.Metadata.GetNamespace() == "" {
				return NoNamespaceAllError
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return printVersion(NewKube(opts.Metadata.GetNamespace(), opts.Top.KubeContext), os.Stdout, opts)
		},
	}

	pflags := cmd.PersistentFlags()
	flagutils.AddOutputFlag(pflags, &versionOutput)
	flagutils.AddNamespaceFlag(pflags, &opts.Metadata.Namespace)

	return cmd
}

func GetClientServerVersions(ctx context.Context, sv ServerVersion) (*version.Version, *kube1vVersion.Info, error) {
	v := &version.Version{
		Client: getClientVersion(),
	}
	serverVersion, k8sServerVersion, err := sv.Get(ctx)
	if err != nil {
		return v, k8sServerVersion, err
	}
	v.Server = serverVersion
	return v, k8sServerVersion, nil
}

func getWrappedVersions(vrs *version.Version, k8sV *kube1vVersion.Info) (*VersionWrapper, error) {
	marshalledVrs, err := getJson(vrs)
	if err != nil {
		return nil, err
	}
	return &VersionWrapper{
		GlooVersion:       marshalledVrs,
		KubernetesVersion: k8sV,
	}, nil
}

func getClientVersion() *version.ClientVersion {
	return &version.ClientVersion{
		Version: linkedversion.Version,
	}
}

func printVersion(sv ServerVersion, w io.Writer, opts *options.Options) error {
	vrs, k8sV, _ := GetClientServerVersions(opts.Top.Ctx, sv)
	wrappedVersions, _ := getWrappedVersions(vrs, k8sV)
	// ignoring error so we still print client version even if we can't get server versions (e.g., not deployed, no rbac)
	switch opts.Top.Output {
	case printers.JSON:
		formattedVer, err := getFormattedJson(wrappedVersions) // GetJson(vrs)
		if err != nil {
			return err
		}
		if vrs.GetServer() == nil {
			fmt.Fprintf(w, "%s\n\n", undefinedServer)
		}
		fmt.Fprintf(w, "%s", string(formattedVer))
	case printers.YAML:
		formattedVer, err := getFormattedYaml(wrappedVersions) // GetJson(vrs)
		if err != nil {
			return err
		}
		if vrs.GetServer() == nil {
			fmt.Fprintf(w, "%s\n\n", undefinedServer)
		}
		fmt.Fprintf(w, "%s", string(formattedVer))
	default:
		fmt.Fprintf(w, "Client version: %s\n", vrs.GetClient().GetVersion())
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
		fmt.Fprintln(w, "Server version:")
		table.Render()

		if k8sV != nil {
			fmt.Fprintf(w, "Kubernetes version: %s\n", k8sV.GitVersion)
		}
	}
	return nil
}

func getDistributionName(name string, enterprise bool) string {
	if enterprise {
		return name + " Enterprise"
	}
	return name
}

func getJson(pb proto.Message) ([]byte, error) {
	data, err := protoutils.MarshalBytes(pb)
	if err != nil {
		contextutils.LoggerFrom(context.Background()).DPanic(err)
		return nil, err
	}
	return data, nil
}

func getYaml(pb proto.Message) ([]byte, error) {
	jsn, err := getJson(pb)
	if err != nil {
		contextutils.LoggerFrom(context.Background()).DPanic(err)
		return nil, err
	}
	data, err := yaml.JSONToYAML(jsn)
	if err != nil {
		contextutils.LoggerFrom(context.Background()).DPanic(err)
		return nil, err
	}
	return data, nil
}

func getFormattedJson(ver *VersionWrapper) ([]byte, error) {
	marshalled, err := json.MarshalIndent(&ver, "", "  ")
	if err != nil {
		contextutils.LoggerFrom(context.Background()).DPanic(err)
		return nil, err
	}
	return marshalled, nil
}

func getFormattedYaml(ver *VersionWrapper) ([]byte, error) {
	marshalled, err := kubeYaml.Marshal(&ver)
	if err != nil {
		contextutils.LoggerFrom(context.Background()).DPanic(err)
		return nil, err
	}
	return marshalled, nil
}
