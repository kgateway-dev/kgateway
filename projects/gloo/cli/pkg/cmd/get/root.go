package get

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/prerun"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var EmptyGetError = eris.New("please provide a subcommand")
var UnsetNamespaceError = eris.New("Gloo installation namespace does not exist. Did you install it in another namespace and forgot to add the '-n NAMESPACE' flag?")

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GET_COMMAND.Use,
		Aliases: constants.GET_COMMAND.Aliases,
		Short:   constants.GET_COMMAND.Short,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := prerun.CallParentPrerun(cmd, args); err != nil {
				return err
			}
			if err := prerun.EnableConsulClients(opts); err != nil {
				return err
			}
			if !opts.Top.Consul.UseConsul {
				client := helpers.MustKubeClientWithKubecontext(opts.Top.KubeContext)
				_, err := client.CoreV1().Namespaces().Get(opts.Top.Ctx, opts.Metadata.GetNamespace(), metav1.GetOptions{})
				if err != nil {
					if apierrors.IsNotFound(err) {
						return UnsetNamespaceError
					}
					// we would still like to attempt the command even if we don't have rbac to list namespaces, so just log a warning
					contextutils.LoggerFrom(context.TODO()).Warnf("unable to locate gloo installation namespace", err)
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return EmptyGetError
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddMetadataFlags(pflags, &opts.Metadata)
	flagutils.AddOutputFlag(pflags, &opts.Top.Output)

	cmd.AddCommand(VirtualService(opts))
	cmd.AddCommand(RouteTable(opts))
	cmd.AddCommand(Proxy(opts))
	cmd.AddCommand(Upstream(opts))
	cmd.AddCommand(UpstreamGroup(opts))
	cmd.AddCommand(AuthConfig(opts))
	cmd.AddCommand(RateLimitConfig(opts))
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
