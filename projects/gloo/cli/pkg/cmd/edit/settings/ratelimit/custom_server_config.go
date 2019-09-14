package ratelimit

import (
	"regexp"
	"strings"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"

	"github.com/gogo/protobuf/types"
	"github.com/pkg/errors"
	editOptions "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/edit/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmdutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	ratelimitpb "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/plugins/ratelimit"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/utils"
	"github.com/solo-io/go-utils/protoutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RateLimitCustomConfig(opts *editOptions.EditOptions, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {

	cmd := &cobra.Command{
		// Use command constants to aid with replacement.
		Use:   "custom-server-config",
		Short: "Add a custom rate limit settings (Enterprise)",
		Long: `This allows using lyft rate limit server configuration language to configure the rate limit server.
		For more information see: https://github.com/lyft/ratelimit
		Note: do not add the 'domain' configuration key.
		This is a Gloo Enterprise feature.`, RunE: func(cmd *cobra.Command, args []string) error {
			return edit(opts)
		},
	}

	return cmd
}

func edit(opts *editOptions.EditOptions) error {
	settingsClient := helpers.MustSettingsClient()
	settings, err := settingsClient.Read(opts.Metadata.Namespace, opts.Metadata.Name, clients.ReadOpts{})
	if err != nil {
		return errors.Wrapf(err, "Error reading settings")
	}

	var rlSettings ratelimitpb.EnvoySettings
	err = utils.UnmarshalExtension(settings, constants.EnvoyRateLimitExtensionName, &rlSettings)
	if err != nil {
		if err != utils.NotFoundError {
			return err
		}
	}

	customcfg := rlSettings.CustomConfig
	if customcfg == nil {
		customcfg = new(ratelimitpb.EnvoySettings_RateLimitCustomConfig)
	}

	var editor cmdutils.Editor
	editor.JsonTransform = func(jsn []byte) []byte {
		// make unit upper case
		unit := "\"unit\":"
		findUnits := regexp.MustCompile(unit + "\\s*\"(second|minute|hour|day)\"")
		return findUnits.ReplaceAllFunc(jsn, func(u []byte) []byte {
			return []byte(unit + strings.ToUpper(string(u[len(unit):])))
		})
	}
	customcfgProto, err := editor.EditConfig(customcfg)
	if err != nil {
		return err
	}
	customcfg = customcfgProto.(*ratelimitpb.EnvoySettings_RateLimitCustomConfig)
	rlSettings.CustomConfig = customcfg

	rlStruct, err := protoutils.MarshalStruct(&rlSettings)
	if err != nil {
		return err
	}

	if settings.Extensions == nil {
		settings.Extensions = &gloov1.Extensions{}
	}

	if settings.Extensions.Configs == nil {
		settings.Extensions.Configs = make(map[string]*types.Struct)
	}

	settings.Extensions.Configs[constants.EnvoyRateLimitExtensionName] = rlStruct
	_, err = settingsClient.Write(settings, clients.WriteOpts{OverwriteExisting: true})

	return err
}
