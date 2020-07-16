package unregister

import (
	"fmt"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

var EmptyClusterNameError = eris.New("please provide a cluster name to unregister")

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.UNREGISTER_COMMAND.Use,
		Short: constants.UNREGISTER_COMMAND.Short,
		Long:  constants.UNREGISTER_COMMAND.Long,
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, clusterName := range args {
				err := helpers.MustSecretClient().
					Delete(opts.Cluster.Register.FederationNamespace, clusterName, clients.DeleteOpts{})
				if err != nil {
					fmt.Printf("Error unregistering cluster %s", clusterName)
				}
			}
			if len(args) == 0 {
				if opts.Cluster.Register.ClusterName == "" {
					return EmptyClusterNameError
				}
				return helpers.MustSecretClient().
					Delete(opts.Cluster.Register.FederationNamespace, opts.Cluster.Register.ClusterName, clients.DeleteOpts{})
			}
			return nil
		},
	}

	pflags := cmd.PersistentFlags()
	flagutils.AddUnregisterFlags(pflags, &opts.Cluster.Register)
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
