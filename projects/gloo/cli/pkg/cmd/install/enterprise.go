package install

import (
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
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
				"license_key":      opts.Install.LicenseKey,
				"gloo-fed.enabled": opts.Install.WithGlooFed,
			}
			if opts.Install.LicenseKey == "" {
				return eris.New("No license key provided, please re-run the install with the following flag `--license-key=<YOUR-LICENSE-KEY>")
			}
			mode := Enterprise
			if err := NewInstaller(opts, DefaultHelmClient()).Install(&InstallerConfig{
				InstallCliArgs: &opts.Install,
				ExtraValues:    extraValues,
				Mode:           mode, // mode will be overwritten in Install to Gloo if the helm chart doesn't have gloo subchart
				Verbose:        opts.Top.Verbose,
				Ctx:            opts.Top.Ctx,
			}); err != nil {
				return eris.Wrapf(err, "installing Gloo Edge Enterprise in gateway mode")
			}

			return nil
		},
	}

	pFlags := cmd.PersistentFlags()
	flagutils.AddGlooInstallFlags(cmd.Flags(), &opts.Install)
	flagutils.AddEnterpriseInstallFlags(pFlags, &opts.Install)

	pFlags.Lookup("gloo-fed-values").Hidden = true
	return cmd
}
