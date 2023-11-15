package flagutils

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/pflag"
)

func AddDebugFlags(set *pflag.FlagSet, top *options.Top) {
	set.BoolVar(&top.Zip, "zip", false, "save logs to a tar file (specify location with -f)")
	set.BoolVar(&top.ErrorsOnly, "errors-only", false, "filter for error logs only")
}
