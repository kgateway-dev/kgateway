package route

import (
	"github.com/solo-io/gloo/pkg/cliutil"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	editRouteOptions "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/edit/route/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauthpb "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type authEditInput struct {
	RouteEditInput *editRouteOptions.RouteEditInput
	Disable        bool
}

func editRouteFlags(set *pflag.FlagSet, route *authEditInput) {
	set.BoolVarP(&route.Disable, "disable", "d", false, "set to true to disable authentication on this route")
}

func ExtAuthConfig(opts *editRouteOptions.RouteEditInput, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {

	input := &authEditInput{RouteEditInput: opts}

	cmd := &cobra.Command{
		// Use command constants to aid with replacement.
		Use:     constants.CONFIG_EXTAUTH_COMMAND.Use,
		Aliases: constants.CONFIG_EXTAUTH_COMMAND.Aliases,
		Short:   "Configure disable external auth on a route (Enterprise)",
		Long:    "Allows disabling external auth on specific routes. External auth is a gloo enterprise feature.",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if err := editRouteOptions.EditRoutePreRunE(opts); err != nil {
				return err
			}
			if opts.Top.Interactive {
				cliutil.ChooseBool("Disable auth on this route?", &input.Disable)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return editRoute(opts, input, args)
		},
	}

	pflags := cmd.PersistentFlags()
	flagutils.AddMetadataFlags(pflags, &opts.Metadata)

	editRouteFlags(cmd.Flags(), input)
	cliutils.ApplyOptions(cmd, optionsFunc)

	return cmd
}

func editRoute(opts *editRouteOptions.RouteEditInput, input *authEditInput, args []string) error {
	return editRouteOptions.UpdateRoute(opts, func(route *gatewayv1.Route) error {
		if route.GetOptions() == nil {
			route.Options = &gloov1.RouteOptions{}
		}
		if route.GetOptions().GetExtauth() == nil {
			route.GetOptions().Extauth = &extauthpb.ExtAuthExtension{}
		}
		switch spec := route.GetOptions().GetExtauth().GetSpec().(type) {
		case *extauthpb.ExtAuthExtension_Disable:
			route.GetOptions().GetExtauth().Spec = spec
		default:
			if input.Disable {
				route.GetOptions().GetExtauth().Spec = &extauthpb.ExtAuthExtension_Disable{Disable: true}
			}
		}
		return nil
	})
}
