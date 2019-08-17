package flagutils

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/spf13/pflag"
)

func AddOutputFlag(set *pflag.FlagSet, outputType *printers.OutputType) {
	set.VarP(outputType, "output", "o", "output format: (yaml, json, table, kube-yaml, wide)")
}

func AddFileFlag(set *pflag.FlagSet, strptr *string) {
	set.StringVarP(strptr, "file", "f", "", "file to be read or written to")
}

func AddDryRunFlag(set *pflag.FlagSet, dryRun *bool) {
	set.BoolVarP(dryRun, "dry-run", "", false, "print kubernetes-formatted yaml "+
		"rather than creating or updating a resource")
}
