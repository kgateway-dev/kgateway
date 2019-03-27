package secret

import (
	"context"
	"fmt"
	"io/ioutil"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/common"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/argsutils"

	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/surveyutils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/spf13/cobra"
)

func tlsCmd(opts *options.Options) *cobra.Command {
	input := opts.Create.InputSecret.TlsSecret
	cmd := &cobra.Command{
		Use:   "tls",
		Short: `Create a secret with the given name`,
		Long:  `Create a secret with the given name`,
		RunE: func(c *cobra.Command, args []string) error {
			if err := argsutils.MetadataArgsParse(opts, args); err != nil {
				return err
			}
			if opts.Top.Interactive {
				// and gather any missing args that are available through interactive mode
				if err := TlsSecretArgsInteractive(&opts.Metadata, &input); err != nil {
					return err
				}
			}
			// create the secret
			if err := createTlsSecret(opts.Top.Ctx, opts.Metadata, input, opts.Create.DryRun); err != nil {
				return err
			}
			return nil
		},
	}

	flags := cmd.Flags()

	flags.StringVar(&input.RootCaFilename, "rootca", "", "filename of rootca for secret")
	flags.StringVar(&input.PrivateKeyFilename, "privatekey", "", "filename of privatekey for secret")
	flags.StringVar(&input.CertChainFilename, "certchain", "", "filename of certchain for secret")

	return cmd
}

func TlsSecretArgsInteractive(meta *core.Metadata, input *options.TlsSecret) error {
	if err := surveyutils.InteractiveNamespace(&meta.Namespace); err != nil {
		return err
	}

	if err := cliutil.GetStringInput("name of secret", &meta.Name); err != nil {
		return err
	}
	if err := cliutil.GetStringInput("filename of rootca for secret", &input.RootCaFilename); err != nil {
		return err
	}
	if err := cliutil.GetStringInput("filename of privatekey for secret", &input.PrivateKeyFilename); err != nil {
		return err
	}
	if err := cliutil.GetStringInput("filename of certchain for secret", &input.CertChainFilename); err != nil {
		return err
	}

	return nil
}

func createTlsSecret(ctx context.Context, meta core.Metadata, input options.TlsSecret, dryRun bool) error {
	if meta.Name == "" {
		return errors.Errorf("must provide name")
	}
	if meta.Namespace == "" {
		return errors.Errorf("must provide namespace")
	}

	// read the values
	rootCa, err := ioutil.ReadFile(input.RootCaFilename)
	if err != nil {
		return errors.Wrapf(err, "reading rootca file: %v", input.RootCaFilename)
	}
	privateKey, err := ioutil.ReadFile(input.PrivateKeyFilename)
	if err != nil {
		return errors.Wrapf(err, "reading privatekey file: %v", input.PrivateKeyFilename)
	}
	certChain, err := ioutil.ReadFile(input.CertChainFilename)
	if err != nil {
		return errors.Wrapf(err, "reading certchain file: %v", input.CertChainFilename)
	}

	secret := &gloov1.Secret{
		Metadata: meta,
		Kind: &gloov1.Secret_Tls{
			Tls: &gloov1.TlsSecret{
				CertChain:  string(certChain),
				PrivateKey: string(privateKey),
				RootCa:     string(rootCa),
			},
		},
	}

	if dryRun {
		return common.PrintKubeSecret(ctx, secret)
	}

	secretClient := helpers.MustSecretClient()
	if _, err = secretClient.Write(secret, clients.WriteOpts{Ctx: ctx}); err != nil {
		return err
	}

	fmt.Printf("Created TLS secret [%v] in namespace [%v]\n", meta.Name, meta.Namespace)

	return nil
}
