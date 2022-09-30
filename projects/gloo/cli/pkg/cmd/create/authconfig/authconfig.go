package authconfig

import (
	"strings"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/prerun"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/argsutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/surveyutils"
	extauth "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	ProvideNamespaceAndNameError = func(namespace, secretName string) error {
		return errors.Errorf("provide both a secret namespace [%v] and secret name [%v]", namespace, secretName)
	}
	EmptyQueryError       = errors.Errorf("query must not be empty")
	InvalidRefFormatError = errors.Errorf("invalid format: provide namespaced names for config maps (namespace.configMapName)")
)

func AuthConfigCreate(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {

	opts.Create.AuthConfig.OIDCAuth.ClientSecretRef = new(core.ResourceRef)

	cmd := &cobra.Command{
		// Use command constants to aid with replacement.
		Use:     constants.AUTH_CONFIG_COMMAND.Use,
		Aliases: constants.AUTH_CONFIG_COMMAND.Aliases,
		Short:   "Create an Auth Config",
		Long: "When using Gloo Enterprise, the Gloo extauth server can be configured with numerous types of auth " +
			"schemes. This configuration lives on top-level AuthConfig resources, which can be referenced from your " +
			"virtual services. Virtual service auth settings can be overridden at the route or weighted destination " +
			"level. Auth schemes can be chained together and executed in order, e.g. oauth, apikey auth, and more.",
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
				if err := surveyutils.AddAuthConfigFlagsInteractive(&opts.Create.AuthConfig); err != nil {
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
			return createAuthConfig(opts, args)
		},
	}

	pflags := cmd.PersistentFlags()
	flagutils.AddMetadataFlags(pflags, &opts.Metadata)
	flagutils.AddAuthConfigFlags(pflags, &opts.Create.AuthConfig)
	cliutils.ApplyOptions(cmd, optionsFunc)

	return cmd
}

func createAuthConfig(opts *options.Options, args []string) error {
	ac, err := authConfigFromOpts(&opts.Metadata, opts.Create.AuthConfig)
	if err != nil {
		return err
	}

	if !opts.Create.DryRun {
		authConfigClient := helpers.MustNamespacedAuthConfigClient(opts.Top.Ctx, opts.Metadata.GetNamespace())
		ac, err = authConfigClient.Write(ac, clients.WriteOpts{})
		if err != nil {
			return err
		}
	}

	printers.PrintAuthConfigs(extauth.AuthConfigList{ac}, opts.Top.Output)

	return nil
}

func authConfigFromOpts(meta *core.Metadata, input options.InputAuthConfig) (*extauth.AuthConfig, error) {
	ac := &extauth.AuthConfig{
		Metadata: meta,
	}
	return ac, authFromOpts(ac, input)
}

func authFromOpts(ac *extauth.AuthConfig, input options.InputAuthConfig) error {
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
		if oidc.ClientSecretRef.GetName() == "" || oidc.ClientSecretRef.GetNamespace() == "" {
			return errors.Errorf("invalid client secret ref specified: %v.%v", oidc.ClientSecretRef.GetNamespace(), oidc.ClientSecretRef.GetName())
		}

		oauthConf := &extauth.AuthConfig_Config{
			AuthConfig: &extauth.AuthConfig_Config_Oauth{
				Oauth: &extauth.OAuth{
					AppUrl:                  oidc.AppUrl,
					CallbackPath:            oidc.CallbackPath,
					ClientId:                oidc.ClientId,
					ClientSecretRef:         oidc.ClientSecretRef,
					IssuerUrl:               oidc.IssuerUrl,
					AuthEndpointQueryParams: oidc.AuthEndpointQueryParams,
					Scopes:                  oidc.Scopes,
				},
			},
		}
		ac.Configs = append(ac.GetConfigs(), oauthConf)

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

		apiKeyAuthConfig := &extauth.AuthConfig_Config{
			AuthConfig: &extauth.AuthConfig_Config_ApiKeyAuth{
				ApiKeyAuth: &extauth.ApiKeyAuth{
					LabelSelector:    labelSelector,
					ApiKeySecretRefs: secretRefs,
					StorageBackend: &extauth.ApiKeyAuth_K8SSecretApikeyStorage{
						K8SSecretApikeyStorage: &extauth.K8SSecretApiKeyStorage{
							LabelSelector:    labelSelector,
							ApiKeySecretRefs: secretRefs,
						},
					},
				},
			},
		}
		ac.Configs = append(ac.GetConfigs(), apiKeyAuthConfig)
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
				return InvalidRefFormatError
			}
			namespace := splits[0]
			name := splits[1]
			modules = append(modules, &core.ResourceRef{Name: name, Namespace: namespace})
		}

		opaAuthConfig := &extauth.AuthConfig_Config{
			AuthConfig: &extauth.AuthConfig_Config_OpaAuth{
				OpaAuth: &extauth.OpaAuth{
					Modules: modules,
					Query:   query,
				},
			},
		}
		ac.Configs = append(ac.GetConfigs(), opaAuthConfig)
	}

	return nil
}
