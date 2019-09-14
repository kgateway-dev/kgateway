package virtualservice

import (
	editOptions "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/edit/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RateLimitConfig(opts *editOptions.EditOptions, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {

	cmd := &cobra.Command{
		// Use command constants to aid with replacement.
		Use:     constants.CONFIG_RATELIMIT_COMMAND.Use,
		Aliases: constants.CONFIG_RATELIMIT_COMMAND.Aliases,
		Short:   "Configure rate limit settings (Enterprise)",
		Long:    "Let gloo know the location of the rate limit server. This is a Gloo Enterprise feature.",
	}

	cliutils.ApplyOptions(cmd, optionsFunc)

	cmd.AddCommand(RateLimitCustomConfig(opts))
	return cmd
}
