package virtualservice

import (
	"fmt"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"

	"github.com/gogo/protobuf/types"
	"github.com/pkg/errors"
	editOptions "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/edit/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmdutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	ratelimitpb "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/plugins/ratelimit"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils"
	"github.com/solo-io/go-utils/protoutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RateLimitCustomConfig(opts *editOptions.EditOptions, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {

	cmd := &cobra.Command{
		// Use command constants to aid with replacement.
		Use:   "custom-envoy-config",
		Short: "Add a custom rate limit actions (Enterprise)",
		Long: `This allows using envoy actions to specify your rate limit descriptors.
		For available actions and more information see: https://www.envoyproxy.io/docs/envoy/v1.9.0/api-v2/api/v2/route/route.proto#route-ratelimit-action
		
		This is a Gloo Enterprise feature.`,

		RunE: func(cmd *cobra.Command, args []string) error {
			return editVhost(opts)
		},
	}

	return cmd
}

func editVhost(opts *editOptions.EditOptions) error {

	vsClient := helpers.MustVirtualServiceClient()
	vs, err := vsClient.Read(opts.Metadata.Namespace, opts.Metadata.Name, clients.ReadOpts{})
	if err != nil {
		return errors.Wrapf(err, "Error reading virtual service")
	}

	if opts.ResourceVersion != "" {
		if vs.Metadata.ResourceVersion != opts.ResourceVersion {
			return fmt.Errorf("conflict - resource version does not match")
		}
	}

	ratelimitExtension := new(ratelimitpb.RateLimitVhostExtension)
	err = utils.UnmarshalExtension(vs.VirtualHost.VirtualHostPlugins, constants.EnvoyRateLimitExtensionName, ratelimitExtension)
	if err != nil {
		if err != utils.NotFoundError {
			return err
		}
	}

	var editor cmdutils.Editor
	ratelimitExtensionProto, err := editor.EditConfig(ratelimitExtension)
	if err != nil {
		return err
	}
	ratelimitExtension = ratelimitExtensionProto.(*ratelimitpb.RateLimitVhostExtension)
	if vs.VirtualHost.VirtualHostPlugins == nil {
		vs.VirtualHost.VirtualHostPlugins = &gloov1.VirtualHostPlugins{}
	}

	if vs.VirtualHost.VirtualHostPlugins.Extensions == nil {
		vs.VirtualHost.VirtualHostPlugins.Extensions = &gloov1.Extensions{}
	}

	if vs.VirtualHost.VirtualHostPlugins.Extensions.Configs == nil {
		vs.VirtualHost.VirtualHostPlugins.Extensions.Configs = make(map[string]*types.Struct)
	}

	extStruct, err := protoutils.MarshalStruct(ratelimitExtension)
	if err != nil {
		return err
	}
	vs.VirtualHost.VirtualHostPlugins.Extensions.Configs[constants.EnvoyRateLimitExtensionName] = extStruct

	_, err = vsClient.Write(vs, clients.WriteOpts{OverwriteExisting: true})
	return err
}
