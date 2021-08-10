package ratelimit

import (
	"regexp"
	"strings"

	errors "github.com/rotisserie/eris"
	editOptions "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/edit/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmdutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	ratelimitpb "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RateLimitCustomConfig(opts *editOptions.EditOptions, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {

	cmd := &cobra.Command{
		// Use command constants to aid with replacement.
		Use:   "server-config",
		Short: "Add rate-limit descriptor settings (Enterprise)",
		Long: `This allows using lyft rate-limit server configuration language to configure the rate-limit server.
		For more information see: https://github.com/lyft/ratelimit
		Note: do not add the 'domain' configuration key.
		This is a Gloo Enterprise feature.`, RunE: func(cmd *cobra.Command, args []string) error {
			return edit(opts)
		},
	}

	return cmd
}

func edit(opts *editOptions.EditOptions) error {
	settingsClient := helpers.MustNamespacedSettingsClient(opts.Top.Ctx, opts.Metadata.GetNamespace())
	settings, err := settingsClient.Read(opts.Metadata.GetNamespace(), opts.Metadata.GetName(), clients.ReadOpts{})
	if err != nil {
		return errors.Wrapf(err, "Error reading settings")
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
	var rlSettings ratelimitpb.ServiceSettings
	if rls := settings.Ratelimit; rls != nil {
		rlSettings = *rls
	}
	rlSettingsProto, err := editor.EditConfig(&rlSettings)
	if err != nil {
		return err
	}

	rlSettings = *rlSettingsProto.(*ratelimitpb.ServiceSettings)
	settings.Ratelimit = &ratelimitpb.ServiceSettings{
		Descriptors:    rlSettings.GetDescriptors(),
		SetDescriptors: rlSettings.GetSetDescriptors(),
	}
	_, err = settingsClient.Write(settings, clients.WriteOpts{OverwriteExisting: true})

	return err
}
