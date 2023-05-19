package secret

import (
	"context"

	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/argsutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/surveyutils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	MissingInputError = errors.Errorf("Username and password must be provided for credentials secret")
)

type credentialsSecret struct {
	Username string
	Password string
}

func ExtAuthAccountCredentialsCmd(opts *options.Options) *cobra.Command {
	meta := &opts.Metadata
	input := &credentialsSecret{}
	cmd := &cobra.Command{
		Use:   "authcredentials",
		Short: `Create an AuthenticationCredentials secret with the given name (Enterprise)`,
		Long: "Create an AuthenticationCredentials secret with the given name. The AuthenticationCredentials secret contains " +
			"a username and password to bind as an LDAP service account. This is an enterprise-only feature.",
		RunE: func(c *cobra.Command, args []string) error {
			err := argsutils.MetadataArgsParse(opts, args)
			if err != nil {
				return err
			}
			if opts.Top.Interactive {
				if err := credentialsSecretArgsInteractive(opts.Top.Ctx, meta, input); err != nil {
					return err
				}
			}
			if err := createCredentialsSecret(opts.Top.Ctx, meta, input, opts.Create.DryRun, opts.Top.Output); err != nil {
				return err
			}
			return nil
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&input.Username, "username", "", "user name to be stored in secret")
	flags.StringVar(&input.Password, "password", "", "password to be stored in secret")
	return cmd
}
func credentialsSecretArgsInteractive(ctx context.Context, meta *core.Metadata, input *credentialsSecret) error {
	if err := surveyutils.InteractiveNamespace(ctx, &meta.Namespace); err != nil {
		return err
	}

	if err := cliutil.GetStringInput("Name of secret:", &meta.Name); err != nil {
		return err
	}

	if err := cliutil.GetStringInput("Username to store:", &input.Username); err != nil {
		return err
	}
	if err := cliutil.GetStringInput("Password to store:", &input.Password); err != nil {
		return err
	}
	return nil
}
func createCredentialsSecret(ctx context.Context, meta *core.Metadata, input *credentialsSecret, dryRun bool, outputType printers.OutputType) error {
	if input.Username == "" || input.Password == "" {
		return MissingInputError
	}
	secret := &gloov1.Secret{
		Metadata: meta,
		Kind: &gloov1.Secret_Credentials{
			Credentials: &gloov1.AccountCredentialsSecret{
				Username: input.Username,
				Password: input.Password,
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
