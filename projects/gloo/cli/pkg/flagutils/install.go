package flagutils

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/spf13/pflag"
)

func AddInstallFlags(set *pflag.FlagSet, install *options.Install) {
	set.BoolVarP(&install.DryRun, "dry-run", "d", false, "Dump the raw installation yaml instead of applying it to kubernetes")
	set.StringVarP(&install.HelmChartOverride, "file", "f", "", "Install Gloo from this Helm chart archive file rather than from a release")
	set.StringVarP(&install.Namespace, "namespace", "n", defaults.GlooSystem, "namespace to install gloo into")
}

func AddKnativeInstallFlags(set *pflag.FlagSet, install *options.Knative) {
	set.StringVarP(&install.InstallKnativeVersion, "install-knative-version", "v", "0.7.0",
		"Version of Knative-Serving to install, when --install-knative is set to `true`")
	set.BoolVarP(&install.InstallKnative, "install-knative", "k", true,
		"Bundle Knative-Serving with your Gloo installation")
	set.BoolVarP(&install.SkipGlooInstall, "skip-installing-gloo", "g", false,
		"Skip installing Gloo. Only Knative components will be installed")
	set.BoolVarP(&install.InstallKnativeEventing, "install-eventing", "e", false,
		"Bundle Knative-Eventing with your Gloo installation. Requires install-knative to be true")
	set.BoolVarP(&install.InstallKnativeBuild, "install-build", "b", false,
		"Bundle Knative-Build with your Gloo installation. Requires install-knative to be true")
	set.BoolVarP(&install.InstallKnativeMonitoring, "install-monitoring", "m", false,
		"Bundle Knative-Monitoring with your Gloo installation. Requires install-knative to be true")
}
