package secret

import (
	"context"
	"encoding/base64"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"

	"github.com/hashicorp/go-uuid"
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

const (
	apiKeySource_Generate = "generate an apikey"
	apiKeySource_Provide  = "provide an apikey"
)

var apiKeySourceOptions = []string{
	apiKeySource_Generate,
	apiKeySource_Provide,
}

var (
	MissingApiKeyError = errors.Errorf("ApiKey not provided or generated")
)

type apiKeySecret struct {
	// If true, generate an API key.
	GenerateApiKey bool
	// If present, use the provided apikey
	ApiKey string
	// A list of labels (key=value) for the apikey secret.
	Labels []string
}

func ExtAuthApiKeyCmd(opts *options.Options) *cobra.Command {
	meta := &opts.Metadata
	input := apiKeySecret{}
	cmd := &cobra.Command{
		Use:   "apikey",
		Short: `Create an ApiKey secret with the given name (Enterprise)`,
		Long: "Create an ApiKey secret with the given name. The ApiKey secret contains a single apikey. " +
			"This is an enterprise-only feature. The API key will be stored is the secret data under the key `api-key`.",
		RunE: func(c *cobra.Command, args []string) error {
			err := argsutils.MetadataArgsParse(opts, args)
			if err != nil {
				return err
			}

			if opts.Top.Interactive {
				// and gather any missing args that are available through interactive mode
				if err := apiKeySecretArgsInteractive(opts.Top.Ctx, meta, &input); err != nil {
					return err
				}
			}
			// create the secret
			if err := createApiKeySecret(opts.Top.Ctx, meta, input, opts.Create.DryRun, opts.Top.Output); err != nil {
				return err
			}

			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&input.ApiKey, "apikey", "", "apikey to be stored in secret")
	flags.BoolVar(&input.GenerateApiKey, "apikey-generate", false, "generate an apikey")
	flags.StringSliceVar(&input.Labels, "apikey-labels", []string{}, "comma-separated labels for the apikey secret (key=value)")

	return cmd
}

func apiKeySecretArgsInteractive(ctx context.Context, meta *core.Metadata, input *apiKeySecret) error {
	if err := surveyutils.InteractiveNamespace(ctx, &meta.Namespace); err != nil {
		return err
	}

	if err := cliutil.GetStringInput("Name of secret:", &meta.Name); err != nil {
		return err
	}

	apiKeySource := ""
	if err := cliutil.ChooseFromList(
		"How would you like to provide an apikey: ",
		&apiKeySource,
		apiKeySourceOptions,
	); err != nil {
		return err
	}

	if apiKeySource == apiKeySource_Generate {
		input.GenerateApiKey = true
	} else {
		if err := cliutil.GetStringInput("Enter ApiKey:", &input.ApiKey); err != nil {
			return err
		}
	}

	if err := cliutil.GetStringSliceInput("Add a label (key=value) for the ApiKey (empty to finish)", &input.Labels); err != nil {
		return err
	}

	return nil
}

func createApiKeySecret(ctx context.Context, meta *core.Metadata, input apiKeySecret, dryRun bool, outputType printers.OutputType) error {
	if input.ApiKey == "" {
		if !input.GenerateApiKey {
			return MissingApiKeyError
		}
		var err error
		apiKey, err := uuid.GenerateUUID()
		if err != nil {
			return err
		}
		input.ApiKey = base64.StdEncoding.EncodeToString([]byte(apiKey))
	}

	var labels options.InputMapStringString
	labels.Entries = input.Labels
	meta.Labels = labels.MustMap()

	secret := &gloov1.Secret{
		Metadata: meta,
		Kind: &gloov1.Secret_ApiKey{
			ApiKey: &v1.ApiKey{
				ApiKey: input.ApiKey,
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
