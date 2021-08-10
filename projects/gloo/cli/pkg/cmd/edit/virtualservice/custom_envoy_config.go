package virtualservice

import (
	"fmt"

	errors "github.com/rotisserie/eris"
	editOptions "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/edit/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmdutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	ratelimitpb "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RateLimitCustomConfig(opts *editOptions.EditOptions, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {

	cmd := &cobra.Command{
		// Use command constants to aid with replacement.
		Use:   "client-config",
		Short: "Add rate-limits (Enterprise)",
		Long: `Configure rate-limits, which are composed of rate-limit actions that translate request characteristics to rate-limit descriptor tuples.
		For available actions and more information see: https://www.envoyproxy.io/docs/envoy/v1.9.0/api-v2/api/v2/route/route.proto#route-ratelimit-action
		
		This is a Gloo Enterprise feature.`,

		RunE: func(cmd *cobra.Command, args []string) error {
			return editVhost(opts)
		},
	}

	return cmd
}

func editVhost(opts *editOptions.EditOptions) error {

	vsClient := helpers.MustNamespacedVirtualServiceClient(opts.Top.Ctx, opts.Metadata.GetNamespace())
	vs, err := vsClient.Read(opts.Metadata.GetNamespace(), opts.Metadata.GetName(), clients.ReadOpts{})
	if err != nil {
		return errors.Wrapf(err, "Error reading virtual service")
	}

	if opts.ResourceVersion != "" {
		if vs.GetMetadata().GetResourceVersion() != opts.ResourceVersion {
			return fmt.Errorf("conflict - resource version does not match")
		}
	}

	ratelimitExtension := new(ratelimitpb.RateLimitVhostExtension)
	if rlExt := vs.GetVirtualHost().GetOptions().GetRatelimit(); rlExt != nil {
		ratelimitExtension = rlExt
	}

	var editor cmdutils.Editor
	ratelimitExtensionProto, err := editor.EditConfig(ratelimitExtension)
	if err != nil {
		return err
	}
	ratelimitExtension = ratelimitExtensionProto.(*ratelimitpb.RateLimitVhostExtension)
	if vs.GetVirtualHost().GetOptions() == nil {
		vs.GetVirtualHost().Options = &gloov1.VirtualHostOptions{}
	}

	vs.GetVirtualHost().GetOptions().RateLimitConfigType = &gloov1.VirtualHostOptions_Ratelimit{Ratelimit: ratelimitExtension}
	_, err = vsClient.Write(vs, clients.WriteOpts{OverwriteExisting: true})
	return err
}
