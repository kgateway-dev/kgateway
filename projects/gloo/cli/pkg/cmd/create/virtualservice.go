package create

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/prerun"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	rltypes "github.com/solo-io/solo-apis/pkg/api/ratelimit.solo.io/v1alpha1"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/argsutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/surveyutils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/spf13/cobra"
)

var defaultDomains = []string{"*"}

func VSCreate(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {

	cmd := &cobra.Command{
		// Use command constants to aid with replacement.
		Use:     constants.VIRTUAL_SERVICE_COMMAND.Use,
		Aliases: constants.VIRTUAL_SERVICE_COMMAND.Aliases,
		Short:   "Create a Virtual Service",
		Long: "A virtual service describes the set of routes to match for a set of domains. \n" +
			"Virtual services are containers for routes assigned to a domain or set of domains. \n" +
			"Virtual services must not have overlapping domains, as the virtual service to match a request " +
			"is selected by the Host header (in HTTP1) or :authority header (in HTTP2). When using " +
			"Gloo Enterprise, virtual services can be configured with rate limiting, oauth, apikey auth, and more.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := prerun.CallParentPrerun(cmd, args); err != nil {
				return err
			}
			if err := prerun.EnableConsulClients(opts); err != nil {
				return err
			}
			return nil
		},
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
	vs, err := virtualServiceFromOpts(&opts.Metadata, opts.Create.VirtualService)
	if err != nil {
		return err
	}

	if !opts.Create.DryRun {
		virtualServiceClient := helpers.MustNamespacedVirtualServiceClient(opts.Top.Ctx, opts.Metadata.GetNamespace())
		vs, err = virtualServiceClient.Write(vs, clients.WriteOpts{})
		if err != nil {
			return err
		}
	}

	printers.PrintVirtualServices(opts.Top.Ctx, v1.VirtualServiceList{vs}, opts.Top.Output, opts.Metadata.GetNamespace())

	return nil
}

func virtualServiceFromOpts(meta *core.Metadata, input options.InputVirtualService) (*v1.VirtualService, error) {
	if len(input.Domains) == 0 {
		input.Domains = defaultDomains
	}
	displayName := meta.Name
	if input.DisplayName != "" {
		displayName = input.DisplayName
	}
	vs := &v1.VirtualService{
		Metadata: meta,
		VirtualHost: &gatewayv1.VirtualHost{
			Domains: input.Domains,
		},
		DisplayName: displayName,
	}
	rl := input.RateLimit
	if rl.Enable {
		if vs.GetVirtualHost().GetOptions() == nil {
			vs.GetVirtualHost().Options = &gloov1.VirtualHostOptions{}
		}
		timeUnit, ok := rltypes.RateLimit_Unit_value[rl.TimeUnit]
		if !ok {
			return nil, errors.Errorf("invalid time unit specified: %v", rl.TimeUnit)
		}
		ingressRateLimit := &ratelimit.IngressRateLimit{
			AnonymousLimits: &rltypes.RateLimit{
				Unit:            rltypes.RateLimit_Unit(timeUnit),
				RequestsPerUnit: rl.RequestsPerTimeUnit,
			},
		}
		vs.GetVirtualHost().GetOptions().RatelimitBasic = ingressRateLimit
	}

	return vs, authFromOpts(vs, input)
}

func authFromOpts(vs *v1.VirtualService, input options.InputVirtualService) error {
	if input.AuthConfig.Name == "" || input.AuthConfig.Namespace == "" {
		return nil
	}

	acRef := &core.ResourceRef{
		Name:      input.AuthConfig.Name,
		Namespace: input.AuthConfig.Namespace,
	}
	if vs.GetVirtualHost().GetOptions() == nil {
		vs.GetVirtualHost().Options = &gloov1.VirtualHostOptions{}
	}
	if vs.GetVirtualHost().GetOptions().GetExtauth() == nil {
		vs.GetVirtualHost().GetOptions().Extauth = &extauthv1.ExtAuthExtension{}
	}
	vs.GetVirtualHost().GetOptions().GetExtauth().Spec = &extauthv1.ExtAuthExtension_ConfigRef{ConfigRef: acRef}
	return nil
}
