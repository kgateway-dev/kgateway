package ratelimit

import (
	"github.com/gogo/protobuf/types"
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	editRouteOptions "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/edit/route/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmdutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	ratelimitpb "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/plugins/ratelimit"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils"
	"github.com/solo-io/go-utils/protoutils"

	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RateLimitCustomConfig(opts *editRouteOptions.RouteEditInput, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {

	cmd := &cobra.Command{
		// Use command constants to aid with replacement.
		Use:   "custom-envoy-config",
		Short: "Add a custom rate limit actions (Enterprise)",
		Long: `This allows using envoy actions to specify your rate limit descriptors.
		For available actions and more information see: https://www.envoyproxy.io/docs/envoy/v1.9.0/api-v2/api/v2/route/route.proto#route-ratelimit-action
		
		This is a Gloo Enterprise feature.`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return editRouteOptions.EditRoutePreRunE(opts)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return editRoute(opts)
		},
	}

	return cmd
}

func editRoute(opts *editRouteOptions.RouteEditInput) error {
	return editRouteOptions.UpdateRoute(opts, func(route *gatewayv1.Route) error {
		ratelimitRouteExtension := new(ratelimitpb.RateLimitRouteExtension)
		err := utils.UnmarshalExtension(route.RoutePlugins, constants.EnvoyRateLimitExtensionName, ratelimitRouteExtension)
		if err != nil {
			if err != utils.NotFoundError {
				return err
			}
		}

		var editor cmdutils.Editor
		ratelimitRouteExtensionProto, err := editor.EditConfig(ratelimitRouteExtension)
		if err != nil {
			return err
		}
		ratelimitRouteExtension = ratelimitRouteExtensionProto.(*ratelimitpb.RateLimitRouteExtension)
		if route.RoutePlugins == nil {
			route.RoutePlugins = &gloov1.RoutePlugins{}
		}

		if route.RoutePlugins.Extensions == nil {
			route.RoutePlugins.Extensions = &gloov1.Extensions{}
		}

		if route.RoutePlugins.Extensions.Configs == nil {
			route.RoutePlugins.Extensions.Configs = make(map[string]*types.Struct)
		}

		extStruct, err := protoutils.MarshalStruct(ratelimitRouteExtension)
		if err != nil {
			return err
		}
		route.RoutePlugins.Extensions.Configs[constants.EnvoyRateLimitExtensionName] = extStruct
		return nil
	})
}
