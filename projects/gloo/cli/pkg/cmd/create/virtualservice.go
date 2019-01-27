package create

import (
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/argsutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/surveyutils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/spf13/cobra"
)

func VirtualService(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.VIRTUAL_SERVICE_COMMAND.Use,
		Aliases: constants.VIRTUAL_SERVICE_COMMAND.Aliases,
		Short:   "Create a Virtual Service",
		Long: "A virtual service describes the set of routes to match for a set of domains. \n" +
			"Virtual services are containers for routes assigned to a domain or set of domains. \n" +
			"Virtual services must not have overlapping domains, as the virtual service to match a request " +
			"is selected by the Host header (in HTTP1) or :authority header (in HTTP2). " +
			"The routes within a virtual service ",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if opts.Top.Interactive {
				if err := surveyutils.AddVirtualServiceFlagsInteractive(&opts.Create.VirtualService); err != nil {
					return err
				}
			}
			err := argsutils.MetadataArgsParse(opts, args)
			if err != nil {
				return err
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return createVirtualService(opts, args)
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddMetadataFlags(pflags, &opts.Metadata)
	flagutils.AddVirtualServiceFlags(pflags, &opts.Create.VirtualService)
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func createVirtualService(opts *options.Options, args []string) error {
	vs, err := virtualServiceFromOpts(opts.Metadata, opts.Create.VirtualService)
	if err != nil {
		return err
	}
	virtualServiceClient := helpers.MustVirtualServiceClient()
	vs, err = virtualServiceClient.Write(vs, clients.WriteOpts{})
	if err != nil {
		return err
	}

	helpers.PrintVirtualServices(v1.VirtualServiceList{vs}, opts.Top.Output)

	return nil
}

var allDomains = []string{"*"}

func virtualServiceFromOpts(meta core.Metadata, input options.InputVirtualService) (*v1.VirtualService, error) {
	if len(input.Domains) == 0 {
		input.Domains = allDomains
	}
	vs := &v1.VirtualService{
		Metadata: meta,
		VirtualHost: &gloov1.VirtualHost{
			Domains: input.Domains,
		},
	}

	return vs, nil
}
