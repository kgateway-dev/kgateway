package install

import (
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"helm.sh/helm/v3/pkg/chartutil"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/cobra"
)

func ingressCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "ingress",
		Short:  "install the Gloo Ingress Controller on Kubernetes",
		Long:   "requires kubectl to be installed",
		PreRun: setVerboseMode(opts),
		RunE: func(cmd *cobra.Command, args []string) error {

			ingressOverrides, err := chartutil.ReadValues([]byte(ingressValues))
			if err != nil {
				return eris.Wrapf(err, "parsing override values for ingress mode")
			}

			if err := NewInstaller(opts, DefaultHelmClient()).Install(&InstallerConfig{
				InstallCliArgs: &opts.Install,
				ExtraValues:    ingressOverrides,
				Verbose:        opts.Top.Verbose,
				Ctx:            opts.Top.Ctx,
			}); err != nil {
				return eris.Wrapf(err, "installing gloo edge in ingress mode")
			}

			return nil
		},
	}
	flagutils.AddGlooInstallFlags(cmd.Flags(), &opts.Install)
	return cmd
}
