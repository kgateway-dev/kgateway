package list

import (
	"os"
	"os/exec"

	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.LIST_COMMAND.Use,
		Short: constants.LIST_COMMAND.Short,
		RunE: func(cmd *cobra.Command, args []string) error {
			kCmd := "kubectl get secrets -n gloo-fed -o=jsonpath='{range .items[*]}{.metadata.name} {.type}{\"\\n\"}{end}'" +
				"| grep -i solo.io/kubeconfig | cut -d ' ' -f1"
			listCmd := exec.Command("bash", "-c", kCmd)
			listCmd.Stdout = os.Stderr
			listCmd.Stderr = os.Stderr
			if err := listCmd.Run(); err != nil {
				return errors.Wrapf(err, "failed to start port-forward")
			}
			return nil
		},
	}

	pflags := cmd.PersistentFlags()
	flagutils.AddListFlags(pflags, &opts.Cluster.Register)
	return cmd
}
