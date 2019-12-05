package flagutils

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/spf13/pflag"
)

func AddUninstallFlags(set *pflag.FlagSet, opts *options.Uninstall) {
	set.StringVarP(&opts.Namespace, "namespace", "n", defaults.GlooSystem, "namespace in which Gloo is installed")
	set.BoolVar(&opts.DeleteCrds, "delete-crds", false, "Delete all gloo crds (all custom gloo objects will be deleted)")
	set.BoolVar(&opts.DeleteNamespace, "delete-namespace", false, "Delete the namespace (all objects written to this namespace will be deleted)")
	set.BoolVar(&opts.DeleteAll, "all", false, "Deletes all gloo resources, including the namespace, crds, and cluster role")
	set.BoolVar(&opts.Force, "force", false, "Uninstalls Gloo even if the installation ID cannot be determined from the gloo pod labels (using this may delete cluster-scoped resources belonging to other Gloo installations)")
}
