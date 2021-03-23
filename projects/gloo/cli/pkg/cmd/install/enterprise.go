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
				"license_key": opts.Install.LicenseKey,
			}

			mode := Enterprise
			if err := NewInstaller(DefaultHelmClient()).Install(&InstallerConfig{
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
	pFlags.Lookup("gloo-fed-file").Hidden = true
	pFlags.Lookup("gloo-fed-values").Hidden = true
	pFlags.Lookup("gloo-fed-release-name").Hidden = true
	pFlags.Lookup("gloo-fed-create-namespace").Hidden = true
	pFlags.Lookup("gloo-fed-namespace").Hidden = true
	return cmd
}
