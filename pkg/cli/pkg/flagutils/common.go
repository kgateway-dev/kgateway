package flagutils

import (
	"github.com/solo-io/gloo/v2/pkg/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/v2/pkg/cli/pkg/printers"
	"github.com/spf13/pflag"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	OutputFlag     = "output"
	FileFlag       = "file"
	DryRunFlag     = "dry-run"
	VersionFlag    = "version"
	LocalChartFlag = "local-chart"
	ShowYamlFlag   = "show-yaml"
)

func AddCheckOutputFlag(set *pflag.FlagSet, outputType *printers.OutputType) {
	set.VarP(outputType, OutputFlag, "o", "output format: (json, table)")
}

func AddVersionFlag(set *pflag.FlagSet, version *string) {
	set.StringVarP(version, VersionFlag, "", "", "version of gloo's CRDs to check against")
}

func AddLocalChartFlag(set *pflag.FlagSet, localChart *string) {
	set.StringVarP(localChart, LocalChartFlag, "", "", "check against CRDs in helm chart at path specified by this flag (supersedes --version)")
}

func AddShowYamlFlag(set *pflag.FlagSet, showYaml *bool) {
	set.BoolVarP(showYaml, ShowYamlFlag, "", false, "show full yaml of both CRDs that differ")
}

func AddOutputFlag(set *pflag.FlagSet, outputType *printers.OutputType) {
	set.VarP(outputType, OutputFlag, "o", "output format: (yaml, json, table, kube-yaml, wide)")
}

func AddFileFlag(set *pflag.FlagSet, strptr *string) {
	set.StringVarP(strptr, FileFlag, "f", "", "file to be read or written to")
}

func AddDryRunFlag(set *pflag.FlagSet, dryRun *bool) {
	set.BoolVarP(dryRun, DryRunFlag, "", false, "print kubernetes-formatted yaml "+
		"rather than creating or updating a resource")
}

// currently only used by install/uninstall/dashboard but should be changed if it gets shared by more
func AddVerboseFlag(set *pflag.FlagSet, opts *options.Options) {
	set.BoolVarP(&opts.Top.Verbose, "verbose", "v", false,
		"If true, output from kubectl commands will print to stdout/stderr")
}

func AddKubeConfigFlag(set *pflag.FlagSet, kubeConfig *string) {
	set.StringVarP(kubeConfig, clientcmd.RecommendedConfigPathFlag, "", "", "kubeconfig to use, if not standard one")
}
