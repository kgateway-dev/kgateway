package secret

import (
	"context"

	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/surveyutils"

	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/argsutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/spf13/cobra"
)

var (
	MissingNameOrSecret = errors.Errorf("The Name or the key is missing for the encryption secret")
	KeyIsNotValidLength = errors.Errorf("The length of the key is not 32 bytes in length")
)

type encryptionKeySecretstruct struct {
	Key string
}

func EncryptionKeyCmd(opts *options.Options) *cobra.Command {
	meta := &opts.Metadata
	input := &encryptionKeySecretstruct{}
	cmd := &cobra.Command{
		Use:   "encryptionkey",
		Short: `Create an encryption key secret with the given name`,
		Long: "Create an encryption key secret with the given name. The format of the secret data is: <key>" +
			"The encryption key will be stored is the secret data under the key `key`.",
		RunE: func(c *cobra.Command, args []string) error {
			if err := argsutils.MetadataArgsParse(opts, args); err != nil {
				return err
			}
			if opts.Top.Interactive {
				// and gather any missing args that are available through interactive mode
				if err := encryptionKeyArgsInteractive(opts.Top.Ctx, meta, input); err != nil {
					return err
				}
			}
			// create the secret
			if err := createEncryptionKeySecret(opts.Top.Ctx, meta, input, opts.Create.DryRun, opts.Top.Output); err != nil {
				return err
			}
			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&input.Key, "key", "", "key for encryption")
	return cmd
}

const (
	encryptionKeyName   = "Enter name of the Encryption Key Secret: "
	encryptionKeyPrompt = "Enter Encryption Key, must be 32 bytes in length: "
)

func encryptionKeyArgsInteractive(ctx context.Context, meta *core.Metadata, input *encryptionKeySecretstruct) error {
	if err := surveyutils.InteractiveNamespace(ctx, &meta.Namespace); err != nil {
		return err
	}
	if err := cliutil.GetStringInput(encryptionKeyName, &meta.Name); err != nil {
		return err
	}
	if err := cliutil.GetStringInput(encryptionKeyPrompt, &input.Key); err != nil {
		return err
	}
	return nil
}

func createEncryptionKeySecret(ctx context.Context, meta *core.Metadata, input *encryptionKeySecretstruct, dryRun bool, outputType printers.OutputType) error {
	const encryptionKeyLength = 32
	if meta.GetName() == "" || input.Key == "" {
		return MissingNameOrSecret
	}
	if len(input.Key) != encryptionKeyLength {
		return KeyIsNotValidLength
	}
	secret := &gloov1.Secret{
		Metadata: meta,
		Kind: &gloov1.Secret_Encryption{
			Encryption: &gloov1.EncryptionKeySecret{
				Key: input.Key,
			},
		},
	}

	if !dryRun {
		secretClient := helpers.MustSecretClient(ctx)
		if _, err := secretClient.Write(secret, clients.WriteOpts{Ctx: ctx}); err != nil {
			return err
		}
	}
	return printers.PrintSecrets(gloov1.SecretList{secret}, outputType)
}
