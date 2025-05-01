package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var (
	configFlags                 = genericclioptions.NewConfigFlags(true)
	matchVersionKubeConfigFlags = cmdutil.NewMatchVersionFlags(configFlags)
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use: "applier",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	configFlags.AddFlags(rootCmd.PersistentFlags())
	matchVersionKubeConfigFlags.AddFlags(rootCmd.PersistentFlags())

}
