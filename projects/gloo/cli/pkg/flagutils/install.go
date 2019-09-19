package flagutils

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/spf13/pflag"
)

func AddInstallFlags(set *pflag.FlagSet, install *options.Install) {
	set.BoolVarP(&install.DryRun, "dry-run", "d", false, "Dump the raw installation yaml instead of applying it to kubernetes")
	set.BoolVarP(&install.Upgrade, "upgrade", "u", false, "Upgrade an existing v1 gateway installation to use v2 CRDs. Set this when upgrading from v0.17.x or earlier versions of gloo")
	set.StringVarP(&install.HelmChartOverride, "file", "f", "", "Install Gloo from this Helm chart archive file rather than from a release")
	set.StringVarP(&install.HelmChartValues, "values", "", "", "Values for the Gloo Helm chart")
	set.StringVarP(&install.Namespace, "namespace", "n", defaults.GlooSystem, "namespace to install gloo into")
	set.BoolVar(&install.WithUi, "with-admin-console", false, "install gloo and a read-only version of its admin console")
}

func AddEnterpriseInstallFlags(set *pflag.FlagSet, install *options.Install) {
	set.StringVar(&install.LicenseKey, "license-key", "", "License key to activate GlooE features")
}

func AddKnativeInstallFlags(set *pflag.FlagSet, install *options.Knative) {
	set.StringVar(&install.InstallKnativeVersion, "install-knative-version", "0.8.0",
		"Version of Knative Serving to install, when --install-knative is set to `true`. This version"+
			" will also be used to install Knative Monitoring, --install-monitoring is set")
	set.BoolVarP(&install.InstallKnative, "install-knative", "k", true,
		"Bundle Knative-Serving with your Gloo installation")
	set.BoolVarP(&install.SkipGlooInstall, "skip-installing-gloo", "g", false,
		"Skip installing Gloo. Only Knative components will be installed")
	set.BoolVarP(&install.InstallKnativeEventing, "install-eventing", "e", false,
		"Bundle Knative-Eventing with your Gloo installation. Requires install-knative to be true")
	set.StringVar(&install.InstallKnativeEventingVersion, "install-eventing-version", "0.7.0",
		"Version of Knative Eventing to install, when --install-eventing is set to `true`")
	set.BoolVarP(&install.InstallKnativeBuild, "install-build", "b", false,
		"Bundle Knative-Build with your Gloo installation. Requires install-knative to be true")
	set.StringVar(&install.InstallKnativeBuildVersion, "install-build-version", "0.7.0",
		"Version of Knative Build to install, when --install-build is set to `true`")
	set.BoolVarP(&install.InstallKnativeMonitoring, "install-monitoring", "m", false,
		"Bundle Knative-Monitoring with your Gloo installation. Requires install-knative to be true")
}

// currently only used by install/uninstall but should be changed if it gets shared by more
func AddVerboseFlag(set *pflag.FlagSet, opts *options.Options) {
	set.BoolVarP(&opts.Top.Verbose, "verbose", "v", false,
		"If true, output from kubectl commands will print to stdout/stderr")
}
