package secret

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/argsutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"

	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/spf13/cobra"
)

func tlsCmd(opts *options.Options) *cobra.Command {
	input := &opts.Create.InputSecret.TlsSecret
	cmd := &cobra.Command{
		Use:   "tls",
		Short: `Create a secret with the given name`,
		Long: "Create a secret with the given name. " +
			"The format of the secret data is: `{\"tls\" : { \"ca.crt\": [root ca], \"tls.crt\": [cert chain], \"tls.key\": [private key], \"tls.ocsp-staple\": [ocsp staple]}}`. ",
		RunE: func(c *cobra.Command, args []string) error {
			if err := argsutils.MetadataArgsParse(opts, args); err != nil {
				return err
			}
			if opts.Top.Interactive {
				// and gather any missing args that are available through interactive mode
				if err := TlsSecretArgsInteractive(input); err != nil {
					return err
				}
			}
			// create the secret
			if err := createTlsSecret(opts.Top.Ctx, &opts.Metadata, *input, opts.Create.DryRun, opts.Top.Output); err != nil {
				return err
			}
			return nil
		},
	}

	flags := cmd.Flags()

	flags.StringVar(&input.RootCaFilename, "rootca", "", "filename of rootca for secret")
	flags.StringVar(&input.PrivateKeyFilename, "privatekey", "", "filename of privatekey for secret")
	flags.StringVar(&input.CertChainFilename, "certchain", "", "filename of certchain for secret")
	flags.StringVar(&input.OCSPStapleFilename, "ocspstaple", "", "filename of ocspstaple for secret")

	return cmd
}

const (
	tlsPromptRootCa     = "filename of rootca for secret (optional)"
	tlsPromptPrivateKey = "filename of privatekey for secret"
	tlsPromptCertChain  = "filename of certchain for secret"
	tlsPromptOcspStaple = "filename of ocspstaple for secret (optional)"
)

func TlsSecretArgsInteractive(input *options.TlsSecret) error {
	if err := cliutil.GetStringInput(tlsPromptRootCa, &input.RootCaFilename); err != nil {
		return err
	}
	if err := cliutil.GetStringInput(tlsPromptPrivateKey, &input.PrivateKeyFilename); err != nil {
		return err
	}
	if err := cliutil.GetStringInput(tlsPromptCertChain, &input.CertChainFilename); err != nil {
		return err
	}
	if err := cliutil.GetStringInput(tlsPromptOcspStaple, &input.OCSPStapleFilename); err != nil {
		return err
	}

	return nil
}

func createTlsSecret(ctx context.Context, meta *core.Metadata, input options.TlsSecret, dryRun bool, outputType printers.OutputType) error {
	// read the values
	tlsSecretData, err := input.ReadFiles()
	if err != nil {
		return err
	}

	secret := &gloov1.Secret{
		Metadata: meta,
		Kind: &gloov1.Secret_Tls{
			Tls: tlsSecretData,
		},
	}

	if !dryRun {
		var err error
		secretClient := helpers.MustSecretClient(ctx)
		if secret, err = secretClient.Write(secret, clients.WriteOpts{Ctx: ctx}); err != nil {
			return err
		}

	}

	_ = printers.PrintSecrets(gloov1.SecretList{secret}, outputType)
	return nil
}
