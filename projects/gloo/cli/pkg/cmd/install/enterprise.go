package install

import (
	"github.com/solo-io/gloo/pkg/version"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/go-utils/errors"
	"github.com/spf13/cobra"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

const (
	GlooEHelmRepoTemplate = "https://storage.googleapis.com/gloo-ee-helm/charts/gloo-ee-%s.tgz"
)

func enterpriseCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "enterprise",
		Short:  "install the Gloo Enterprise Gateway on Kubernetes",
		Long:   "requires kubectl to be installed",
		PreRun: setVerboseMode(opts),
		RunE: func(cmd *cobra.Command, args []string) error {

			extraValues := map[string]interface{}{
				"license_key": opts.Install.Enterprise.LicenseKey,
			}
			if opts.Install.Enterprise.Release != "" {
				version.EnterpriseTag = opts.Install.Enterprise.Release
			}

			if err := NewInstaller(DefaultHelmClient()).Install(&InstallerConfig{
				InstallCliArgs: &opts.Install,
				ExtraValues:    extraValues,
				Enterprise:     true,
				Verbose:        opts.Top.Verbose,
			}); err != nil {
				return errors.Wrapf(err, "installing Gloo Enterprise in gateway mode")
			}

			return nil
		},
	}

	pFlags := cmd.PersistentFlags()
	flagutils.AddEnterpriseInstallFlags(pFlags, &opts.Install)
	return cmd
}
