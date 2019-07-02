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

func AddKnativeInstallFlags(set *pflag.FlagSet, install *options.Install) {
	set.BoolVarP(&install.InstallKnative, "install-knative", "k", true, "Install Knative-Serving before installing Gloo")
	set.StringVarP(&install.InstallKnativeVersion, "install-knative-version", "v", "0.7.0",
		"Version of Knative-Serving to install, when --install-knative is set to `true`")
}
