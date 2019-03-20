package flagutils

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/spf13/pflag"
)

func AddUninstallFlags(set *pflag.FlagSet, opts *options.Uninstall) {
	set.StringVarP(&opts.Namespace, "namespace", "n", defaults.GlooSystem, "namespace in which Gloo is installed")
	set.BoolVarP(&opts.DeleteNamespace, "--delete-namespace", "", false, "Delete the namespace (all objects written to this namespace will be deleted)")
	set.BoolVarP(&opts.DeleteCrds, "--delete-crds", "", false, "Delete all gloo crds (all custom gloo objects will be deleted)")
	set.BoolVarP(&opts.DeleteAll, "--all", "", false, "Deletes all gloo resour	ces, including the namespace, crds, and cluster role")
}
