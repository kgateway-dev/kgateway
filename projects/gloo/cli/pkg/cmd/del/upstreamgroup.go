package del

import (
	"fmt"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/common"
	"github.com/solo-io/go-utils/cliutils"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/spf13/cobra"
)

func UpstreamGroup(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.UPSTREAM_GROUP_COMMAND.Use,
		Aliases: constants.UPSTREAM_GROUP_COMMAND.Aliases,
		Short:   "delete an upstream group",
		Long:    "usage: glooctl delete upstreamgroup [NAME] [--namespace=namespace]",
		RunE: func(cmd *cobra.Command, args []string) error {
			name := common.GetName(args, opts)
			if err := helpers.MustNamespacedUpstreamGroupClient(opts.Top.Ctx, opts.Metadata.GetNamespace()).Delete(opts.Metadata.GetNamespace(), name,
				clients.DeleteOpts{Ctx: opts.Top.Ctx}); err != nil {
				return err
			}
			fmt.Printf("upstreamgroup %v deleted", name)
			return nil
		},
	}
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
