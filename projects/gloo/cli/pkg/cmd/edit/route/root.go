package route

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"

	editOptions "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/edit/options"
	editRouteOptions "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/edit/route/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/edit/route/ratelimit"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RootCmd(opts *editOptions.EditOptions, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	routeEditOpts := new(editRouteOptions.RouteEditInput)
	routeEditOpts.EditOptions = opts
	cmd := &cobra.Command{
		Use:     constants.ROUTE_COMMAND.Use,
		Aliases: constants.ROUTE_COMMAND.Aliases,
		Short:   constants.ROUTE_COMMAND.Short,
		Long:    constants.ROUTE_COMMAND.Long,
	}
	flagutils.AddOutputFlag(cmd.PersistentFlags(), &opts.Top.Output)
	cmd.PersistentFlags().Uint32VarP(&routeEditOpts.Index, "index", "x", 0, "edit the route with this index in the virtual service "+
		"route list")
	cmd.AddCommand(ExtAuthConfig(routeEditOpts))
	cmd.AddCommand(ratelimit.RateLimitConfig(routeEditOpts))
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
