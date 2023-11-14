package flagutils

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/spf13/pflag"
)

func AddGlooInstallFlags(set *pflag.FlagSet, install *options.Install) {
	set.BoolVarP(&install.DryRun, "dry-run", "d", false, "Dump the raw installation yaml instead of applying it to kubernetes")
	set.StringVarP(&install.Gloo.HelmChartOverride, "file", "f", "", "Install Gloo from this Helm chart archive file rather than from a release")
	set.StringSliceVarP(&install.Gloo.HelmChartValueFileNames, "values", "", []string{}, "List of files with value overrides for the Gloo Helm chart, (e.g. --values file1,file2 or --values file1 --values file2)")
	set.StringVar(&install.Gloo.HelmReleaseName, "release-name", constants.GlooReleaseName, "helm release name")
	set.StringVar(&install.Version, "version", "", "version to install (e.g. 1.4.0, defaults to latest)")
	set.BoolVar(&install.Gloo.CreateNamespace, "create-namespace", true, "Create the namespace to install gloo into")
	set.StringVarP(&install.Gloo.Namespace, "namespace", "n", defaults.GlooSystem, "namespace to install gloo into")
}
