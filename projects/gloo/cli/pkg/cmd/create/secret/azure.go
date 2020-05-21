package secret

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"

	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/argsutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/spf13/cobra"
)

func azureCmd(opts *options.Options) *cobra.Command {
	input := &opts.Create.InputSecret.AzureSecret
	cmd := &cobra.Command{
		Use:   "azure",
		Short: `Create an Azure secret with the given name`,
		Long: "Create an Azure secret with the given name. The format of the secret data is: " +
			"`{\"azure\" : [api-keys]}`",
		RunE: func(c *cobra.Command, args []string) error {
			if err := argsutils.MetadataArgsParse(opts, args); err != nil {
				return err
			}
			if opts.Top.Interactive {
				// and gather any missing args that are available through interactive mode
				if err := AzureSecretArgsInteractive(input); err != nil {
					return err
				}
			}
			// create the secret
			if err := createAzureSecret(opts.Top.Ctx, opts.Metadata, *input, opts.Create.DryRun, opts.Top.Output); err != nil {
				return err
			}
			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringSliceVar(&input.ApiKeys.Entries, "api-keys", []string{}, "comma-separated list of azure api key=value entries")

	return cmd
}

const azurePromptApiKeys = "Enter API key entry (key=value)"

func AzureSecretArgsInteractive(input *options.AzureSecret) error {

	if err := cliutil.GetStringSliceInput(azurePromptApiKeys, &input.ApiKeys.Entries); err != nil {
		return err
	}

	return nil
}

func createAzureSecret(ctx context.Context, meta core.Metadata, input options.AzureSecret, dryRun bool, outputType printers.OutputType) error {
	if input.ApiKeys.Entries == nil {
		return errors.Errorf("must provide azure api keys")
	}
	secret := &gloov1.Secret{
		Metadata: meta,
		Kind: &gloov1.Secret_Azure{
			Azure: &gloov1.AzureSecret{
				ApiKeys: input.ApiKeys.MustMap(),
			},
		},
	}

	if !dryRun {
		var err error
		secretClient := helpers.MustSecretClient()
		if secret, err = secretClient.Write(secret, clients.WriteOpts{Ctx: ctx}); err != nil {
			return err
		}

	}

	_ = printers.PrintSecrets(gloov1.SecretList{secret}, outputType)
	return nil
}
