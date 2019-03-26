package flagutils

import "github.com/spf13/pflag"

func AddOutputFlag(set *pflag.FlagSet, strptr *string) {
	set.StringVarP(strptr, "output", "o", "", "output format: (yaml, json, table)")
}

func AddFileFlag(set *pflag.FlagSet, strptr *string) {
	set.StringVarP(strptr, "file", "f", "", "file to be read or written to")
}

func AddKubeYamlFlag(set *pflag.FlagSet, kubeyaml *bool) {
	set.BoolVarP(kubeyaml, "kubeyaml", "k", false, "print kubernetes-formatted yaml " +
		"rather than creating or updating a resource")
}
