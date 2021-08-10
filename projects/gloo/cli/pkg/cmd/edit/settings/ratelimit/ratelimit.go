package ratelimit

import (
	"time"

	editOptions "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/edit/options"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"

	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	ratelimitpb "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/ratelimit"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func RateLimitConfig(opts *editOptions.EditOptions, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {

	optsExt := &RateLimitSettings{}

	cmd := &cobra.Command{
		// Use command constants to aid with replacement.
		Use:     constants.CONFIG_RATELIMIT_COMMAND.Use,
		Aliases: constants.CONFIG_RATELIMIT_COMMAND.Aliases,
		Short:   "Configure rate limit settings (Enterprise)",
		Long:    "Let gloo know the location of the rate limit server. This is a Gloo Enterprise feature.",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if opts.Top.Interactive {
				if err := AddSettingsInteractive(optsExt); err != nil {
					return err
				}
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return editSettings(opts, optsExt, args)
		},
	}

	AddConfigFlagsSettings(cmd.Flags(), optsExt)
	cliutils.ApplyOptions(cmd, optionsFunc)

	cmd.AddCommand(RateLimitCustomConfig(opts))
	return cmd
}

func editSettings(opts *editOptions.EditOptions, optsExt *RateLimitSettings, args []string) error {
	settingsClient := helpers.MustNamespacedSettingsClient(opts.Top.Ctx, opts.Metadata.GetNamespace())
	settings, err := settingsClient.Read(opts.Metadata.GetNamespace(), opts.Metadata.GetName(), clients.ReadOpts{})
	if err != nil {
		return errors.Wrapf(err, "Error reading settings")
	}

	var rlSettings ratelimitpb.Settings
	if rls := settings.GetRatelimitServer(); rls != nil {
		rlSettings = *rls
	}

	if rlSettings.GetRatelimitServerRef() == nil {
		rlSettings.RatelimitServerRef = new(core.ResourceRef)
	}
	if optsExt.RateLimitServerUpstreamRef.GetName() != "" {
		rlSettings.GetRatelimitServerRef().Name = optsExt.RateLimitServerUpstreamRef.Name
	}
	if optsExt.RateLimitServerUpstreamRef.GetNamespace() != "" {
		rlSettings.GetRatelimitServerRef().Namespace = optsExt.RateLimitServerUpstreamRef.Namespace
	}

	var zeroDuration time.Duration
	if optsExt.RequestTimeout != zeroDuration {
		rlSettings.RequestTimeout = prototime.DurationToProto(optsExt.RequestTimeout)
	}
	if optsExt.DenyOnFailure != nil {
		rlSettings.DenyOnFail = *optsExt.DenyOnFailure
	}

	settings.RatelimitServer = &rlSettings
	_, err = settingsClient.Write(settings, clients.WriteOpts{OverwriteExisting: true})
	return err
}

func AddSettingsInteractive(opts *RateLimitSettings) error {

	if err := cliutil.GetStringInput("name of the ratelimit server upstream: ", &opts.RateLimitServerUpstreamRef.Name); err != nil {
		return err
	}
	if err := cliutil.GetStringInput("namespace of the ratelimit server upstream: ", &opts.RateLimitServerUpstreamRef.Namespace); err != nil {
		return err
	}

	var requestTimeout string
	if err := cliutil.GetStringInput("the timeout for a request: ", &requestTimeout); err != nil {
		return err
	}

	t, err := time.ParseDuration(requestTimeout)
	if err != nil {
		return err
	}
	opts.RequestTimeout = t

	var deny bool
	if err := cliutil.GetBoolInput("enable failure mode deny: ", &deny); err != nil {
		return err
	}
	opts.DenyOnFailure = &deny

	return nil
}

type RateLimitSettings struct {
	RateLimitServerUpstreamRef core.ResourceRef
	RequestTimeout             time.Duration
	DenyOnFailure              *bool
}

func AddConfigFlagsSettings(set *pflag.FlagSet, opts *RateLimitSettings) {
	set.StringVar(&opts.RateLimitServerUpstreamRef.Name, "ratelimit-server-name", "", "name of the ext rate limit upstream")
	set.StringVar(&opts.RateLimitServerUpstreamRef.Namespace, "ratelimit-server-namespace", "", "namespace of the ext rate limit upstream")
	set.DurationVar(&opts.RequestTimeout, "request-timeout", time.Duration(0), "The timeout of the request to the rate limit server. set to 0 to use the default.")

	flag := set.VarPF(newBoolValue(nil, &opts.DenyOnFailure), "deny-on-failure", "", "On a failure to contact rate limit server, or on a timeout - deny the request (default is to allow)")
	flag.NoOptDefVal = "true"

}
