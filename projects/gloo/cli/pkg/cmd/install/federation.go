package install

import (
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/spf13/cobra"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

const (
	GlooFedHelmRepoTemplate = "https://storage.googleapis.com/gloo-fed-helm/gloo-fed-%s.tgz"
)

func glooFedCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "federation",
		Short:  "install Gloo Federation on Kubernetes",
		Long:   "requires kubectl to be installed",
		PreRun: setVerboseMode(opts),
		RunE: func(cmd *cobra.Command, args []string) error {

			extraValues := map[string]interface{}{
				"license_key": opts.Install.LicenseKey,
			}

			opts.Install.Namespace = opts.Install.Federation.Namespace
			opts.Install.HelmChartOverride = opts.Install.Federation.HelmChartOverride
			opts.Install.HelmChartValueFileNames = opts.Install.Federation.HelmChartValueFileNames
			opts.Install.HelmReleaseName = opts.Install.Federation.HelmReleaseName
			opts.Install.Version = opts.Install.Federation.Version
			opts.Install.LicenseKey = opts.Install.Federation.LicenseKey

			if err := NewInstaller(DefaultHelmClient()).Install(&InstallerConfig{
				InstallCliArgs: &opts.Install,
				ExtraValues:    extraValues,
				Enterprise:     false,
				Federation:     true,
				Verbose:        opts.Top.Verbose,
			}); err != nil {
				return eris.Wrapf(err, "installing Gloo Federation")
			}

			return nil
		},
	}

	pflags := cmd.PersistentFlags()
	flagutils.AddFederationInstallFlags(pflags, &opts.Install.Federation)
	return cmd
}
