package flagutils

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers/types"
	"github.com/spf13/pflag"
)

func AddOutputFlag(set *pflag.FlagSet, outputType *types.OutputType) {
	set.VarP(outputType, "output", "o", "output format: (yaml, json, table, kube-yaml)")
}

func AddGetFlags(set *pflag.FlagSet, getOpts *options.Get) {
	set.BoolVar(&getOpts.Wide, "wide", false, "if set, will fetch additional details")
}

func AddFileFlag(set *pflag.FlagSet, strptr *string) {
	set.StringVarP(strptr, "file", "f", "", "file to be read or written to")
}

func AddDryRunFlag(set *pflag.FlagSet, dryRun *bool) {
	set.BoolVarP(dryRun, "dry-run", "", false, "print kubernetes-formatted yaml "+
		"rather than creating or updating a resource")
}
