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

func headerCmd(opts *options.Options) *cobra.Command {
	input := &opts.Create.InputSecret.HeaderSecret
	cmd := &cobra.Command{
		Use:   "header",
		Short: `Create a header secret with the given name`,
		Long: "Create a header secret with the given name. The format of the secret data is: " +
			"`{\"headers\" : [name=value,...]}`",
		RunE: func(c *cobra.Command, args []string) error {
			if err := argsutils.MetadataArgsParse(opts, args); err != nil {
				return err
			}
			if opts.Top.Interactive {
				// gather any missing args that are available through interactive mode
				if err := HeaderSecretArgsInteractive(input); err != nil {
					return err
				}
			}
			// create the secret
			if err := createHeaderSecret(opts.Top.Ctx, &opts.Metadata, *input, opts.Create.DryRun, opts.Top.Output); err != nil {
				return err
			}
			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringSliceVar(&input.Headers.Entries, "headers", []string{}, "comma-separated list of header name=value entries")

	return cmd
}

const headersPrompt = "Enter header entry (name=value): "

func HeaderSecretArgsInteractive(input *options.HeaderSecret) error {
	if err := cliutil.GetStringSliceInput(headersPrompt, &input.Headers.Entries); err != nil {
		return err
	}

	return nil
}

func createHeaderSecret(ctx context.Context, meta *core.Metadata, input options.HeaderSecret, dryRun bool, outputType printers.OutputType) error {
	if input.Headers.Entries == nil {
		return errors.Errorf("must provide headers")
	}
	secret := &gloov1.Secret{
		Metadata: meta,
		Kind: &gloov1.Secret_Header{
			Header: &gloov1.HeaderSecret{
				Headers: input.Headers.MustMap(),
			},
		},
	}

	if !dryRun {
		var err error
		secretClient := helpers.MustSecretClientWithOptions(ctx, 0, []string{meta.GetNamespace()})
		if secret, err = secretClient.Write(secret, clients.WriteOpts{Ctx: ctx}); err != nil {
			return err
		}
	}

	_ = printers.PrintSecrets(gloov1.SecretList{secret}, outputType)
	return nil
}
