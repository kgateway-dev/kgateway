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
	set.BoolVar(&install.WithUi, "with-admin-console", false, "install gloo and a read-only version of its admin console")
}

func AddEnterpriseInstallFlags(set *pflag.FlagSet, install *options.Install) {
	set.BoolVarP(&install.DryRun, "dry-run", "d", false, "Dump the raw installation yaml instead of applying it to kubernetes")
	set.StringVar(&install.LicenseKey, "license-key", "", "License key to activate GlooE features")
	set.BoolVar(&install.WithUi, "with-admin-console", false, "install gloo and a read-only version of its admin console")
	set.BoolVar(&install.WithGlooFed, "with-gloo-fed", true, "Install Gloo-Fed alongside Gloo Enterprise")
	// Gloo-fed
	set.StringVar(&install.Federation.HelmChartOverride, "gloo-fed-file", "", "Install Gloo Federation from this Helm chart archive file rather than from a release")
	set.StringSliceVar(&install.Federation.HelmChartValueFileNames, "gloo-fed-values", []string{}, "List of files with value overrides for the Gloo Fed Helm chart, (e.g. --values file1,file2 or --values file1 --values file2)")
	set.StringVar(&install.Federation.HelmReleaseName, "gloo-fed-release-name", constants.GlooFedReleaseName, "helm release name")
	set.BoolVar(&install.Federation.CreateNamespace, "gloo-fed-create-namespace", true, "Create the namespace to install gloo fed into")
	set.StringVar(&install.Federation.Namespace, "gloo-fed-namespace", defaults.GlooFed, "namespace to install gloo fed into")
}

func AddFederationInstallFlags(set *pflag.FlagSet, install *options.Install) {
	set.BoolVar(&install.DryRun, "dry-run", false, "Dump the raw installation yaml instead of applying it to kubernetes")
	set.StringVar(&install.Federation.HelmChartOverride, "file", "", "Install Gloo Fed from this Helm chart archive file rather than from a release")
	set.StringSliceVar(&install.Federation.HelmChartValueFileNames, "values", []string{}, "List of files with value overrides for the Gloo Fed Helm chart, (e.g. --values file1,file2 or --values file1 --values file2)")
	set.StringVar(&install.Federation.HelmReleaseName, "release-name", constants.GlooFedReleaseName, "helm release name")
	set.StringVar(&install.Version, "version", "", "version to install (e.g. 0.0.6, defaults to latest)")
	set.BoolVar(&install.Federation.CreateNamespace, "create-namespace", true, "Create the namespace to install gloo fed into")
	set.StringVar(&install.Federation.Namespace, "namespace", defaults.GlooFed, "namespace to install gloo fed into")
	set.StringVar(&install.LicenseKey, "license-key", "", "License key to activate Gloo Fed features")
}

func AddFederationDemoFlags(set *pflag.FlagSet, install *options.Install) {
	set.StringVar(&install.LicenseKey, "license-key", "", "License key to activate Gloo Fed features")
	set.StringVar(&install.Federation.HelmChartOverride, "file", "", "Install Gloo Fed from this Helm chart archive file rather than from a release")
}

func AddKnativeInstallFlags(set *pflag.FlagSet, install *options.Knative) {
	set.StringVar(&install.InstallKnativeVersion, "install-knative-version", "0.10.0",
		"Version of Knative Serving to install, when --install-knative is set to `true`. This version"+
			" will also be used to install Knative Monitoring, --install-monitoring is set")
	set.BoolVarP(&install.InstallKnative, "install-knative", "k", true,
		"Bundle Knative-Serving with your Gloo installation")
	set.BoolVarP(&install.SkipGlooInstall, "skip-installing-gloo", "g", false,
		"Skip installing Gloo Edge. Only Knative components will be installed")
	set.BoolVarP(&install.InstallKnativeEventing, "install-eventing", "e", false,
		"Bundle Knative-Eventing with your Gloo installation. Requires install-knative to be true")
	set.StringVar(&install.InstallKnativeEventingVersion, "install-eventing-version", "0.10.0",
		"Version of Knative Eventing to install, when --install-eventing is set to `true`")
	set.BoolVarP(&install.InstallKnativeMonitoring, "install-monitoring", "m", false,
		"Bundle Knative-Monitoring with your Gloo installation. Requires install-knative to be true")
}
