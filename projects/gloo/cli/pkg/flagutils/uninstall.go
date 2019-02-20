package flagutils

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/spf13/pflag"
)

func AddUninstallFlags(set *pflag.FlagSet, opts *options.Uninstall) {
	set.StringVarP(&opts.Namespace, "namespace", "n", defaults.GlooSystem, "namespace in which Gloo is installed")
}
