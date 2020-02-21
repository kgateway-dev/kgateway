package prerun

import (
	"context"
	"os"

	"github.com/solo-io/reporting-client/pkg/signature"

	"github.com/solo-io/gloo/pkg/utils/setuputils"
	"github.com/solo-io/gloo/pkg/utils/usage"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/spf13/cobra"
)

func HarmonizeDryRunAndOutputFormat(opts *options.Options, cmd *cobra.Command) error {
	// in order to allow table output by default, and meaningful dry runs we need to override the output default.
	// if we want a dry run, and the output is any other format, we do not override the output flag.
	// enforcing this in the PersistentPreRun saves us from having to do so in any new printers or output types
	if (opts.Create.DryRun || opts.Add.DryRun) && !cmd.PersistentFlags().Changed(flagutils.OutputFlag) {
		opts.Top.Output = printers.DryRunFallbackOutputType
	}
	return nil
}

func SetKubeConfigEnv(opts *options.Options, cmd *cobra.Command) error {
	// If kubeconfig is set, and not equal to "", set the ENV
	if opts.Top.KubeConfig != "" {
		return os.Setenv("KUBECONFIG", opts.Top.KubeConfig)
	}
	return nil
}

func ReportUsage(opts *options.Options, cmd *cobra.Command) error {
	if opts.Top.DisableUsageStatistics {
		return nil
	}

	signatureManager := &signature.FileBackedSignatureManager{}

	_ = setuputils.StartReportingUsage(context.Background(), &usage.CliUsageReader{}, "glooctl", signatureManager)
	return nil
}
