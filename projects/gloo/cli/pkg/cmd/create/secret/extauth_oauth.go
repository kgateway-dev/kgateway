package secret

import (
	"context"
	"fmt"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/argsutils"

	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/surveyutils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauth "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/spf13/cobra"
)

func ExtAuthOathCmd(opts *options.Options) *cobra.Command {
	meta := &opts.Metadata
	input := extauth.OauthSecret{}
	cmd := &cobra.Command{
		Use:   "oauth",
		Short: `Create an OAuth secret with the given name (Enterprise)`,
		Long: "Create an OAuth secret with the given name. The OAuth secret contains the client_secret as defined in [RFC 6749](https://tools.ietf.org/html/rfc6749). " +
			"This is an enterprise-only feature. The format of the secret data is: `{\"oauth\" : [client-secret string]}`. ",
		RunE: func(c *cobra.Command, args []string) error {
			err := argsutils.MetadataArgsParse(opts, args)
			if err != nil {
				return err
			}

			if opts.Top.Interactive {
				// and gather any missing args that are available through interactive mode
				if err := oauthSecretArgsInteractive(opts.Top.Ctx, meta, &input); err != nil {
					return err
				}
			}
			// create the secret
			if err := createOauthSecret(opts.Top.Ctx, meta, input, opts.Create.DryRun, opts.Top.Output); err != nil {
				return err
			}

			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&input.ClientSecret, "client-secret", "", "oauth client secret")

	return cmd
}

func oauthSecretArgsInteractive(ctx context.Context, meta *core.Metadata, input *extauth.OauthSecret) error {
	if err := surveyutils.InteractiveNamespace(ctx, &meta.Namespace); err != nil {
		return err
	}

	if err := cliutil.GetStringInput("Name of secret:", &meta.Name); err != nil {
		return err
	}

	if err := cliutil.GetStringInput("Enter Client Secret:", &input.ClientSecret); err != nil {
		return err
	}

	return nil
}

func createOauthSecret(ctx context.Context, meta *core.Metadata, input extauth.OauthSecret, dryRun bool, outputType printers.OutputType) error {
	if input.GetClientSecret() == "" {
		return fmt.Errorf("client-secret not provided")
	}

	secret := &gloov1.Secret{
		Metadata: meta,
		Kind: &gloov1.Secret_Oauth{
			Oauth: &input,
		},
	}

	if !dryRun {
		secretClient := helpers.MustSecretClient(ctx)
		var err error
		if secret, err = secretClient.Write(secret, clients.WriteOpts{Ctx: ctx}); err != nil {
			return err
		}
	}
	printers.PrintSecrets(gloov1.SecretList{secret}, outputType)

	return nil
}
