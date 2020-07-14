package flagutils

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/spf13/pflag"
)

func AddGlooUninstallFlags(set *pflag.FlagSet, opts *options.Uninstall) {
	set.StringVarP(&opts.Namespace, "namespace", "n", defaults.GlooSystem, "namespace in which Gloo is installed")
	set.StringVar(&opts.HelmReleaseName, "release-name", constants.GlooReleaseName, "helm release name")
	set.BoolVar(&opts.DeleteCrds, "delete-crds", false, "Delete all gloo crds (all custom gloo objects will be deleted)")
	set.BoolVar(&opts.DeleteNamespace, "delete-namespace", false, "Delete the namespace (all objects written to this namespace will be deleted)")
	set.BoolVar(&opts.DeleteAll, "all", false, "Deletes all gloo resources, including the namespace, crds, and cluster role")
}

func AddGlooFedUninstallFlags(set *pflag.FlagSet, opts *options.FedUninstall) {
	set.StringVar(&opts.Namespace, "fed-namespace", defaults.GlooFed, "namespace in which Gloo Fed is installed")
	set.StringVar(&opts.HelmReleaseName, "fed-release-name", constants.GlooFedReleaseName, "helm release name")
	set.BoolVar(&opts.DeleteCrds, "fed-delete-crds", false, "Delete all gloo fed crds (all custom gloo fed objects will be deleted)")
	set.BoolVar(&opts.DeleteNamespace, "fed-delete-namespace", false, "Delete the namespace (all objects written to this namespace will be deleted)")
	set.BoolVar(&opts.DeleteAll, "fed-delete-all", false, "Deletes all gloo fed resources, including the namespace, crds, and cluster role")
}
