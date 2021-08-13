package settings

import (
	editOptions "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/edit/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	flagutilsExt "github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	surveyutilsExt "github.com/solo-io/gloo/projects/gloo/cli/pkg/surveyutils"
	extauthpb "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/spf13/cobra"
)

func ExtAuthConfig(opts *editOptions.EditOptions, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {

	optsExt := &options.OIDCSettings{}

	cmd := &cobra.Command{
		// Use command constants to aid with replacement.
		Use:     constants.CONFIG_EXTAUTH_COMMAND.Use,
		Aliases: constants.CONFIG_EXTAUTH_COMMAND.Aliases,
		Short:   "Configure external auth settings (Enterprise)",
		Long:    "Let gloo know the location of the ext auth server. This is a Gloo Enterprise feature.",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if opts.Top.Interactive {
				if err := surveyutilsExt.AddSettingsExtAuthFlagsInteractive(optsExt); err != nil {
					return err
				}
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return editSettings(opts, optsExt, args)
		},
	}

	flagutilsExt.AddConfigFlagsOIDCSettings(cmd.Flags(), optsExt)
	cliutils.ApplyOptions(cmd, optionsFunc)

	return cmd
}

func editSettings(opts *editOptions.EditOptions, optsExt *options.OIDCSettings, args []string) error {
	settingsClient := helpers.MustNamespacedSettingsClient(opts.Top.Ctx, opts.Metadata.GetNamespace())
	settings, err := settingsClient.Read(opts.Metadata.GetNamespace(), opts.Metadata.GetName(), clients.ReadOpts{})
	if err != nil {
		return errors.Wrapf(err, "Error reading settings")
	}

	extAuthSettings := settings.Extauth
	if extAuthSettings == nil {
		extAuthSettings = new(extauthpb.Settings)
	}
	if extAuthSettings.GetExtauthzServerRef() == nil {
		extAuthSettings.ExtauthzServerRef = new(core.ResourceRef)
	}
	if optsExt.ExtAuthServerUpstreamRef.GetName() != "" {
		extAuthSettings.GetExtauthzServerRef().Name = optsExt.ExtAuthServerUpstreamRef.Name
	}
	if optsExt.ExtAuthServerUpstreamRef.GetNamespace() != "" {
		extAuthSettings.GetExtauthzServerRef().Namespace = optsExt.ExtAuthServerUpstreamRef.Namespace
	}

	if settings.GetExtauth() == nil {
		settings.Extauth = extAuthSettings
	}

	_, err = settingsClient.Write(settings, clients.WriteOpts{OverwriteExisting: true})
	return err
}
