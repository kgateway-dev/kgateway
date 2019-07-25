package flagutils

import "github.com/spf13/pflag"

func AddOutputFlag(set *pflag.FlagSet, strptr *string) {
	set.StringVarP(strptr, "output", "o", "", "output format: (yaml, json, table)")
}

func AddFileFlag(set *pflag.FlagSet, strptr *string) {
	set.StringVarP(strptr, "file", "f", "", "file to be read or written to")
}

func AddDryRunFlag(set *pflag.FlagSet, dryRun *bool) {
	set.BoolVarP(dryRun, "dry-run", "", false, "print kubernetes-formatted yaml "+
		"rather than creating or updating a resource")
}

func AddPrintYamlFlag(set *pflag.FlagSet, yaml *bool) {
	set.BoolVarP(yaml, "yaml", "", false, "print basic (non-kubernetes) yaml "+
		"rather than creating or updating a resource")
}
