package create

import (
	"strings"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/prerun"

	envoyutil "github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/types"
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
	extauth "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/plugins/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/plugins/ratelimit"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/spf13/cobra"
)

var defaultDomains = []string{"*"}

var (
	ProvideNamespaceAndNameError = func(namespace, secretName string) error {
		return errors.Errorf("provide both a secret namespace [%v] and secret name [%v]", namespace, secretName)
	}
	UnableToMarshalApiKeyConfig = func(err error) error {
		return errors.Wrapf(err, "Error marshalling apikey config")
	}
	EmptyQueryError       = errors.Errorf("query must not be empty")
	InvlaidRefFormatError = errors.Errorf("invalid format: provide namespaced names for config maps (namespace.configMapName)")
)

func VSCreate(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {

	opts.Create.VirtualService.OIDCAuth.ClientSecretRef = new(core.ResourceRef)

	cmd := &cobra.Command{
		// Use command constants to aid with replacement.
		Use:     constants.VIRTUAL_SERVICE_COMMAND.Use,
		Aliases: constants.VIRTUAL_SERVICE_COMMAND.Aliases,
		Short:   "Create a Virtual Service",
		Long: "A virtual service describes the set of routes to match for a set of domains. \n" +
			"Virtual services are containers for routes assigned to a domain or set of domains. \n" +
			"Virtual services must not have overlapping domains, as the virtual service to match a request " +
			"is selected by the Host header (in HTTP1) or :authority header (in HTTP2). " +
			"When using Gloo Enterprise, virtual services can be configured with rate limiting, oauth, and apikey auth.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := prerun.CallParentPrerun(cmd, args); err != nil {
				return err
			}
			if err := prerun.EnableConsulClients(opts.Create.Consul); err != nil {
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
	flagutils.AddConsulConfigFlags(cmd.PersistentFlags(), &opts.Create.Consul)
	flagutils.AddVirtualServiceFlags(pflags, &opts.Create.VirtualService)
	cliutils.ApplyOptions(cmd, optionsFunc)

	return cmd
}

func createVirtualService(opts *options.Options, args []string) error {
	vs, err := virtualServiceFromOpts(opts.Metadata, opts.Create.VirtualService)
	if err != nil {
		return err
	}

	if !opts.Create.DryRun {
		virtualServiceClient := helpers.MustVirtualServiceClient()
		vs, err = virtualServiceClient.Write(vs, clients.WriteOpts{})
		if err != nil {
			return err
		}
	}

	printers.PrintVirtualServices(v1.VirtualServiceList{vs}, opts.Top.Output)

	return nil
}

// TODO: dedupe with Gloo
func virtualServiceFromOpts(meta core.Metadata, input options.InputVirtualService) (*v1.VirtualService, error) {
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
		if vs.VirtualHost.VirtualHostPlugins == nil {
			vs.VirtualHost.VirtualHostPlugins = &gloov1.VirtualHostPlugins{}
		}
		timeUnit, ok := ratelimit.RateLimit_Unit_value[rl.TimeUnit]
		if !ok {
			return nil, errors.Errorf("invalid time unit specified: %v", rl.TimeUnit)
		}
		ingressRateLimit := &ratelimit.IngressRateLimit{
			AnonymousLimits: &ratelimit.RateLimit{
				Unit:            ratelimit.RateLimit_Unit(timeUnit),
				RequestsPerUnit: rl.RequestsPerTimeUnit,
			},
		}
		ingressRateLimitStruct, err := envoyutil.MessageToStruct(ingressRateLimit)
		if err != nil {
			return nil, errors.Wrapf(err, "Error marshalling ingress rate limit")
		}
		if vs.VirtualHost.VirtualHostPlugins.Extensions == nil {
			vs.VirtualHost.VirtualHostPlugins.Extensions = new(gloov1.Extensions)
		}
		if vs.VirtualHost.VirtualHostPlugins.Extensions.Configs == nil {
			vs.VirtualHost.VirtualHostPlugins.Extensions.Configs = make(map[string]*types.Struct)
		}
		vs.VirtualHost.VirtualHostPlugins.Extensions.Configs[constants.RateLimitExtensionName] = ingressRateLimitStruct
	}

	return vs, authFromOpts(vs, input)
}

func authFromOpts(vs *v1.VirtualService, input options.InputVirtualService) error {

	var vhostAuth *extauth.VhostExtension

	oidc := input.OIDCAuth
	if oidc.Enable {
		if oidc.AppUrl == "" {
			return errors.Errorf("invalid app url specified: %v", oidc.AppUrl)
		}
		if oidc.IssuerUrl == "" {
			return errors.Errorf("invalid issuer url specified: %v", oidc.IssuerUrl)
		}
		if oidc.ClientId == "" {
			return errors.Errorf("invalid client id specified: %v", oidc.ClientId)
		}
		if oidc.CallbackPath == "" {
			return errors.Errorf("invalid callback path specified: %v", oidc.CallbackPath)
		}
		if oidc.ClientSecretRef.Name == "" || oidc.ClientSecretRef.Namespace == "" {
			return errors.Errorf("invalid client secret ref specified: %v.%v", oidc.ClientSecretRef.Namespace, oidc.ClientSecretRef.Name)
		}
		vhostAuth = &extauth.VhostExtension{
			Configs: []*extauth.VhostExtension_AuthConfig{{
				AuthConfig: &extauth.VhostExtension_AuthConfig_Oauth{
					Oauth: &extauth.OAuth{
						AppUrl:          oidc.AppUrl,
						CallbackPath:    oidc.CallbackPath,
						ClientId:        oidc.ClientId,
						ClientSecretRef: oidc.ClientSecretRef,
						IssuerUrl:       oidc.IssuerUrl,
						Scopes:          oidc.Scopes,
					},
				},
			}},
		}
	}

	apiKey := input.ApiKeyAuth
	if apiKey.Enable {
		var secretRefs []*core.ResourceRef
		if apiKey.SecretNamespace != "" && apiKey.SecretName != "" {
			secretRefs = []*core.ResourceRef{
				{
					Namespace: apiKey.SecretNamespace,
					Name:      apiKey.SecretName,
				},
			}
		} else if apiKey.SecretNamespace != "" || apiKey.SecretName != "" {
			return ProvideNamespaceAndNameError(apiKey.SecretNamespace, apiKey.SecretName)
		}

		var labels options.InputMapStringString
		labels.Entries = apiKey.Labels
		var labelSelector map[string]string
		if len(labels.MustMap()) > 0 {
			labelSelector = labels.MustMap()
		}

		vhostAuth = &extauth.VhostExtension{
			Configs: []*extauth.VhostExtension_AuthConfig{{
				AuthConfig: &extauth.VhostExtension_AuthConfig_ApiKeyAuth{
					ApiKeyAuth: &extauth.ApiKeyAuth{
						LabelSelector:    labelSelector,
						ApiKeySecretRefs: secretRefs,
					},
				},
			}},
		}
	}

	opaAuth := input.OpaAuth
	if opaAuth.Enable {

		var modules []*core.ResourceRef
		query := opaAuth.Query

		if len(query) == 0 {
			return EmptyQueryError
		}

		for _, moduleRef := range opaAuth.Modules {

			splits := strings.Split(moduleRef, ".")
			if len(splits) != 2 {
				return InvlaidRefFormatError
			}
			namespace := splits[0]
			name := splits[1]
			modules = append(modules, &core.ResourceRef{Name: name, Namespace: namespace})
		}

		if vhostAuth == nil {
			vhostAuth = &extauth.VhostExtension{}
		}
		cfg := &extauth.VhostExtension_AuthConfig{
			AuthConfig: &extauth.VhostExtension_AuthConfig_OpaAuth{
				OpaAuth: &extauth.OpaAuth{
					Modules: modules,
					Query:   query,
				},
			},
		}
		vhostAuth.Configs = append(vhostAuth.Configs, cfg)
	}

	if vhostAuth != nil {

		if vs.VirtualHost.VirtualHostPlugins == nil {
			vs.VirtualHost.VirtualHostPlugins = &gloov1.VirtualHostPlugins{}
		}

		vhostAuthStruct, err := envoyutil.MessageToStruct(vhostAuth)
		if err != nil {
			return errors.Wrapf(err, "Error marshalling oauth config")
		}
		if vs.VirtualHost.VirtualHostPlugins.Extensions == nil {
			vs.VirtualHost.VirtualHostPlugins.Extensions = new(gloov1.Extensions)
		}
		if vs.VirtualHost.VirtualHostPlugins.Extensions.Configs == nil {
			vs.VirtualHost.VirtualHostPlugins.Extensions.Configs = make(map[string]*types.Struct)
		}
		vs.VirtualHost.VirtualHostPlugins.Extensions.Configs[constants.ExtAuthExtensionName] = vhostAuthStruct

	}

	return nil
}
