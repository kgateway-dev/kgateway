package ratelimit

import (
	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	editRouteOptions "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/edit/route/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmdutils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	ratelimitpb "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RateLimitCustomConfig(opts *editRouteOptions.RouteEditInput, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {

	cmd := &cobra.Command{
		// Use command constants to aid with replacement.
		Use:   "client-config",
		Short: "Add rate-limits (Enterprise)",
		Long: `Configure rate-limits, which are composed of rate-limit actions that translate request characteristics to rate-limit descriptor tuples.
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
		if rlExt := route.GetOptions().GetRatelimit(); rlExt != nil {
			ratelimitRouteExtension = rlExt
		}

		var editor cmdutils.Editor
		ratelimitRouteExtensionProto, err := editor.EditConfig(ratelimitRouteExtension)
		if err != nil {
			return err
		}
		ratelimitRouteExtension = ratelimitRouteExtensionProto.(*ratelimitpb.RateLimitRouteExtension)
		if route.GetOptions() == nil {
			route.Options = &gloov1.RouteOptions{}
		}

		route.GetOptions().RateLimitConfigType = &gloov1.RouteOptions_Ratelimit{Ratelimit: ratelimitRouteExtension}
		return nil
	})
}
